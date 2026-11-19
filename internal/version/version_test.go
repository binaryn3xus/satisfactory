package version

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectChangelistFromVersionFile(t *testing.T) {
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "FactoryGame", "Binaries", "Linux")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	versionJSON := `{
		"MajorVersion": 5,
		"MinorVersion": 3,
		"PatchVersion": 2,
		"Changelist": 517554,
		"CompatibleChangelist": 517554,
		"BranchName": "++FactoryGame+rel-main"
	}`
	if err := os.WriteFile(filepath.Join(binDir, "FactoryServer-Linux-Shipping.version"), []byte(versionJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cl, err := DetectChangelist(tmp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cl != 517554 {
		t.Fatalf("expected changelist 517554, got %d", cl)
	}
}

func TestDetectChangelistFallbackToLog(t *testing.T) {
	tmp := t.TempDir()
	logDir := filepath.Join(tmp, "FactoryGame", "Saved", "Logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logContent := "LogInit: Build: ++FactoryGame+rel-main-CL-520123\n"
	if err := os.WriteFile(filepath.Join(logDir, "FactoryGame.log"), []byte(logContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cl, err := DetectChangelist(tmp)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cl != 520123 {
		t.Fatalf("expected changelist 520123, got %d", cl)
	}
}

func TestReadLocalSteamBuildID(t *testing.T) {
	tmp := t.TempDir()
	steamDir := filepath.Join(tmp, "steamapps")
	if err := os.MkdirAll(steamDir, 0o755); err != nil {
		t.Fatal(err)
	}

	acf := `"AppState"
{
	"appid"		"1690800"
	"Universe"		"1"
	"name"		"Satisfactory Dedicated Server"
	"buildid"		"15987654"
}`
	if err := os.WriteFile(filepath.Join(steamDir, "appmanifest_1690800.acf"), []byte(acf), 0o644); err != nil {
		t.Fatal(err)
	}

	buildID, err := ReadLocalSteamBuildID(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buildID != "15987654" {
		t.Fatalf("expected buildid 15987654, got %s", buildID)
	}
}
