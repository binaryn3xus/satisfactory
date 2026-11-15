package saves

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func writeFString(buf *bytes.Buffer, s string) {
	if s == "" {
		_ = binary.Write(buf, binary.LittleEndian, int32(0))
		return
	}
	b := append([]byte(s), 0)
	_ = binary.Write(buf, binary.LittleEndian, int32(len(b)))
	buf.Write(b)
}

func TestParseHeaderBytesV14(t *testing.T) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, int32(14))     // HeaderVersion (1.0+)
	_ = binary.Write(&buf, binary.LittleEndian, int32(60))     // SaveVersion
	_ = binary.Write(&buf, binary.LittleEndian, int32(502094)) // BuildVersion (CL)
	writeFString(&buf, "Project-MJOLNIR_101026-163231")        // SaveName
	writeFString(&buf, "Persistent_Level")                     // MapName
	writeFString(&buf, "")                                     // MapOptions
	writeFString(&buf, "Project-MJOLNIR")                      // SessionName
	_ = binary.Write(&buf, binary.LittleEndian, int32(263734)) // PlayDurationSeconds (73h 15m)

	hdr, err := ParseHeaderBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hdr.SessionName != "Project-MJOLNIR" {
		t.Errorf("SessionName = %q, want Project-MJOLNIR", hdr.SessionName)
	}
	if hdr.BuildVersion != 502094 {
		t.Errorf("BuildVersion = %d, want 502094", hdr.BuildVersion)
	}
	if hdr.FormattedPlaytime != "73h 15m" {
		t.Errorf("FormattedPlaytime = %q, want 73h 15m", hdr.FormattedPlaytime)
	}
}

func TestListSaves(t *testing.T) {
	tmp := t.TempDir()

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, int32(14))
	_ = binary.Write(&buf, binary.LittleEndian, int32(60))
	_ = binary.Write(&buf, binary.LittleEndian, int32(517554))
	writeFString(&buf, "UNSC-Section-III_autosave_0")
	writeFString(&buf, "Persistent_Level")
	writeFString(&buf, "")
	writeFString(&buf, "UNSC-Section-III")
	_ = binary.Write(&buf, binary.LittleEndian, int32(36000)) // 10h 00m

	if err := os.WriteFile(filepath.Join(tmp, "UNSC-Section-III_autosave_0.sav"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	// ServerSettings should be ignored
	if err := os.WriteFile(filepath.Join(tmp, "ServerSettings.7777.sav"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := ListSaves(tmp)
	if err != nil {
		t.Fatalf("ListSaves error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 save file, got %d", len(list))
	}
	if list[0].SessionName != "UNSC-Section-III" || list[0].FormattedPlaytime != "10h 00m" || list[0].BuildVersion != 517554 {
		t.Errorf("unexpected save metadata: %+v", list[0])
	}
}
