package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/binaryn3xus/satisfactory/internal/api"
	"github.com/binaryn3xus/satisfactory/internal/blueprints"
	"github.com/binaryn3xus/satisfactory/internal/mods"
	"github.com/binaryn3xus/satisfactory/internal/players"
	"github.com/binaryn3xus/satisfactory/internal/saves"
)

type PortalConfig struct {
	Port           string
	ServerName     string
	ProfileName    string
	SavesDir       string
	BlueprintsDir  string
	GamefilesDir   string
	LogFile        string
	ServerAPIURL   string
	ServerAPIToken string
	FRMAPIURL      string
	MaxPlayers     int
	ClientOnlyMods []mods.ModSpec
	AdminPassword  string
}

type PortalServer struct {
	cfg       PortalConfig
	apiClient *api.Client
}

const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
  <defs>
    <linearGradient id="cog" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#fa8029"/>
      <stop offset="100%" stop-color="#e06c19"/>
    </linearGradient>
    <linearGradient id="ficsit-blue" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#38bdf8"/>
      <stop offset="100%" stop-color="#0284c7"/>
    </linearGradient>
  </defs>
  <rect width="64" height="64" rx="14" fill="#0f141c"/>
  <rect x="2" y="2" width="60" height="60" rx="12" fill="none" stroke="#fa8029" stroke-width="2" stroke-opacity="0.35"/>
  <g fill="url(#cog)" transform="translate(32,32)">
    <circle r="16"/>
    <rect x="-4.5" y="-23" width="9" height="9" rx="1.5"/>
    <rect x="-4.5" y="-23" width="9" height="9" rx="1.5" transform="rotate(45)"/>
    <rect x="-4.5" y="-23" width="9" height="9" rx="1.5" transform="rotate(90)"/>
    <rect x="-4.5" y="-23" width="9" height="9" rx="1.5" transform="rotate(135)"/>
    <rect x="-4.5" y="-23" width="9" height="9" rx="1.5" transform="rotate(180)"/>
    <rect x="-4.5" y="-23" width="9" height="9" rx="1.5" transform="rotate(225)"/>
    <rect x="-4.5" y="-23" width="9" height="9" rx="1.5" transform="rotate(270)"/>
    <rect x="-4.5" y="-23" width="9" height="9" rx="1.5" transform="rotate(315)"/>
    <circle r="11" fill="#0f141c"/>
  </g>
  <!-- FICSIT Blue Checkmark (Isometric Chamfered Chevron) -->
  <polygon points="20.5,31.5 28.5,39.5 43,24 39,20 28.5,31.5 24.5,27.5" fill="url(#ficsit-blue)"/>
</svg>`

//go:embed assets/inspector.bundle.js
var inspectorBundleJS []byte

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := loadConfigFromEnv()
	srv := NewPortalServer(cfg)

	addr := "0.0.0.0:" + cfg.Port
	slog.Info("starting FICSIT Save, Mod & Blueprint Portal",
		slog.String("listen", addr),
		slog.String("saves_dir", cfg.SavesDir),
		slog.String("blueprints_dir", cfg.BlueprintsDir),
		slog.String("gamefiles_dir", cfg.GamefilesDir),
		slog.String("profile_name", cfg.ProfileName),
	)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("portal server failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func loadConfigFromEnv() PortalConfig {
	gamefilesDir := envOr("GAMEFILES_DIR", "/config/gamefiles")
	if envModsDir := strings.TrimSpace(os.Getenv("MODS_DIR")); envModsDir != "" && os.Getenv("GAMEFILES_DIR") == "" {
		// Derive gamefilesDir if only legacy MODS_DIR (/config/gamefiles/FactoryGame/Mods) was set
		gamefilesDir = filepath.Dir(filepath.Dir(envModsDir))
	}

	profileName := envOr("PORTAL_PROFILE_NAME", envOr("SMM_PROFILE_NAME", envOr("MODS_PROFILE", "default")))
	serverName := envOr("PORTAL_TITLE", envOr("SERVER_NAME", profileName))
	logFile := envOr("LOG_FILE", filepath.Join(gamefilesDir, "FactoryGame/Saved/Logs/FactoryGame.log"))
	gamePort := envOr("SERVER_GAME_PORT", "7777")
	serverAPIURL := envOr("SERVER_API_URL", "https://127.0.0.1:"+gamePort)

	maxPlayers, err := strconv.Atoi(envOr("MAX_PLAYERS", "8"))
	if err != nil || maxPlayers <= 0 {
		maxPlayers = 8
	}

	clientOnlySpecs, _ := mods.ParseModList(os.Getenv("MODS_CLIENT_ONLY"))

	return PortalConfig{
		Port:           envOr("PORTAL_PORT", envOr("PORT", "8081")),
		ServerName:     serverName,
		ProfileName:    profileName,
		SavesDir:       envOr("SAVES_DIR", "/config/saved/server"),
		BlueprintsDir:  envOr("BLUEPRINTS_DIR", "/config/saved/blueprints"),
		GamefilesDir:   gamefilesDir,
		LogFile:        logFile,
		ServerAPIURL:   serverAPIURL,
		ServerAPIToken: os.Getenv("SERVER_API_TOKEN"),
		FRMAPIURL:      os.Getenv("FRM_API_URL"),
		MaxPlayers:     maxPlayers,
		ClientOnlyMods: clientOnlySpecs,
		AdminPassword:  envOr("PORTAL_ADMIN_PASSWORD", envOr("ADMIN_PASSWORD", "")),
	}
}

func NewPortalServer(cfg PortalConfig) *PortalServer {
	var apiClient *api.Client
	if cfg.ServerAPIURL != "" {
		apiClient = api.NewClient(cfg.ServerAPIURL, cfg.ServerAPIToken)
	}
	return &PortalServer{
		cfg:       cfg,
		apiClient: apiClient,
	}
}

func (s *PortalServer) Routes() http.Handler {
	return http.HandlerFunc(s.handleRequest)
}

func setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, Content-Disposition")
}

func (s *PortalServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sendBody := r.Method == http.MethodGet
	path := r.URL.Path

	switch path {
	case "/admin":
		s.handleAdmin(w, r, sendBody)
		return

	case "/admin/login":
		s.handleAdminLogin(w, r)
		return

	case "/admin/logout":
		s.handleAdminLogout(w, r)
		return

	case "/admin/api/upload-save":
		s.handleAdminUploadSave(w, r)
		return

	case "/admin/api/load-game":
		s.handleAdminLoadGame(w, r)
		return

	case "/healthz", "/health":
		s.writeBytes(w, http.StatusOK, "text/plain; charset=utf-8", []byte("OK\n"), sendBody)
		return

	case "/favicon.ico", "/favicon.svg":
		w.Header().Set("Cache-Control", "public, max-age=86400")
		s.writeBytes(w, http.StatusOK, "image/svg+xml", []byte(faviconSVG), sendBody)
		return

	case "/inspector.js", "/inspector-v2.js", "/inspector-v3.js", "/inspector-v4.js":
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		s.writeBytes(w, http.StatusOK, "application/javascript; charset=utf-8", inspectorBundleJS, sendBody)
		return

	case "/metrics":
		s.handleMetrics(w, r, sendBody)
		return

	case "/status.json":
		s.handleStatusJSON(w, r, sendBody)
		return

	case "/mods.json":
		s.handleModsJSON(w, sendBody)
		return

	case "/" + s.cfg.ProfileName + ".smmprofile", "/mods.smmprofile", "/profile.smmprofile":
		s.handleSMMProfile(w, sendBody)
		return

	case "/blueprints.json":
		s.handleBlueprintsJSON(w, sendBody)
		return

	case "/blueprints.zip":
		s.handleAllBlueprintsZip(w, sendBody)
		return
	}

	if strings.HasPrefix(path, "/blueprints/") && strings.HasSuffix(path, ".zip") {
		s.handleSingleBlueprintZip(w, path, sendBody)
		return
	}

	saveList, _ := saves.ListSaves(s.cfg.SavesDir)

	if path == "/latest.sav" || path == "/latest" {
		if len(saveList) == 0 {
			http.Error(w, "No save files found on server", http.StatusNotFound)
			return
		}
		s.serveDiskFile(w, saveList[0].Path, saveList[0].Name, sendBody)
		return
	}

	if path == "/" || path == "/index.html" {
		s.handleDashboard(w, r, saveList, sendBody)
		return
	}

	reqFilename := strings.TrimPrefix(path, "/")
	if reqFilename != "" && !strings.HasSuffix(strings.ToLower(reqFilename), ".sav") {
		reqFilename += ".sav"
	}
	for _, sv := range saveList {
		if sv.Name == reqFilename {
			s.serveDiskFile(w, sv.Path, sv.Name, sendBody)
			return
		}
	}

	http.Error(w, fmt.Sprintf("File not found: %s", path), http.StatusNotFound)
}

func (s *PortalServer) writeBytes(w http.ResponseWriter, status int, contentType string, body []byte, sendBody bool) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if sendBody {
		_, _ = w.Write(body)
	}
}

func (s *PortalServer) writeJSON(w http.ResponseWriter, status int, payload any, sendBody bool) {
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		http.Error(w, "JSON serialization error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	s.writeBytes(w, status, "application/json; charset=utf-8", body, sendBody)
}

func (s *PortalServer) handleStatusJSON(w http.ResponseWriter, r *http.Request, sendBody bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	snap := players.CollectSnapshot(ctx, s.apiClient, s.cfg.LogFile, s.cfg.FRMAPIURL, s.cfg.MaxPlayers)
	s.writeJSON(w, http.StatusOK, snap, sendBody)
}

func (s *PortalServer) handleModsJSON(w http.ResponseWriter, sendBody bool) {
	modsList, _, gameVersion := mods.InspectInstalledMods(s.cfg.GamefilesDir, s.cfg.ProfileName)
	payload := map[string]any{
		"profile_name":   s.cfg.ProfileName,
		"game_version":   gameVersion,
		"smmprofile_url": "/" + s.cfg.ProfileName + ".smmprofile",
		"mods":           modsList,
	}
	s.writeJSON(w, http.StatusOK, payload, sendBody)
}

func (s *PortalServer) handleSMMProfile(w http.ResponseWriter, sendBody bool) {
	profileObj := mods.BuildSMMProfile(s.cfg.GamefilesDir, s.cfg.ProfileName, s.cfg.ClientOnlyMods)
	body, err := json.MarshalIndent(profileObj, "", "  ")
	if err != nil {
		http.Error(w, "Failed to build SMM profile", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.smmprofile"`, s.cfg.ProfileName))
	w.Header().Set("Cache-Control", "no-cache")
	s.writeBytes(w, http.StatusOK, "application/json; charset=utf-8", body, sendBody)
}

func (s *PortalServer) handleBlueprintsJSON(w http.ResponseWriter, sendBody bool) {
	bpList, _ := blueprints.ListBlueprints(s.cfg.BlueprintsDir)
	payload := map[string]any{
		"count":      len(bpList),
		"zip_url":    "/blueprints.zip",
		"blueprints": bpList,
	}
	s.writeJSON(w, http.StatusOK, payload, sendBody)
}

func (s *PortalServer) handleAllBlueprintsZip(w http.ResponseWriter, sendBody bool) {
	bpList, _ := blueprints.ListBlueprints(s.cfg.BlueprintsDir)
	if len(bpList) == 0 {
		http.Error(w, "No blueprints found on server", http.StatusNotFound)
		return
	}
	filename := fmt.Sprintf("%s-blueprints.zip", s.cfg.ProfileName)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	if sendBody {
		_ = blueprints.WriteAllBlueprintsZip(w, bpList)
	}
}

func (s *PortalServer) handleSingleBlueprintZip(w http.ResponseWriter, path string, sendBody bool) {
	trimmed := strings.TrimPrefix(path, "/blueprints/")
	trimmed = strings.TrimSuffix(trimmed, ".zip")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "Invalid blueprint path", http.StatusBadRequest)
		return
	}
	sessionReq, _ := url.PathUnescape(parts[0])
	nameReq, _ := url.PathUnescape(parts[1])

	bpList, _ := blueprints.ListBlueprints(s.cfg.BlueprintsDir)
	for _, bp := range bpList {
		if bp.SessionName == sessionReq && bp.Name == nameReq {
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, bp.Name))
			w.WriteHeader(http.StatusOK)
			if sendBody {
				_ = blueprints.WriteSingleBlueprintZip(w, bp)
			}
			return
		}
	}
	http.Error(w, "Blueprint not found", http.StatusNotFound)
}

func (s *PortalServer) handleMetrics(w http.ResponseWriter, r *http.Request, sendBody bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	snap := players.CollectSnapshot(ctx, s.apiClient, s.cfg.LogFile, s.cfg.FRMAPIURL, s.cfg.MaxPlayers)
	modsList, _, gameVersion := mods.InspectInstalledMods(s.cfg.GamefilesDir, s.cfg.ProfileName)
	saveList, _ := saves.ListSaves(s.cfg.SavesDir)
	bpList, _ := blueprints.ListBlueprints(s.cfg.BlueprintsDir)

	upVal := 0
	if snap.ServerOnline {
		upVal = 1
	}
	var latestSize int64
	var latestPlaytime int32
	if len(saveList) > 0 {
		latestSize = saveList[0].Size
		latestPlaytime = saveList[0].PlayDurationSeconds
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# HELP satisfactory_server_up Whether the Satisfactory server HTTPS API is reachable.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_server_up gauge\n")
	fmt.Fprintf(&b, "satisfactory_server_up %d\n", upVal)

	fmt.Fprintf(&b, "# HELP satisfactory_build_changelist Installed Satisfactory server build changelist.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_build_changelist gauge\n")
	fmt.Fprintf(&b, "satisfactory_build_changelist %d\n", gameVersion)

	fmt.Fprintf(&b, "# HELP satisfactory_tick_rate Average server simulation tick rate (TPS).\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_tick_rate gauge\n")
	fmt.Fprintf(&b, "satisfactory_tick_rate %.2f\n", snap.AverageTickRate)

	fmt.Fprintf(&b, "# HELP satisfactory_players_online Number of currently connected pioneers.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_players_online gauge\n")
	fmt.Fprintf(&b, "satisfactory_players_online %d\n", snap.NumConnectedPlayers)

	fmt.Fprintf(&b, "# HELP satisfactory_player_limit Maximum player capacity configured on the server.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_player_limit gauge\n")
	fmt.Fprintf(&b, "satisfactory_player_limit %d\n", snap.PlayerLimit)

	fmt.Fprintf(&b, "# HELP satisfactory_tech_tier Highest unlocked HUB tech tier.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_tech_tier gauge\n")
	fmt.Fprintf(&b, "satisfactory_tech_tier %d\n", snap.TechTier)

	fmt.Fprintf(&b, "# HELP satisfactory_mods_installed_total Total number of installed server mods (including SML).\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_mods_installed_total gauge\n")
	fmt.Fprintf(&b, "satisfactory_mods_installed_total %d\n", len(modsList))

	fmt.Fprintf(&b, "# HELP satisfactory_saves_total Number of world save files available.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_saves_total gauge\n")
	fmt.Fprintf(&b, "satisfactory_saves_total %d\n", len(saveList))

	fmt.Fprintf(&b, "# HELP satisfactory_latest_save_size_bytes Size of the most recent world save file in bytes.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_latest_save_size_bytes gauge\n")
	fmt.Fprintf(&b, "satisfactory_latest_save_size_bytes %d\n", latestSize)

	fmt.Fprintf(&b, "# HELP satisfactory_latest_save_playtime_seconds Total play duration recorded in the most recent world save.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_latest_save_playtime_seconds gauge\n")
	fmt.Fprintf(&b, "satisfactory_latest_save_playtime_seconds %d\n", latestPlaytime)

	fmt.Fprintf(&b, "# HELP satisfactory_blueprints_total Total number of server blueprints available.\n")
	fmt.Fprintf(&b, "# TYPE satisfactory_blueprints_total gauge\n")
	fmt.Fprintf(&b, "satisfactory_blueprints_total %d\n", len(bpList))

	if snap.Power != nil {
		fmt.Fprintf(&b, "# HELP satisfactory_power_production_mw Total power production in MW (via FRM).\n")
		fmt.Fprintf(&b, "# TYPE satisfactory_power_production_mw gauge\n")
		fmt.Fprintf(&b, "satisfactory_power_production_mw %.2f\n", snap.Power.TotalProductionMW)

		fmt.Fprintf(&b, "# HELP satisfactory_power_consumption_mw Total power consumption in MW (via FRM).\n")
		fmt.Fprintf(&b, "# TYPE satisfactory_power_consumption_mw gauge\n")
		fmt.Fprintf(&b, "satisfactory_power_consumption_mw %.2f\n", snap.Power.TotalConsumptionMW)
	}

	s.writeBytes(w, http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(b.String()), sendBody)
}

func (s *PortalServer) serveDiskFile(w http.ResponseWriter, filePath, downloadName string, sendBody bool) {
	f, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Error opening file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Error reading file metadata", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, downloadName))
	w.Header().Set("Last-Modified", stat.ModTime().UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)

	if sendBody {
		_, _ = io.Copy(w, f)
	}
}

func isCustomMap(mapName string) bool {
	if mapName == "" {
		return false
	}
	m := strings.ToLower(mapName)
	if m == "persistent_level" || strings.Contains(m, "gamelevel01") {
		return false
	}
	return true
}

func scimButtonHTML(scimURL, mapName, label string) string {
	if isCustomMap(mapName) {
		title := fmt.Sprintf("⚠️ Custom Map Detected (%s): SCIM only officially supports the default planetary map. Custom terrain and modded actors may fail to load or crash.", html.EscapeString(mapName))
		return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="btn btn-map btn-map-warn" title="%s">⚠️ %s</a>`, html.EscapeString(scimURL), title, label)
	}
	return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer" class="btn btn-map" title="Open save in Satisfactory-Calculator Interactive Map">%s</a>`, html.EscapeString(scimURL), label)
}

func (s *PortalServer) handleDashboard(w http.ResponseWriter, r *http.Request, saveList []saves.SaveFile, sendBody bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	host := r.Host
	if host == "" {
		host = "localhost:" + s.cfg.Port
	}
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", proto, host)

	snap := players.CollectSnapshot(ctx, s.apiClient, s.cfg.LogFile, s.cfg.FRMAPIURL, s.cfg.MaxPlayers)
	modsList, _, gameVersion := mods.InspectInstalledMods(s.cfg.GamefilesDir, s.cfg.ProfileName)
	bpList, _ := blueprints.ListBlueprints(s.cfg.BlueprintsDir)

	if len(saveList) > 0 && saveList[0].BuildVersion > 0 && gameVersion == 517554 {
		gameVersion = int(saveList[0].BuildVersion)
	}

	// Build Pioneer Badges
	var pioneerHTML strings.Builder
	if len(snap.OnlinePioneers) > 0 {
		for _, p := range snap.OnlinePioneers {
			extra := ""
			if p.Ping > 0 {
				extra = fmt.Sprintf(" • %dms", p.Ping)
			}
			fmt.Fprintf(&pioneerHTML, `<span class="pioneer-pill online">🟢 %s%s</span>`, html.EscapeString(p.Name), extra)
		}
	} else {
		pioneerHTML.WriteString(`<span class="text-muted">No pioneers currently connected</span>`)
	}

	var recentHTML strings.Builder
	for _, p := range snap.RecentPioneers {
		when := ""
		if p.LastEvent != "" {
			when = fmt.Sprintf(" (%s)", html.EscapeString(p.LastEvent))
		}
		fmt.Fprintf(&recentHTML, `<span class="pioneer-pill recent">⚪ %s%s</span>`, html.EscapeString(p.Name), when)
	}

	statusClass := "status-offline"
	if snap.ServerOnline {
		if snap.IsGamePaused {
			statusClass = "status-paused"
		} else {
			statusClass = "status-online"
		}
	}

	tpsDisplay := "—"
	if snap.AverageTickRate > 0 {
		tpsDisplay = fmt.Sprintf("%.1f TPS", snap.AverageTickRate)
	}
	sessionDisplay := snap.ActiveSessionName
	if sessionDisplay == "" && len(saveList) > 0 {
		sessionDisplay = saveList[0].SessionName
	}
	if sessionDisplay == "" {
		sessionDisplay = s.cfg.ServerName
	}

	// Mod Section (only rendered if mods or client-only mods are present)
	var modCardHTML, modTableHTML string
	if len(modsList) > 0 || len(s.cfg.ClientOnlyMods) > 0 {
		escProfile := html.EscapeString(s.cfg.ProfileName)
		modCardHTML = fmt.Sprintf(`
		<div class="mod-card">
			<div class="latest-info">
				<h2>🧩 One-Click Server Mod Profile (%s.smmprofile)</h2>
				<p>Live-synced with the server's active lockfile. Download and import into <strong>Satisfactory Mod Manager</strong> (Profiles &rarr; Import) to match exact server versions.</p>
			</div>
			<div class="btn-group">
				<a href="/%s.smmprofile" class="btn btn-primary">⬇ Download SMM Profile</a>
				<a href="/mods.json" target="_blank" rel="noopener noreferrer" class="btn btn-secondary">Mods JSON</a>
			</div>
		</div>`, escProfile, url.PathEscape(s.cfg.ProfileName))

		var modRows strings.Builder
		for _, m := range modsList {
			roleBadge := ""
			if m.IsSML {
				roleBadge = `<span class="version-badge sml-badge">Core Loader</span>`
			}
			desc := m.Description
			if desc == "" {
				desc = "Required server mod"
			}
			fmt.Fprintf(&modRows, `
			<tr>
				<td class="name">
					<span class="icon">🧩</span>
					<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>
					<span class="mod-ref">(%s)</span>
					%s
				</td>
				<td class="version"><span class="version-badge">v%s</span></td>
				<td class="time">%s</td>
				<td class="actions">
					<a href="%s" target="_blank" rel="noopener noreferrer" class="btn btn-secondary">Ficsit.app ↗</a>
				</td>
			</tr>`,
				html.EscapeString(m.FicsitURL),
				html.EscapeString(m.Name),
				html.EscapeString(m.Ref),
				roleBadge,
				html.EscapeString(m.Version),
				html.EscapeString(desc),
				html.EscapeString(m.FicsitURL),
			)
		}

		modTableHTML = fmt.Sprintf(`
		<h2 class="section-title">
			<span>🧩 Installed Server Mods (Live)</span>
			<span class="badge">%d Mods Loaded</span>
		</h2>
		<table>
			<thead>
				<tr>
					<th>Mod Name</th>
					<th>Server Version</th>
					<th>Description</th>
					<th style="text-align: right;">Link</th>
				</tr>
			</thead>
			<tbody>%s</tbody>
		</table>`, len(modsList), modRows.String())
	}

	// Latest Save Hero Card & Saves Table
	var latestCardHTML string
	var saveRows strings.Builder
	if len(saveList) > 0 {
		latest := saveList[0]
		latestURL := baseURL + "/latest.sav"
		scimLatestURL := "https://satisfactory-calculator.com/interactive-map?url=" + url.QueryEscape(latestURL)
		playMeta := ""
		if latest.FormattedPlaytime != "" {
			playMeta = fmt.Sprintf(" &bull; Playtime: <strong>%s</strong>", html.EscapeString(latest.FormattedPlaytime))
		}
		latestCardHTML = fmt.Sprintf(`
		<div class="latest-card">
			<div class="latest-info">
				<h2>💾 Latest Save: %s</h2>
				<p>Session: <strong>%s</strong>%s &bull; Modified: %s &bull; Size: %s</p>
			</div>
			<div class="btn-group">
				<a href="/latest.sav" class="btn btn-primary">⬇ Download Latest Save</a>
				<button type="button" onclick="openSaveInspector('/latest.sav', '%s')" class="btn btn-secondary">🔍 Inspect</button>
				%s
			</div>
		</div>`,
			html.EscapeString(latest.Name),
			html.EscapeString(latest.SessionName),
			playMeta,
			html.EscapeString(latest.FormattedTime),
			html.EscapeString(latest.FormattedSize),
			html.EscapeString(latest.Name),
			scimButtonHTML(scimLatestURL, latest.MapName, "Open in SCIM ↗"),
		)

		for _, sv := range saveList {
			escapedFile := url.PathEscape(sv.Name)
			saveURL := baseURL + "/" + escapedFile
			scimURL := "https://satisfactory-calculator.com/interactive-map?url=" + url.QueryEscape(saveURL)
			playBadge := "—"
			if sv.FormattedPlaytime != "" {
				playBadge = fmt.Sprintf(`<span class="version-badge">%s</span>`, html.EscapeString(sv.FormattedPlaytime))
			}
			fmt.Fprintf(&saveRows, `
			<tr>
				<td class="name">
					<span class="icon">💾</span>
					<a href="/%s">%s</a>
					<span class="mod-ref">(%s)</span>
				</td>
				<td>%s</td>
				<td class="time">%s</td>
				<td class="size">%s</td>
				<td class="actions">
					<a href="/%s" class="btn btn-secondary">Download</a>
					<button type="button" onclick="openSaveInspector('/%s', '%s')" class="btn btn-secondary">🔍 Inspect</button>
					%s
				</td>
			</tr>`,
				escapedFile,
				html.EscapeString(sv.Name),
				html.EscapeString(sv.SessionName),
				playBadge,
				html.EscapeString(sv.FormattedTime),
				html.EscapeString(sv.FormattedSize),
				escapedFile,
				escapedFile,
				html.EscapeString(sv.Name),
				scimButtonHTML(scimURL, sv.MapName, "SCIM Map ↗"),
			)
		}
	} else {
		latestCardHTML = `<div class="latest-card"><div class="latest-info"><h2>No World Saves Found Yet</h2><p>Saves will appear here automatically once a session is created.</p></div></div>`
		saveRows.WriteString(`<tr><td colspan="5" style="text-align:center;">No saves available</td></tr>`)
	}

	// Blueprints Section
	var bpSectionHTML string
	if len(bpList) > 0 {
		var bpRows strings.Builder
		for _, bp := range bpList {
			zipPath := fmt.Sprintf("/blueprints/%s/%s.zip", url.PathEscape(bp.SessionName), url.PathEscape(bp.Name))
			desc := bp.Description
			if desc == "" {
				desc = "FICSIT Custom Blueprint"
			}
			fmt.Fprintf(&bpRows, `
			<tr>
				<td class="name">
					<span class="icon">📐</span>
					<a href="%s">%s</a>
					<span class="version-badge sml-badge">%s</span>
				</td>
				<td class="time">%s</td>
				<td class="size">%s</td>
				<td class="actions">
					<a href="%s" class="btn btn-secondary">⬇ Download (.zip)</a>
				</td>
			</tr>`,
				zipPath,
				html.EscapeString(bp.Name),
				html.EscapeString(bp.SessionName),
				html.EscapeString(desc),
				html.EscapeString(bp.FormattedSize),
				zipPath,
			)
		}
		bpSectionHTML = fmt.Sprintf(`
		<h2 class="section-title">
			<span>📐 Server Blueprints (%d Available)</span>
			<a href="/blueprints.zip" class="btn btn-secondary">⬇ Download All Blueprints (.zip)</a>
		</h2>
		<table>
			<thead>
				<tr>
					<th>Blueprint Name</th>
					<th>Description</th>
					<th>Size</th>
					<th style="text-align: right;">Actions</th>
				</tr>
			</thead>
			<tbody>%s</tbody>
		</table>`, len(bpList), bpRows.String())
	}

	pageHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<link rel="icon" type="image/svg+xml" href="/favicon.svg">
	<link rel="alternate icon" href="/favicon.ico">
	<title>%s • FICSIT Save, Mod &amp; Blueprint Portal</title>
	<script src="/inspector-v4.js" defer></script>
	<style>
		:root {
			--bg-base: #0f141c;
			--bg-surface: #1a2230;
			--bg-hover: #222d40;
			--accent-primary: #fa8029;
			--accent-hover: #e06c19;
			--accent-blue: #3b82f6;
			--text-main: #f3f4f6;
			--text-muted: #9ca3af;
			--border: #2d3748;
		}
		body {
			font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
			background-color: var(--bg-base);
			color: var(--text-main);
			margin: 0;
			padding: 2rem;
			display: flex;
			justify-content: center;
		}
		.container { width: 100%%; max-width: 980px; }
		header {
			margin-bottom: 1.5rem;
			border-bottom: 1px solid var(--border);
			padding-bottom: 1.25rem;
			display: flex;
			justify-content: space-between;
			align-items: center;
			flex-wrap: wrap;
			gap: 0.75rem;
		}
		h1 { margin: 0; font-size: 1.65rem; display: flex; align-items: center; gap: 0.5rem; }
		.section-title {
			margin: 2rem 0 0.75rem 0;
			font-size: 1.15rem;
			color: var(--text-main);
			display: flex;
			justify-content: space-between;
			align-items: center;
			flex-wrap: wrap;
			gap: 0.5rem;
		}
		.badge {
			background: #374151;
			font-size: 0.8rem;
			padding: 0.25rem 0.55rem;
			border-radius: 4px;
			color: var(--text-muted);
		}
		.status-badge {
			font-size: 0.8rem;
			font-weight: 600;
			padding: 0.25rem 0.65rem;
			border-radius: 9999px;
		}
		.status-online { background: rgba(46, 204, 113, 0.15); border: 1px solid rgba(46, 204, 113, 0.5); color: #2ecc71; }
		.status-paused { background: rgba(250, 128, 41, 0.15); border: 1px solid rgba(250, 128, 41, 0.5); color: var(--accent-primary); }
		.status-offline { background: rgba(156, 163, 175, 0.15); border: 1px solid rgba(156, 163, 175, 0.4); color: var(--text-muted); }
		.telemetry-bar {
			background: var(--bg-surface);
			border: 1px solid var(--border);
			border-radius: 8px;
			padding: 1rem 1.25rem;
			margin-bottom: 1.25rem;
			display: flex;
			flex-direction: column;
			gap: 0.75rem;
		}
		.telemetry-row {
			display: flex;
			justify-content: space-between;
			align-items: center;
			flex-wrap: wrap;
			gap: 0.75rem;
		}
		.pioneer-list { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; font-size: 0.88rem; }
		.pioneer-pill {
			padding: 0.2rem 0.6rem;
			border-radius: 9999px;
			font-size: 0.82rem;
			background: #131a24;
			border: 1px solid var(--border);
		}
		.pioneer-pill.online { border-color: rgba(46, 204, 113, 0.5); color: #e5e7eb; }
		.pioneer-pill.recent { color: var(--text-muted); }
		.text-muted { color: var(--text-muted); font-size: 0.85rem; }
		.version-badge {
			background: rgba(250, 128, 41, 0.15);
			border: 1px solid rgba(250, 128, 41, 0.4);
			color: var(--accent-primary);
			font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
			font-size: 0.8rem;
			padding: 0.15rem 0.45rem;
			border-radius: 4px;
			white-space: nowrap;
		}
		.sml-badge {
			background: rgba(59, 130, 246, 0.15);
			border-color: rgba(59, 130, 246, 0.4);
			color: #60a5fa;
			margin-left: 0.35rem;
		}
		.mod-ref { color: var(--text-muted); font-size: 0.8rem; margin-left: 0.25rem; }
		.latest-card {
			background: linear-gradient(135deg, rgba(250, 128, 41, 0.15) 0%%, rgba(26, 34, 48, 0.8) 100%%);
			border: 1px solid var(--accent-primary);
			border-radius: 8px;
			padding: 1.4rem;
			margin-bottom: 1.25rem;
			display: flex;
			justify-content: space-between;
			align-items: center;
			gap: 1rem;
			flex-wrap: wrap;
		}
		.mod-card {
			background: linear-gradient(135deg, rgba(59, 130, 246, 0.15) 0%%, rgba(26, 34, 48, 0.85) 100%%);
			border: 1px solid rgba(59, 130, 246, 0.5);
			border-radius: 8px;
			padding: 1.4rem;
			margin-bottom: 1.25rem;
			display: flex;
			justify-content: space-between;
			align-items: center;
			gap: 1rem;
			flex-wrap: wrap;
		}
		.latest-info h2 { margin: 0 0 0.25rem 0; font-size: 1.15rem; color: var(--accent-primary); }
		.mod-card .latest-info h2 { color: #60a5fa; }
		.latest-info p { margin: 0; color: var(--text-muted); font-size: 0.9rem; }
		.btn-group { display: flex; gap: 0.5rem; flex-wrap: wrap; }
		.btn {
			display: inline-flex;
			align-items: center;
			gap: 0.35rem;
			padding: 0.5rem 0.95rem;
			border-radius: 6px;
			text-decoration: none;
			font-weight: 500;
			font-size: 0.88rem;
			white-space: nowrap;
		}
		.btn-primary { background-color: var(--accent-primary); color: #ffffff; }
		.btn-primary:hover { background-color: var(--accent-hover); }
		.btn-secondary { background-color: #374151; color: #ffffff; }
		.btn-secondary:hover { background-color: #4b5563; }
		.btn-map {
			background-color: rgba(59, 130, 246, 0.15);
			border: 1px solid rgba(59, 130, 246, 0.4);
			color: #60a5fa;
		}
		.btn-map:hover { background-color: rgba(59, 130, 246, 0.3); }
		.btn-map-warn {
			background-color: rgba(245, 158, 11, 0.15) !important;
			border: 1px solid rgba(245, 158, 11, 0.5) !important;
			color: #fbbf24 !important;
		}
		.btn-map-warn:hover {
			background-color: rgba(245, 158, 11, 0.3) !important;
		}
		table {
			width: 100%%;
			border-collapse: collapse;
			background: var(--bg-surface);
			border-radius: 8px;
			overflow: hidden;
			border: 1px solid var(--border);
			margin-bottom: 2rem;
		}
		th, td { padding: 0.75rem 1rem; text-align: left; }
		th {
			background: #131a24;
			font-weight: 600;
			color: var(--text-muted);
			font-size: 0.83rem;
			border-bottom: 1px solid var(--border);
		}
		tr:not(:last-child) td { border-bottom: 1px solid var(--border); }
		tr:hover td { background: var(--bg-hover); }
		td.name a { color: var(--text-main); text-decoration: none; font-weight: 500; }
		td.name a:hover { color: var(--accent-primary); text-decoration: underline; }
		td.time, td.size { color: var(--text-muted); font-size: 0.85rem; }
		td.actions { text-align: right; display: flex; justify-content: flex-end; gap: 0.5rem; }
		.local-inspect-bar {
			display: flex;
			align-items: center;
			justify-content: space-between;
			flex-wrap: wrap;
			gap: 0.75rem;
			padding: 0.85rem 1.25rem;
			margin-top: -1.25rem;
			margin-bottom: 2rem;
			background: rgba(26, 34, 48, 0.5);
			border: 1px dashed var(--border);
			border-radius: 8px;
			font-size: 0.88rem;
			color: var(--text-muted);
		}
		.inspector-modal-backdrop {
			position: fixed;
			top: 0;
			left: 0;
			right: 0;
			bottom: 0;
			background: rgba(10, 14, 20, 0.82);
			backdrop-filter: blur(4px);
			-webkit-backdrop-filter: blur(4px);
			z-index: 9999;
			display: flex;
			align-items: center;
			justify-content: center;
			padding: 1.5rem;
			box-sizing: border-box;
		}
		.inspector-card {
			background: var(--bg-surface);
			border: 1px solid var(--border);
			border-radius: 12px;
			box-shadow: 0 20px 40px rgba(0, 0, 0, 0.6);
			width: 100%%;
			max-width: 880px;
			max-height: 85vh;
			display: flex;
			flex-direction: column;
			overflow: hidden;
			color: var(--text-main);
			animation: modalFadeIn 0.15s ease-out;
		}
		@keyframes modalFadeIn {
			from { opacity: 0; transform: scale(0.97); }
			to { opacity: 1; transform: scale(1); }
		}
		.inspector-header {
			padding: 1.25rem 1.5rem;
			border-bottom: 1px solid var(--border);
			background: #131a24;
			display: flex;
			justify-content: space-between;
			align-items: center;
		}
		.modal-close-btn {
			background: transparent;
			border: none;
			color: var(--text-muted);
			font-size: 1.5rem;
			cursor: pointer;
			padding: 0.25rem 0.5rem;
			border-radius: 4px;
			line-height: 1;
		}
		.modal-close-btn:hover { color: #ffffff; background: rgba(255, 255, 255, 0.1); }
		.inspector-tabs {
			display: flex;
			background: #111722;
			border-bottom: 1px solid var(--border);
			padding: 0.35rem 1rem 0 1rem;
			gap: 0.5rem;
			overflow-x: auto;
		}
		.inspector-tab-btn {
			background: transparent;
			border: none;
			border-bottom: 2px solid transparent;
			color: var(--text-muted);
			font-size: 0.88rem;
			font-weight: 500;
			padding: 0.6rem 0.9rem;
			cursor: pointer;
			white-space: nowrap;
			transition: all 0.15s;
		}
		.inspector-tab-btn:hover { color: var(--text-main); }
		.inspector-tab-btn.active {
			color: var(--accent-primary);
			border-bottom-color: var(--accent-primary);
		}
		.inspector-body {
			padding: 1.5rem;
			overflow-y: auto;
			flex: 1;
		}
		.inspector-grid {
			display: grid;
			grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
			gap: 0.85rem;
		}
		.inspector-box {
			background: #131a24;
			border: 1px solid var(--border);
			border-radius: 6px;
			padding: 0.85rem 1rem;
		}
		.inspector-box .box-label {
			font-size: 0.72rem;
			font-weight: 600;
			color: var(--text-muted);
			letter-spacing: 0.05em;
			margin-bottom: 0.3rem;
		}
		.inspector-box .box-value {
			font-size: 0.98rem;
			font-weight: 600;
			color: var(--text-main);
			word-break: break-word;
		}
		.inspector-table {
			width: 100%%;
			border-collapse: collapse;
			border: 1px solid var(--border);
			border-radius: 6px;
			overflow: hidden;
			background: #131a24;
		}
		.inspector-table th, .inspector-table td {
			padding: 0.6rem 0.85rem;
			text-align: left;
			border-bottom: 1px solid var(--border);
		}
		.inspector-table th { background: #0e141c; color: var(--text-muted); font-size: 0.8rem; }
		.platform-pill {
			background: rgba(59, 130, 246, 0.15);
			color: #60a5fa;
			border: 1px solid rgba(59, 130, 246, 0.4);
			font-size: 0.75rem;
			font-weight: 600;
			padding: 0.15rem 0.45rem;
			border-radius: 4px;
		}
		.clone-pill {
			background: rgba(168, 85, 247, 0.15);
			color: #c084fc;
			border: 1px solid rgba(168, 85, 247, 0.4);
			font-size: 0.72rem;
			font-weight: 600;
			padding: 0.15rem 0.45rem;
			border-radius: 4px;
			margin-left: 0.4rem;
		}
		.health-badge {
			font-size: 0.8rem;
			font-weight: 600;
			padding: 0.3rem 0.65rem;
			border-radius: 6px;
			white-space: nowrap;
		}
		.health-badge.ok { background: rgba(34, 197, 94, 0.15); border: 1px solid rgba(34, 197, 94, 0.4); color: #4ade80; }
		.health-badge.error { background: rgba(239, 68, 68, 0.15); border: 1px solid rgba(239, 68, 68, 0.4); color: #f87171; }
		.health-badge.warn { background: rgba(245, 158, 11, 0.15); border: 1px solid rgba(245, 158, 11, 0.4); color: #fbbf24; }
		.diag-item {
			padding: 0.9rem 1rem;
			border-radius: 6px;
			border: 1px solid var(--border);
			background: #131a24;
		}
		.diag-item.diag-pass { border-left: 4px solid #22c55e; }
		.diag-item.diag-error { border-left: 4px solid #ef4444; background: rgba(239, 68, 68, 0.05); }
		.diag-item.diag-warn { border-left: 4px solid #f59e0b; background: rgba(245, 158, 11, 0.05); }
		.diag-item.diag-info { border-left: 4px solid #3b82f6; }
		.diag-tag {
			font-size: 0.7rem;
			font-weight: 700;
			padding: 0.1rem 0.4rem;
			border-radius: 4px;
			background: rgba(255, 255, 255, 0.08);
			color: var(--text-muted);
		}
		.mod-settings-grid {
			display: grid;
			grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
			gap: 0.65rem;
		}
		.mod-setting-pill {
			background: #131a24;
			border: 1px solid var(--border);
			padding: 0.6rem 0.75rem;
			border-radius: 6px;
			font-size: 0.82rem;
			overflow-wrap: anywhere;
			word-break: break-word;
		}
		.inspector-spinner {
			width: 42px;
			height: 42px;
			border: 3px solid rgba(250, 128, 41, 0.2);
			border-top-color: var(--accent-primary);
			border-radius: 50%%;
			animation: spin 0.8s linear infinite;
			margin: 0 auto;
		}
		@keyframes spin { to { transform: rotate(360deg); } }
		.editor-section {
			background: #111722;
			border: 1px solid var(--border);
			border-radius: 8px;
			padding: 1.25rem;
		}
		.editor-action-card {
			background: #131a24;
			border: 1px solid var(--border);
			border-radius: 6px;
			padding: 1rem;
			display: flex;
			flex-direction: column;
			justify-content: space-between;
		}
	</style>
</head>
<body>
	<div class="container">
		<header>
			<h1>⚙️ %s Portal</h1>
			<div class="btn-group">
				<span class="status-badge %s">%s</span>
				<span class="badge">Build CL-%d</span>
				<span class="badge">TPS: %s</span>
				<span class="badge">%d / %d Pioneers</span>
				<a href="/admin" class="btn btn-secondary" style="font-size:0.82rem; padding:0.35rem 0.75rem; text-decoration:none; margin-left:0.35rem;">🔒 Admin</a>
			</div>
		</header>

		<div class="telemetry-bar">
			<div class="telemetry-row">
				<div class="pioneer-list">
					<strong>👷 Online Pioneers:</strong>
					%s
				</div>
				<div class="text-muted">Session: <strong>%s</strong> &bull; Tier %d</div>
			</div>
			%s
		</div>

		%s
		%s
		%s
		%s

		<h2 class="section-title">
			<span>💾 World Save Files</span>
			<span class="badge">%d Saves</span>
		</h2>
		<table>
			<thead>
				<tr>
					<th>File Name</th>
					<th>Playtime</th>
					<th>Last Modified</th>
					<th>Size</th>
					<th style="text-align: right;">Actions</th>
				</tr>
			</thead>
			<tbody>%s</tbody>
		</table>

		<div class="local-inspect-bar">
			<span>💡 <strong>Offline Save Inspector:</strong> You can also inspect any single-player or local save directly from your computer in your browser.</span>
			<label class="btn btn-secondary" style="cursor: pointer; display: inline-flex; align-items: center; gap: 0.4rem;">
				📂 Inspect Local .sav
				<input type="file" accept=".sav" style="display: none;" onchange="if(this.files.length) openSaveInspector(this.files[0], this.files[0].name)">
			</label>
		</div>
	</div>
</body>
</html>`,
		html.EscapeString(s.cfg.ServerName),
		html.EscapeString(s.cfg.ServerName),
		statusClass,
		html.EscapeString(snap.StatusLabel),
		gameVersion,
		html.EscapeString(tpsDisplay),
		snap.NumConnectedPlayers,
		snap.PlayerLimit,
		pioneerHTML.String(),
		html.EscapeString(sessionDisplay),
		snap.TechTier,
		func() string {
			if recentHTML.Len() == 0 {
				return ""
			}
			return fmt.Sprintf(`<div class="telemetry-row"><div class="pioneer-list"><span class="text-muted">Recently Seen:</span> %s</div></div>`, recentHTML.String())
		}(),
		modCardHTML,
		latestCardHTML,
		modTableHTML,
		bpSectionHTML,
		len(saveList),
		saveRows.String(),
	)

	s.writeBytes(w, http.StatusOK, "text/html; charset=utf-8", []byte(pageHTML), sendBody)
}
