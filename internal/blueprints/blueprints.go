package blueprints

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/binaryn3xus/satisfactory/internal/saves"
)

// Blueprint represents a Satisfactory blueprint (.sbp + optional .sbpcfg) on disk.
type Blueprint struct {
	Name          string `json:"name"`
	SessionName   string `json:"session_name"`
	Description   string `json:"description"`
	SBPPath       string `json:"-"`
	SBPCfgPath    string `json:"-"`
	HasConfig     bool   `json:"has_config"`
	Size          int64  `json:"size"`
	FormattedSize string `json:"formatted_size"`
	ModTime       int64  `json:"mtime"`
	FormattedTime string `json:"formatted_time"`
}

// ListBlueprints scans blueprintsDir for .sbp files (both top-level and 1-level session subdirectories),
// deduplicating symlinked directories by resolved canonical path.
func ListBlueprints(blueprintsDir string) ([]Blueprint, error) {
	entries, err := os.ReadDir(blueprintsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read blueprints dir %s: %w", blueprintsDir, err)
	}

	seenRealDirs := make(map[string]bool)
	seenRealFiles := make(map[string]bool)
	var list []Blueprint

	scanDir := func(dirPath, sessionName string) {
		realDir, err := filepath.EvalSymlinks(dirPath)
		if err != nil {
			realDir = dirPath
		}
		if seenRealDirs[realDir] {
			return
		}
		seenRealDirs[realDir] = true

		files, err := os.ReadDir(dirPath)
		if err != nil {
			return
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".sbp") {
				continue
			}
			sbpPath := filepath.Join(dirPath, f.Name())
			realSBP, err := filepath.EvalSymlinks(sbpPath)
			if err != nil {
				realSBP = sbpPath
			}
			if seenRealFiles[realSBP] {
				continue
			}
			seenRealFiles[realSBP] = true

			info, err := os.Stat(sbpPath)
			if err != nil || info.IsDir() {
				continue
			}

			baseName := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
			cfgPath := filepath.Join(dirPath, baseName+".sbpcfg")
			totalSize := info.Size()
			hasCfg := false
			var desc string

			if cfgInfo, err := os.Stat(cfgPath); err == nil && !cfgInfo.IsDir() {
				hasCfg = true
				totalSize += cfgInfo.Size()
				desc = ParseSBPCfgDescription(cfgPath)
			}

			list = append(list, Blueprint{
				Name:          baseName,
				SessionName:   sessionName,
				Description:   desc,
				SBPPath:       sbpPath,
				SBPCfgPath:    cfgPath,
				HasConfig:     hasCfg,
				Size:          totalSize,
				FormattedSize: saves.FormatSize(totalSize),
				ModTime:       info.ModTime().Unix(),
				FormattedTime: info.ModTime().UTC().Format("2006-01-02 15:04:05 UTC"),
			})
		}
	}

	// First scan non-symlink subdirectories so canonical session names win over lowercase/alias symlinks
	for _, entry := range entries {
		full := filepath.Join(blueprintsDir, entry.Name())
		if entry.Type()&os.ModeSymlink == 0 && entry.IsDir() {
			scanDir(full, entry.Name())
		}
	}
	// Then scan symlinked directories if they point somewhere not yet scanned
	for _, entry := range entries {
		full := filepath.Join(blueprintsDir, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(full); err == nil && st.IsDir() {
				scanDir(full, entry.Name())
			}
		}
	}
	// Finally scan any top-level .sbp files directly inside blueprintsDir
	scanDir(blueprintsDir, "Global")

	sort.Slice(list, func(i, j int) bool {
		if list[i].SessionName != list[j].SessionName {
			return strings.ToLower(list[i].SessionName) < strings.ToLower(list[j].SessionName)
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})

	return list, nil
}

// ParseSBPCfgDescription extracts the UTF-8 description text from a Satisfactory .sbpcfg file.
func ParseSBPCfgDescription(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < 8 {
		return ""
	}
	r := bytes.NewReader(data)
	var cfgVersion, strLen int32
	if err := binary.Read(r, binary.LittleEndian, &cfgVersion); err != nil {
		return ""
	}
	if err := binary.Read(r, binary.LittleEndian, &strLen); err != nil {
		return ""
	}

	if strLen <= 0 || strLen > 8192 || int(8+strLen) > len(data) {
		return ""
	}
	raw := data[8 : 8+strLen]
	raw = bytes.TrimRight(raw, "\x00")
	if !utf8.Valid(raw) {
		return ""
	}
	s := strings.TrimSpace(string(raw))
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}

// WriteSingleBlueprintZip streams a .zip archive containing the .sbp and .sbpcfg files for a single blueprint.
func WriteSingleBlueprintZip(w io.Writer, bp Blueprint) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	if err := addFileToZip(zw, bp.SBPPath, bp.Name+".sbp"); err != nil {
		return err
	}
	if bp.HasConfig && bp.SBPCfgPath != "" {
		if err := addFileToZip(zw, bp.SBPCfgPath, bp.Name+".sbpcfg"); err != nil {
			return err
		}
	}
	return nil
}

// WriteAllBlueprintsZip streams a .zip archive containing all provided blueprints grouped by session.
func WriteAllBlueprintsZip(w io.Writer, list []Blueprint) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, bp := range list {
		prefix := ""
		if bp.SessionName != "" && bp.SessionName != "Global" {
			prefix = bp.SessionName + "/"
		}
		if err := addFileToZip(zw, bp.SBPPath, prefix+bp.Name+".sbp"); err != nil {
			return err
		}
		if bp.HasConfig && bp.SBPCfgPath != "" {
			if err := addFileToZip(zw, bp.SBPCfgPath, prefix+bp.Name+".sbpcfg"); err != nil {
				return err
			}
		}
	}
	return nil
}

func addFileToZip(zw *zip.Writer, srcPath, zipEntryName string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = zipEntryName
	hdr.Method = zip.Deflate

	writer, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, f)
	return err
}
