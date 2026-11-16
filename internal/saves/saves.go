package saves

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SaveFile represents a Satisfactory world save (.sav) on disk along with its parsed header metadata.
type SaveFile struct {
	Name                string `json:"name"`
	Path                string `json:"-"`
	Size                int64  `json:"size"`
	FormattedSize       string `json:"formatted_size"`
	ModTime             int64  `json:"mtime"`
	FormattedTime       string `json:"formatted_time"`
	SessionName         string `json:"session_name"`
	SaveName            string `json:"save_name"`
	MapName             string `json:"map_name"`
	BuildVersion        int32  `json:"build_version"`
	PlayDurationSeconds int32  `json:"play_duration_seconds"`
	FormattedPlaytime   string `json:"formatted_playtime"`
	HeaderVersion       int32  `json:"header_version"`
	SaveVersion         int32  `json:"save_version"`
}

// FormatSize formats a byte count into a human-readable string (e.g. "2.7 MB").
func FormatSize(sizeBytes int64) string {
	val := float64(sizeBytes)
	for _, unit := range []string{"B", "KB", "MB", "GB"} {
		if val < 1024.0 {
			return fmt.Sprintf("%.1f %s", val, unit)
		}
		val /= 1024.0
	}
	return fmt.Sprintf("%.1f TB", val)
}

// FormatPlaytime formats play duration in seconds into "XH Ym" (e.g. "73h 15m").
func FormatPlaytime(seconds int32) string {
	if seconds <= 0 {
		return "0m"
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// ListSaves scans savesDir for .sav files (excluding ServerSettings*.sav), parses their binary headers,
// and returns them sorted newest-first by modification time.
func ListSaves(savesDir string) ([]SaveFile, error) {
	entries, err := os.ReadDir(savesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read saves dir %s: %w", savesDir, err)
	}

	var list []SaveFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sav") || strings.HasPrefix(name, "ServerSettings") {
			continue
		}

		fullPath := filepath.Join(savesDir, name)
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}

		sf := SaveFile{
			Name:          name,
			Path:          fullPath,
			Size:          info.Size(),
			FormattedSize: FormatSize(info.Size()),
			ModTime:       info.ModTime().Unix(),
			FormattedTime: info.ModTime().UTC().Format("2006-01-02 15:04:05 UTC"),
		}

		if hdr, err := ParseHeaderFile(fullPath); err == nil {
			sf.HeaderVersion = hdr.HeaderVersion
			sf.SaveVersion = hdr.SaveVersion
			sf.BuildVersion = hdr.BuildVersion
			sf.SaveName = hdr.SaveName
			sf.MapName = hdr.MapName
			sf.SessionName = hdr.SessionName
			sf.PlayDurationSeconds = hdr.PlayDurationSeconds
			sf.FormattedPlaytime = FormatPlaytime(hdr.PlayDurationSeconds)
		}

		if sf.SessionName == "" {
			sf.SessionName = inferSessionNameFromFilename(name)
		}

		list = append(list, sf)
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].ModTime != list[j].ModTime {
			return list[i].ModTime > list[j].ModTime
		}
		return list[i].Name < list[j].Name
	})

	return list, nil
}

func inferSessionNameFromFilename(filename string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if idx := strings.Index(base, "_autosave"); idx > 0 {
		return base[:idx]
	}
	return base
}

// ParseHeaderFile reads the first 2KB of a .sav file and extracts its uncompressed header metadata.
func ParseHeaderFile(path string) (*SaveFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, 2048)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return ParseHeaderBytes(buf[:n])
}

// ParseHeaderBytes parses the uncompressed header at the start of a Satisfactory .sav file.
// Supports both SaveHeaderVersion >= 14 (which includes SaveName before MapName)
// and older SaveHeaderVersion < 14 formats.
func ParseHeaderBytes(data []byte) (*SaveFile, error) {
	r := bytes.NewReader(data)

	var headerVersion, saveVersion, buildVersion int32
	if err := binary.Read(r, binary.LittleEndian, &headerVersion); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &saveVersion); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &buildVersion); err != nil {
		return nil, err
	}

	if headerVersion < 5 || headerVersion > 30 {
		return nil, fmt.Errorf("unsupported save header version: %d", headerVersion)
	}

	var saveName string
	if headerVersion >= 14 {
		s, err := readUEFString(r)
		if err != nil {
			return nil, fmt.Errorf("read SaveName: %w", err)
		}
		saveName = s
	}

	mapName, err := readUEFString(r)
	if err != nil {
		return nil, fmt.Errorf("read MapName: %w", err)
	}

	// MapOptions FString (e.g. "?listen?sessionName=...")
	if _, err := readUEFString(r); err != nil {
		return nil, fmt.Errorf("read MapOptions: %w", err)
	}

	sessionName, err := readUEFString(r)
	if err != nil {
		return nil, fmt.Errorf("read SessionName: %w", err)
	}

	var playDurationSeconds int32
	if err := binary.Read(r, binary.LittleEndian, &playDurationSeconds); err != nil {
		return nil, fmt.Errorf("read PlayDurationSeconds: %w", err)
	}

	// Guard against bogus duration values (> 20 years of continuous play)
	if playDurationSeconds < 0 || playDurationSeconds > 20*365*24*3600 {
		playDurationSeconds = 0
	}

	return &SaveFile{
		HeaderVersion:       headerVersion,
		SaveVersion:         saveVersion,
		BuildVersion:        buildVersion,
		SaveName:            saveName,
		MapName:             mapName,
		SessionName:         sessionName,
		PlayDurationSeconds: playDurationSeconds,
		FormattedPlaytime:   FormatPlaytime(playDurationSeconds),
	}, nil
}

// readUEFString reads a length-prefixed Unreal Engine FString (ASCII/UTF-8 or UTF-16 if negative length).
func readUEFString(r *bytes.Reader) (string, error) {
	var length int32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	if length < -1024 || length > 1024 {
		return "", fmt.Errorf("fstring length out of bounds: %d", length)
	}

	if length > 0 {
		buf := make([]byte, length)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return strings.TrimRight(string(buf), "\x00"), nil
	}

	// Negative length indicates UTF-16LE characters
	charCount := int(-length)
	buf := make([]byte, charCount*2)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	var out strings.Builder
	for i := 0; i+1 < len(buf); i += 2 {
		ch := uint16(buf[i]) | (uint16(buf[i+1]) << 8)
		if ch == 0 {
			break
		}
		out.WriteRune(rune(ch))
	}
	return out.String(), nil
}
