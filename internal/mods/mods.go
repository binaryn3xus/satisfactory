package mods

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/binaryn3xus/satisfactory/internal/version"
)

// ModSpec represents a desired mod reference and SemVer constraint.
type ModSpec struct {
	Ref     string `json:"ref"`
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
}

// InstalledMod represents a mod currently installed in FactoryGame/Mods.
type InstalledMod struct {
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	FicsitURL   string `json:"ficsit_url"`
	IsSML       bool   `json:"is_sml"`
}

// ReconcileConfig configures declarative mod reconciliation.
type ReconcileConfig struct {
	GamefilesDir string
	FicsitBin    string
	LocalDir     string
	CacheDir     string
	Enabled      bool
	StrictMode   bool
	ProfileName  string
	UpdatePolicy string // "always" or "locked"
	Mods         []ModSpec
}

// ModChange describes a version change during reconciliation.
type ModChange struct {
	Ref        string
	OldVersion string
	NewVersion string
}

// ReconcileResult summarizes the outcome of a mod reconciliation run.
type ReconcileResult struct {
	Installed []InstalledMod
	Changes   []ModChange
}

var (
	ficsitLockCleaner = regexp.MustCompile(`[^a-zA-Z\d]]`)
	kebabCleaner      = regexp.MustCompile(`[^a-zA-Z\d]+`)
	matchFirstCap     = regexp.MustCompile(`(.)([A-Z][a-z]+)`)
	matchAllCap       = regexp.MustCompile(`([a-z\d])([A-Z])`)
)

// LockFileName returns the clean kebab-case lockfile filename for a profile (e.g., "unsc-section-iii-lock.json").
func LockFileName(profileName string) string {
	if profileName == "" {
		profileName = "Default"
	}
	name := matchFirstCap.ReplaceAllString(profileName, "${1}-${2}")
	name = matchAllCap.ReplaceAllString(name, "${1}-${2}")
	name = kebabCleaner.ReplaceAllLiteralString(name, "-")
	name = strings.Trim(name, "-")
	return strings.ToLower(name) + "-lock.json"
}

// FicsitCLILockFileName returns the exact filename produced by ficsit-cli's internal lockFilePath function.
func FicsitCLILockFileName(profileName string) string {
	if profileName == "" {
		profileName = "Default"
	}
	name := matchFirstCap.ReplaceAllString(profileName, "${1}_${2}")
	name = matchAllCap.ReplaceAllString(name, "${1}_${2}")
	name = ficsitLockCleaner.ReplaceAllLiteralString(name, "-")
	return strings.ToLower(name) + "-lock.json"
}

// LockFileCandidates returns all candidate lockfile paths in FactoryGame/Mods for a given profile.
func LockFileCandidates(modsDir, profileName string) []string {
	c1 := filepath.Join(modsDir, LockFileName(profileName))
	c2 := filepath.Join(modsDir, FicsitCLILockFileName(profileName))
	if c1 == c2 {
		return []string{c1}
	}
	return []string{c1, c2}
}

// NormalizeVersion converts user shorthand ("", "latest", "*") into ficsit-cli SemVer range ">=0.0.0".
func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	switch strings.ToLower(v) {
	case "", "latest", "*":
		return ">=0.0.0"
	default:
		return v
	}
}

// ParseModList parses a comma- or newline-separated string of ModRef or ModRef@Version entries.
func ParseModList(raw string) ([]ModSpec, error) {
	var specs []ModSpec
	seen := make(map[string]int)

	lines := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})

	for _, token := range lines {
		line := strings.TrimSpace(token)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		ref := line
		ver := ">=0.0.0"
		if at := strings.Index(line, "@"); at != -1 {
			ref = strings.TrimSpace(line[:at])
			ver = NormalizeVersion(line[at+1:])
		}
		if ref == "" {
			return nil, fmt.Errorf("invalid mod entry %q", token)
		}

		spec := ModSpec{
			Ref:     ref,
			Version: ver,
			Enabled: true,
		}
		if existingIdx, ok := seen[ref]; ok {
			specs[existingIdx] = spec
		} else {
			seen[ref] = len(specs)
			specs = append(specs, spec)
		}
	}

	return specs, nil
}

// modsConfigFile supports both a simple {"mods": [...]} format and exported .smmprofile JSON files.
type modsConfigFile struct {
	Mods    []ModSpec `json:"mods"`
	Profile *struct {
		Name string `json:"name"`
		Mods map[string]struct {
			Version string `json:"version"`
			Enabled bool   `json:"enabled"`
		} `json:"mods"`
	} `json:"profile"`
}

// LoadModSpecsFromFile reads ModSpec entries from a JSON or .smmprofile file if it exists.
func LoadModSpecsFromFile(path string) ([]ModSpec, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read mods file %s: %w", path, err)
	}

	var parsed modsConfigFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse mods file %s: %w", path, err)
	}

	var out []ModSpec
	for _, m := range parsed.Mods {
		if m.Ref == "" {
			continue
		}
		out = append(out, ModSpec{
			Ref:     m.Ref,
			Version: NormalizeVersion(m.Version),
			Enabled: true,
		})
	}

	if parsed.Profile != nil {
		var refs []string
		for ref := range parsed.Profile.Mods {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		for _, ref := range refs {
			entry := parsed.Profile.Mods[ref]
			if !entry.Enabled || strings.EqualFold(ref, "SML") || strings.EqualFold(ref, "bootstrapper") {
				continue
			}
			out = append(out, ModSpec{
				Ref:     ref,
				Version: NormalizeVersion(entry.Version),
				Enabled: true,
			})
		}
	}

	return out, nil
}

// MergeModSpecs merges two slices of ModSpec, with entries in override taking precedence.
func MergeModSpecs(base, override []ModSpec) []ModSpec {
	var merged []ModSpec
	index := make(map[string]int)

	for _, item := range append(append([]ModSpec{}, base...), override...) {
		if item.Ref == "" {
			continue
		}
		item.Version = NormalizeVersion(item.Version)
		if idx, ok := index[item.Ref]; ok {
			merged[idx] = item
		} else {
			index[item.Ref] = len(merged)
			merged = append(merged, item)
		}
	}
	return merged
}

// ficsitProfilesFile matches satisfactorymodding/ficsit-cli's profiles.json schema.
type ficsitProfilesFile struct {
	Profiles        map[string]ficsitProfile `json:"profiles"`
	SelectedProfile string                   `json:"selected_profile"`
	Version         int                      `json:"version"`
}

type ficsitProfile struct {
	Name            string                    `json:"name"`
	Mods            map[string]ficsitModEntry `json:"mods"`
	RequiredTargets []string                  `json:"required_targets"`
}

type ficsitModEntry struct {
	Version string `json:"version"`
	Enabled bool   `json:"enabled"`
}

// ficsitInstallationsFile matches satisfactorymodding/ficsit-cli's installations.json schema.
type ficsitInstallationsFile struct {
	SelectedInstallation string               `json:"selected_installation"`
	Installations        []ficsitInstallation `json:"installations"`
	Version              int                  `json:"version"`
}

type ficsitInstallation struct {
	Path    string `json:"path"`
	Profile string `json:"profile"`
	Vanilla bool   `json:"vanilla"`
}

// WriteFicsitState writes profiles.json and installations.json for ficsit-cli.
func WriteFicsitState(localDir, gamefilesDir, profileName string, vanilla bool, specs []ModSpec) error {
	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return fmt.Errorf("create ficsit local-dir: %w", err)
	}

	absGamefiles, err := filepath.Abs(gamefilesDir)
	if err != nil {
		return fmt.Errorf("resolve gamefiles path: %w", err)
	}

	if profileName == "" {
		profileName = "Default"
	}

	modMap := make(map[string]ficsitModEntry, len(specs))
	for _, s := range specs {
		if strings.EqualFold(s.Ref, "SML") {
			continue
		}
		modMap[s.Ref] = ficsitModEntry{
			Version: NormalizeVersion(s.Version),
			Enabled: s.Enabled,
		}
	}

	profilesMap := map[string]ficsitProfile{
		profileName: {
			Name:            profileName,
			Mods:            modMap,
			RequiredTargets: []string{"LinuxServer"},
		},
	}
	if profileName != "Default" {
		profilesMap["Default"] = ficsitProfile{
			Name: "Default",
			Mods: map[string]ficsitModEntry{},
		}
	}

	pFile := ficsitProfilesFile{
		Profiles:        profilesMap,
		SelectedProfile: profileName,
		Version:         0,
	}
	pBytes, err := json.MarshalIndent(pFile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profiles.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "profiles.json"), pBytes, 0o644); err != nil {
		return fmt.Errorf("write profiles.json: %w", err)
	}

	iFile := ficsitInstallationsFile{
		SelectedInstallation: absGamefiles,
		Installations: []ficsitInstallation{
			{
				Path:    absGamefiles,
				Profile: profileName,
				Vanilla: vanilla,
			},
		},
		Version: 0,
	}
	iBytes, err := json.MarshalIndent(iFile, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal installations.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "installations.json"), iBytes, 0o644); err != nil {
		return fmt.Errorf("write installations.json: %w", err)
	}

	return nil
}

// Reconcile synchronizes the desired mod profile using ficsit-cli and enforces the Strict World-Protection Gate.
func Reconcile(ctx context.Context, cfg ReconcileConfig) (*ReconcileResult, error) {
	if cfg.FicsitBin == "" {
		cfg.FicsitBin = "ficsit-cli"
	}
	if cfg.ProfileName == "" {
		cfg.ProfileName = "Default"
	}
	if cfg.UpdatePolicy == "" {
		cfg.UpdatePolicy = "always"
	}

	prevMods, _, _ := InspectInstalledMods(cfg.GamefilesDir, cfg.ProfileName)
	prevVersions := make(map[string]string, len(prevMods))
	for _, m := range prevMods {
		prevVersions[m.Ref] = m.Version
	}

	vanilla := !cfg.Enabled || len(cfg.Mods) == 0
	if err := WriteFicsitState(cfg.LocalDir, cfg.GamefilesDir, cfg.ProfileName, vanilla, cfg.Mods); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create ficsit cache-dir: %w", err)
	}

	modsDir := filepath.Join(cfg.GamefilesDir, "FactoryGame", "Mods")
	if !vanilla && strings.EqualFold(cfg.UpdatePolicy, "always") {
		for _, lp := range LockFileCandidates(modsDir, cfg.ProfileName) {
			_ = os.Remove(lp)
		}
	}

	cmd := exec.CommandContext(
		ctx,
		cfg.FicsitBin,
		"--local-dir", cfg.LocalDir,
		"--cache-dir", cfg.CacheDir,
		"--pretty=false",
		"apply",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ficsit-cli apply failed: %w", err)
	}

	if !vanilla {
		if err := VerifyInstallation(cfg.GamefilesDir, cfg.ProfileName, cfg.Mods); err != nil {
			return nil, fmt.Errorf("strict world-protection verification failed: %w", err)
		}
	}

	nowMods, _, _ := InspectInstalledMods(cfg.GamefilesDir, cfg.ProfileName)
	var changes []ModChange
	for _, m := range nowMods {
		oldV, existed := prevVersions[m.Ref]
		if !existed {
			changes = append(changes, ModChange{Ref: m.Ref, OldVersion: "", NewVersion: m.Version})
		} else if oldV != m.Version {
			changes = append(changes, ModChange{Ref: m.Ref, OldVersion: oldV, NewVersion: m.Version})
		}
	}

	return &ReconcileResult{
		Installed: nowMods,
		Changes:   changes,
	}, nil
}

// VerifyInstallation enforces the Strict World-Protection Gate:
// 1. The profile's lockfile must exist and be valid JSON.
// 2. SML.uplugin must exist on disk.
// 3. Every declared ModSpec must be present in the lockfile and have its .uplugin extracted on disk.
func VerifyInstallation(gamefilesDir, profileName string, expectedMods []ModSpec) error {
	modsDir := filepath.Join(gamefilesDir, "FactoryGame", "Mods")

	var lockBytes []byte
	var lockPath string
	for _, candidate := range LockFileCandidates(modsDir, profileName) {
		if b, err := os.ReadFile(candidate); err == nil {
			lockBytes = b
			lockPath = candidate
			break
		}
	}
	if len(lockBytes) == 0 {
		return fmt.Errorf("missing lockfile for profile %q in %s", profileName, modsDir)
	}

	var lockData struct {
		Mods map[string]struct {
			Version string `json:"version"`
		} `json:"mods"`
	}
	if err := json.Unmarshal(lockBytes, &lockData); err != nil {
		return fmt.Errorf("corrupt lockfile %s: %w", lockPath, err)
	}

	if len(expectedMods) > 0 {
		smlPlugin := findUPluginPath(modsDir, "SML")
		if smlPlugin == "" {
			return fmt.Errorf("Satisfactory Mod Loader (SML.uplugin) is missing from %s", modsDir)
		}
	}

	for _, reqMod := range expectedMods {
		if !reqMod.Enabled || strings.EqualFold(reqMod.Ref, "SML") {
			continue
		}
		if _, inLock := lockData.Mods[reqMod.Ref]; !inLock {
			return fmt.Errorf("declared mod %q (%s) is missing from resolved lockfile", reqMod.Ref, reqMod.Version)
		}
		if uplugin := findUPluginPath(modsDir, reqMod.Ref); uplugin == "" {
			return fmt.Errorf("declared mod %q is missing its .uplugin file on disk in %s", reqMod.Ref, modsDir)
		}
	}

	return nil
}

func findUPluginPath(modsDir, modRef string) string {
	candidates := []string{
		filepath.Join(modsDir, modRef, modRef+".uplugin"),
		filepath.Join(modsDir, "GameFeatures", modRef, modRef+".uplugin"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	// Fallback glob inside the mod folder in case .uplugin filename differs slightly in casing
	for _, dir := range []string{filepath.Join(modsDir, modRef), filepath.Join(modsDir, "GameFeatures", modRef)} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.uplugin"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}

var fallbackDescriptions = map[string]string{
	"NoFuel": "Removes fuel consumption requirements for generators and vehicles",
	"SML":    "Satisfactory Mod Loader (Core dependency)",
}

type upluginFile struct {
	FriendlyName string `json:"FriendlyName"`
	Description  string `json:"Description"`
	SemVersion   string `json:"SemVersion"`
	VersionName  string `json:"VersionName"`
}

// InspectInstalledMods scans FactoryGame/Mods for installed .uplugin files and lockfiles.
func InspectInstalledMods(gamefilesDir, profileName string) ([]InstalledMod, map[string]any, int) {
	modsDir := filepath.Join(gamefilesDir, "FactoryGame", "Mods")
	gameVersion, err := version.DetectChangelist(gamefilesDir)
	if err != nil || gameVersion == 0 {
		gameVersion = 517554
	}

	uplugins := make(map[string]InstalledMod)
	patterns := []string{
		filepath.Join(modsDir, "*", "*.uplugin"),
		filepath.Join(modsDir, "GameFeatures", "*", "*.uplugin"),
	}
	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, path := range matches {
			modRef := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			var up upluginFile
			if err := json.Unmarshal(data, &up); err != nil {
				continue
			}
			ver := up.SemVersion
			if ver == "" {
				ver = up.VersionName
			}
			if ver == "" {
				ver = "0.0.0"
			}
			desc := strings.TrimSpace(up.Description)
			if desc == "" {
				desc = fallbackDescriptions[modRef]
			}
			name := strings.TrimSpace(up.FriendlyName)
			if name == "" {
				name = modRef
			}
			uplugins[modRef] = InstalledMod{
				Ref:         modRef,
				Name:        name,
				Version:     ver,
				Description: desc,
				FicsitURL:   "https://ficsit.app/mod/" + modRef,
				IsSML:       modRef == "SML",
			}
		}
	}

	var activeLockfile map[string]any
	lockCandidates := LockFileCandidates(modsDir, profileName)
	if globLocks, err := filepath.Glob(filepath.Join(modsDir, "*-lock.json")); err == nil {
		for _, gl := range globLocks {
			if filepath.Base(gl) != "default-lock.json" {
				lockCandidates = append(lockCandidates, gl)
			}
		}
	}

	for _, lp := range lockCandidates {
		data, err := os.ReadFile(lp)
		if err != nil {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err == nil {
			if modsMap, ok := parsed["mods"].(map[string]any); ok && len(modsMap) > 0 {
				activeLockfile = parsed
				break
			}
		}
	}

	modsByRef := make(map[string]InstalledMod)
	if activeLockfile != nil {
		if modsMap, ok := activeLockfile["mods"].(map[string]any); ok {
			for modRef, rawEntry := range modsMap {
				up, hasUp := uplugins[modRef]
				ver := "0.0.0"
				if entryMap, ok := rawEntry.(map[string]any); ok {
					if vStr, ok := entryMap["version"].(string); ok && vStr != "" {
						ver = vStr
					}
					if hasUp && up.Version != "" {
						ver = up.Version
						entryMap["version"] = ver
					}
				}
				name := modRef
				desc := fallbackDescriptions[modRef]
				if hasUp {
					name = up.Name
					desc = up.Description
				}
				modsByRef[modRef] = InstalledMod{
					Ref:         modRef,
					Name:        name,
					Version:     ver,
					Description: desc,
					FicsitURL:   "https://ficsit.app/mod/" + modRef,
					IsSML:       modRef == "SML",
				}
			}
		}
	}

	for modRef, up := range uplugins {
		if _, exists := modsByRef[modRef]; !exists {
			modsByRef[modRef] = up
		}
	}

	list := make([]InstalledMod, 0, len(modsByRef))
	for _, m := range modsByRef {
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].IsSML != list[j].IsSML {
			return !list[i].IsSML
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})

	return list, activeLockfile, gameVersion
}

// BuildSMMProfile constructs the downloadable .smmprofile JSON object for Desktop SMM clients.
func BuildSMMProfile(gamefilesDir, profileName string, clientOnlyMods []ModSpec) map[string]any {
	if profileName == "" {
		profileName = "UNSC-Section-III"
	}
	modsList, activeLockfile, gameVersion := InspectInstalledMods(gamefilesDir, profileName)

	profileMods := make(map[string]any)
	for _, m := range modsList {
		if m.IsSML {
			continue
		}
		profileMods[m.Ref] = map[string]any{
			"version": ">=" + m.Version,
			"enabled": true,
		}
	}

	for _, cm := range clientOnlyMods {
		if cm.Ref == "" || strings.EqualFold(cm.Ref, "SML") {
			continue
		}
		profileMods[cm.Ref] = map[string]any{
			"version": NormalizeVersion(cm.Version),
			"enabled": true,
		}
	}

	profileObj := map[string]any{
		"profile": map[string]any{
			"mods":             profileMods,
			"name":             profileName,
			"required_targets": nil,
		},
		"metadata": map[string]any{
			"gameVersion": gameVersion,
		},
	}
	if activeLockfile != nil {
		profileObj["lockfile"] = activeLockfile
	}
	return profileObj
}
