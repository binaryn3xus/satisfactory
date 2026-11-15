package blueprints

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestListBlueprintsAndZip(t *testing.T) {
	tmp := t.TempDir()
	sessionDir := filepath.Join(tmp, "Project-MJOLNIR")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a symlink alias like import-satisfactory-blueprints.sh does
	_ = os.Symlink(sessionDir, filepath.Join(tmp, "project-mjolnir"))

	sbpPath := filepath.Join(sessionDir, "Iron Motors 10min.sbp")
	cfgPath := filepath.Join(sessionDir, "Iron Motors 10min.sbpcfg")

	if err := os.WriteFile(sbpPath, []byte("dummy-sbp-binary"), 0o644); err != nil {
		t.Fatal(err)
	}

	desc := "A perfectly load-balanced motor factory."
	var cfgBuf bytes.Buffer
	_ = binary.Write(&cfgBuf, binary.LittleEndian, int32(2))
	_ = binary.Write(&cfgBuf, binary.LittleEndian, int32(len(desc)))
	cfgBuf.WriteString(desc)
	if err := os.WriteFile(cfgPath, cfgBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := ListBlueprints(tmp)
	if err != nil {
		t.Fatalf("ListBlueprints error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 deduplicated blueprint, got %d", len(list))
	}
	if list[0].Name != "Iron Motors 10min" || list[0].SessionName != "Project-MJOLNIR" {
		t.Errorf("unexpected blueprint: %+v", list[0])
	}
	if list[0].Description != desc {
		t.Errorf("Description = %q, want %q", list[0].Description, desc)
	}

	var zipBuf bytes.Buffer
	if err := WriteSingleBlueprintZip(&zipBuf, list[0]); err != nil {
		t.Fatalf("WriteSingleBlueprintZip error: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBuf.Bytes()), int64(zipBuf.Len()))
	if err != nil {
		t.Fatalf("open zip reader: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("expected 2 files in zip (.sbp + .sbpcfg), got %d", len(zr.File))
	}
}
