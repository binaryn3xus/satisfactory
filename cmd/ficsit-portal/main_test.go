package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/binaryn3xus/satisfactory/internal/mods"
)

func writeTestSave(t *testing.T, dir, filename, sessionName string, playtimeSec int32) {
	t.Helper()
	writeFStr := func(buf *bytes.Buffer, s string) {
		if s == "" {
			_ = binary.Write(buf, binary.LittleEndian, int32(0))
			return
		}
		b := append([]byte(s), 0)
		_ = binary.Write(buf, binary.LittleEndian, int32(len(b)))
		buf.Write(b)
	}
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, int32(14))
	_ = binary.Write(&buf, binary.LittleEndian, int32(60))
	_ = binary.Write(&buf, binary.LittleEndian, int32(517554))
	writeFStr(&buf, strings.TrimSuffix(filename, ".sav"))
	writeFStr(&buf, "Persistent_Level")
	writeFStr(&buf, "")
	writeFStr(&buf, sessionName)
	_ = binary.Write(&buf, binary.LittleEndian, playtimeSec)

	if err := os.WriteFile(filepath.Join(dir, filename), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPortalEndpoints(t *testing.T) {
	tmp := t.TempDir()
	savesDir := filepath.Join(tmp, "saved", "server")
	bpDir := filepath.Join(tmp, "saved", "blueprints", "UNSC-Section-III")
	modsDir := filepath.Join(tmp, "gamefiles", "FactoryGame", "Mods")
	logDir := filepath.Join(tmp, "gamefiles", "FactoryGame", "Saved", "Logs")

	for _, d := range []string{savesDir, bpDir, filepath.Join(modsDir, "NoFuel"), logDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	writeTestSave(t, savesDir, "UNSC-Section-III_autosave_0.sav", "UNSC-Section-III", 7200)
	_ = os.WriteFile(filepath.Join(bpDir, "MotorFactory.sbp"), []byte("sbp-data"), 0o644)
	_ = os.WriteFile(filepath.Join(modsDir, "NoFuel", "NoFuel.uplugin"), []byte(`{"FriendlyName":"No Fuel","SemVersion":"1.2.0"}`), 0o644)
	_ = os.WriteFile(filepath.Join(modsDir, "unsc-section-iii-lock.json"), []byte(`{"mods":{"NoFuel":{"version":"1.2.0"}}}`), 0o644)
	_ = os.WriteFile(filepath.Join(logDir, "FactoryGame.log"), []byte("[2026.10.10-18.00.02:200][125]LogNet: Join succeeded: BinaryNexus\n"), 0o644)

	srv := NewPortalServer(PortalConfig{
		Port:           "8080",
		ServerName:     "UNSC-Section-III",
		ProfileName:    "UNSC-Section-III",
		SavesDir:       savesDir,
		BlueprintsDir:  filepath.Join(tmp, "saved", "blueprints"),
		GamefilesDir:   filepath.Join(tmp, "gamefiles"),
		LogFile:        filepath.Join(logDir, "FactoryGame.log"),
		MaxPlayers:     8,
		ClientOnlyMods: []mods.ModSpec{{Ref: "EfficiencyCheckerMod", Version: ">=0.0.0", Enabled: true}},
	})
	handler := srv.Routes()

	// 1. /healthz
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "OK\n" {
		t.Fatalf("unexpected /healthz response: %d %q", rec.Code, rec.Body.String())
	}

	// 2. /mods.json
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/mods.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /mods.json, got %d", rec.Code)
	}
	var modsPayload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &modsPayload); err != nil {
		t.Fatal(err)
	}
	if modsPayload["profile_name"] != "UNSC-Section-III" {
		t.Errorf("unexpected profile_name: %v", modsPayload["profile_name"])
	}

	// 3. /UNSC-Section-III.smmprofile
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/UNSC-Section-III.smmprofile", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "EfficiencyCheckerMod") {
		t.Fatalf("expected client-only mod in .smmprofile, got: %s", rec.Body.String())
	}

	// 4. /blueprints.zip
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/blueprints.zip", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("expected 200 zip on /blueprints.zip, got %d", rec.Code)
	}

	// 5. /metrics
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "satisfactory_saves_total 1") {
		t.Fatalf("unexpected /metrics output: %s", rec.Body.String())
	}

	// 6. Dashboard /
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "BinaryNexus") || !strings.Contains(body, "MotorFactory") || !strings.Contains(body, "2h 00m") {
		t.Fatalf("dashboard missing expected pioneer, blueprint, or playtime content")
	}
	if !strings.Contains(body, `<link rel="icon" type="image/svg+xml" href="/favicon.svg">`) {
		t.Fatalf("dashboard missing favicon link tag")
	}

	// 7. /favicon.svg & /favicon.ico
	for _, p := range []string{"/favicon.svg", "/favicon.ico"} {
		frec := httptest.NewRecorder()
		handler.ServeHTTP(frec, httptest.NewRequest(http.MethodGet, p, nil))
		if frec.Code != http.StatusOK {
			t.Fatalf("expected 200 on %s, got %d", p, frec.Code)
		}
		if frec.Header().Get("Content-Type") != "image/svg+xml" {
			t.Fatalf("expected image/svg+xml on %s, got %s", p, frec.Header().Get("Content-Type"))
		}
		if !strings.Contains(frec.Body.String(), "<svg") {
			t.Fatalf("expected svg content on %s", p)
		}
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	// 1. Defaults
	os.Clearenv()
	cfg := loadConfigFromEnv()
	if cfg.Port != "8081" {
		t.Fatalf("expected default port 8081, got %s", cfg.Port)
	}
	if cfg.ProfileName != "default" {
		t.Fatalf("expected default profile 'default', got %s", cfg.ProfileName)
	}

	// 2. PORTAL_PORT overrides PORT
	t.Setenv("PORT", "8080")
	t.Setenv("PORTAL_PORT", "9090")
	t.Setenv("PORTAL_TITLE", "Custom Portal Title")
	t.Setenv("MODS_PROFILE", "my-profile")
	cfg2 := loadConfigFromEnv()
	if cfg2.Port != "9090" {
		t.Fatalf("expected PORTAL_PORT 9090, got %s", cfg2.Port)
	}
	if cfg2.ServerName != "Custom Portal Title" {
		t.Fatalf("expected PORTAL_TITLE 'Custom Portal Title', got %s", cfg2.ServerName)
	}
	if cfg2.ProfileName != "my-profile" {
		t.Fatalf("expected profile 'my-profile', got %s", cfg2.ProfileName)
	}
}
