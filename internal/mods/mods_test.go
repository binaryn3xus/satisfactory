package mods

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockFileName(t *testing.T) {
	cases := map[string]string{
		"UNSC-Section-III": "unsc-section-iii-lock.json",
		"Default":          "default-lock.json",
		"MyModProfile":     "my-mod-profile-lock.json",
	}
	for input, want := range cases {
		if got := LockFileName(input); got != want {
			t.Errorf("LockFileName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseModList(t *testing.T) {
	raw := `
		# Core server mods
		NoFuel
		SmartFoundations@^2.0.0, AreaActions@latest
		PakUtilityMod@>=1.2.3
	`
	specs, err := ParseModList(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 4 {
		t.Fatalf("expected 4 mods, got %d: %+v", len(specs), specs)
	}
	if specs[0].Ref != "NoFuel" || specs[0].Version != ">=0.0.0" {
		t.Errorf("unexpected specs[0]: %+v", specs[0])
	}
	if specs[1].Ref != "SmartFoundations" || specs[1].Version != "^2.0.0" {
		t.Errorf("unexpected specs[1]: %+v", specs[1])
	}
	if specs[2].Ref != "AreaActions" || specs[2].Version != ">=0.0.0" {
		t.Errorf("unexpected specs[2]: %+v", specs[2])
	}
	if specs[3].Ref != "PakUtilityMod" || specs[3].Version != ">=1.2.3" {
		t.Errorf("unexpected specs[3]: %+v", specs[3])
	}
}

func TestVerifyInstallationStrictGate(t *testing.T) {
	tmp := t.TempDir()
	modsDir := filepath.Join(tmp, "FactoryGame", "Mods")
	if err := os.MkdirAll(filepath.Join(modsDir, "SML"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(modsDir, "NoFuel"), 0o755); err != nil {
		t.Fatal(err)
	}

	_ = os.WriteFile(filepath.Join(modsDir, "SML", "SML.uplugin"), []byte(`{"FriendlyName":"SML","SemVersion":"3.8.0"}`), 0o644)
	_ = os.WriteFile(filepath.Join(modsDir, "NoFuel", "NoFuel.uplugin"), []byte(`{"FriendlyName":"No Fuel","SemVersion":"1.1.0"}`), 0o644)

	lockJSON := `{
		"mods": {
			"SML": {"version": "3.8.0"},
			"NoFuel": {"version": "1.1.0"}
		}
	}`
	_ = os.WriteFile(filepath.Join(modsDir, "unsc-section-iii-lock.json"), []byte(lockJSON), 0o644)

	expected := []ModSpec{{Ref: "NoFuel", Version: ">=0.0.0", Enabled: true}}
	if err := VerifyInstallation(tmp, "UNSC-Section-III", expected); err != nil {
		t.Fatalf("expected verification to pass, got: %v", err)
	}

	// Now require a missing mod and verify strict gate blocks startup
	missing := append(expected, ModSpec{Ref: "MissingMod", Version: ">=0.0.0", Enabled: true})
	if err := VerifyInstallation(tmp, "UNSC-Section-III", missing); err == nil {
		t.Fatalf("expected strict verification to fail when MissingMod is absent")
	}
}

func TestBuildSMMProfileWithClientOnlyMods(t *testing.T) {
	tmp := t.TempDir()
	modsDir := filepath.Join(tmp, "FactoryGame", "Mods")
	_ = os.MkdirAll(filepath.Join(modsDir, "SML"), 0o755)
	_ = os.MkdirAll(filepath.Join(modsDir, "NoFuel"), 0o755)

	_ = os.WriteFile(filepath.Join(modsDir, "SML", "SML.uplugin"), []byte(`{"FriendlyName":"SML","SemVersion":"3.8.0"}`), 0o644)
	_ = os.WriteFile(filepath.Join(modsDir, "NoFuel", "NoFuel.uplugin"), []byte(`{"FriendlyName":"No Fuel","SemVersion":"1.1.0"}`), 0o644)
	_ = os.WriteFile(filepath.Join(modsDir, "unsc-section-iii-lock.json"), []byte(`{"mods":{"SML":{"version":"3.8.0"},"NoFuel":{"version":"1.1.0"}}}`), 0o644)

	clientOnly := []ModSpec{{Ref: "EfficiencyCheckerMod", Version: "^2.0.0", Enabled: true}}
	profileObj := BuildSMMProfile(tmp, "UNSC-Section-III", clientOnly)

	profMap := profileObj["profile"].(map[string]any)
	modsMap := profMap["mods"].(map[string]any)
	if _, ok := modsMap["NoFuel"]; !ok {
		t.Fatalf("expected NoFuel in exported .smmprofile")
	}
	if _, ok := modsMap["EfficiencyCheckerMod"]; !ok {
		t.Fatalf("expected client-only EfficiencyCheckerMod in exported .smmprofile")
	}
	if _, ok := modsMap["SML"]; ok {
		t.Fatalf("did not expect SML in top-level profile.mods")
	}
}
