package version

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// GameVersionFile represents the JSON structure of FactoryServer-Linux-Shipping.version
// written by SteamCMD inside FactoryGame/Binaries/Linux/.
type GameVersionFile struct {
	BranchName           string `json:"BranchName"`
	BuildID              string `json:"BuildId"`
	MajorVersion         int    `json:"MajorVersion"`
	MinorVersion         int    `json:"MinorVersion"`
	PatchVersion         int    `json:"PatchVersion"`
	Changelist           int    `json:"Changelist"`
	CompatibleChangelist int    `json:"CompatibleChangelist"`
}

var (
	logBuildRegex    = regexp.MustCompile(`Build:\s*\+\+FactoryGame\+[^\s]+-CL-(\d+)`)
	acfBuildIDRegex  = regexp.MustCompile(`"buildid"\s+"(\d+)"`)
	versionRelPaths  = []string{
		"Engine/Binaries/Linux/UnrealServer-Linux-Shipping.version",
		"Engine/Binaries/Linux/FactoryServer-Linux-Shipping.version",
		"FactoryGame/Binaries/Linux/FactoryServer-Linux-Shipping.version",
	}
)

// DetectChangelist reads the installed Satisfactory Changelist (CL) from the gamefiles directory.
// It checks the official .version JSON file first, then falls back to FactoryGame.log if needed.
func DetectChangelist(gamefilesDir string) (int, error) {
	for _, rel := range versionRelPaths {
		fullPath := filepath.Join(gamefilesDir, rel)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		var vf GameVersionFile
		if err := json.Unmarshal(data, &vf); err == nil && vf.Changelist > 0 {
			return vf.Changelist, nil
		}
	}

	// Fallback: inspect the first 64KB of FactoryGame.log
	logPath := filepath.Join(gamefilesDir, "FactoryGame/Saved/Logs/FactoryGame.log")
	f, err := os.Open(logPath)
	if err == nil {
		defer f.Close()
		buf := make([]byte, 65536)
		n, _ := f.Read(buf)
		if matches := logBuildRegex.FindSubmatch(buf[:n]); len(matches) == 2 {
			if cl, err := strconv.Atoi(string(matches[1])); err == nil && cl > 0 {
				return cl, nil
			}
		}
	}

	return 0, fmt.Errorf("unable to detect Satisfactory changelist in %s", gamefilesDir)
}

// ReadLocalSteamBuildID reads the installed Steam buildid from steamapps/appmanifest_1690800.acf.
func ReadLocalSteamBuildID(gamefilesDir string) (string, error) {
	acfPath := filepath.Join(gamefilesDir, "steamapps", "appmanifest_1690800.acf")
	data, err := os.ReadFile(acfPath)
	if err != nil {
		return "", err
	}
	matches := acfBuildIDRegex.FindSubmatch(data)
	if len(matches) < 2 {
		return "", fmt.Errorf("buildid not found in %s", acfPath)
	}
	return string(matches[1]), nil
}
