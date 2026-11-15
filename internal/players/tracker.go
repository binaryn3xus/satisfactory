package players

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/binaryn3xus/satisfactory/internal/api"
)

// Pioneer represents a connected or recently seen player.
type Pioneer struct {
	Name       string  `json:"name"`
	Online     bool    `json:"online"`
	LastEvent  string  `json:"last_event,omitempty"`
	Ping       int     `json:"ping,omitempty"`
	Health     float64 `json:"health,omitempty"`
	Source     string  `json:"source"` // "log", "frm"
}

// PowerSummary holds optional FicsitRemoteMonitoring (FRM) power grid telemetry.
type PowerSummary struct {
	TotalProductionMW  float64 `json:"total_production_mw"`
	TotalConsumptionMW float64 `json:"total_consumption_mw"`
	MaxConsumptionMW   float64 `json:"max_consumption_mw"`
}

// Snapshot represents the combined live status of the server and pioneers.
type Snapshot struct {
	ServerOnline        bool           `json:"server_online"`
	StatusLabel         string         `json:"status_label"` // "Online", "Paused", "Loading / Idle", "Offline"
	ActiveSessionName   string         `json:"active_session_name"`
	NumConnectedPlayers int            `json:"num_connected_players"`
	PlayerLimit         int            `json:"player_limit"`
	TechTier            int            `json:"tech_tier"`
	GamePhase           string         `json:"game_phase"`
	AverageTickRate     float64        `json:"average_tick_rate"`
	IsGameRunning       bool           `json:"is_game_running"`
	IsGamePaused        bool           `json:"is_game_paused"`
	OnlinePioneers      []Pioneer      `json:"online_pioneers"`
	RecentPioneers      []Pioneer      `json:"recent_pioneers"`
	FRMConnected        bool           `json:"frm_connected"`
	Power               *PowerSummary  `json:"power,omitempty"`
}

var (
	// Matches Unreal Engine log timestamps like "[2026.10.10-20.15.30:123]"
	logTimestampRe = regexp.MustCompile(`^\[(\d{4}\.\d{2}\.\d{2}-\d{2}\.\d{2}\.\d{2}):\d+\]`)
	// Matches LogNet: Login request: ... ?Name=BinaryNexus ... userId: Steam:12345
	loginRequestRe = regexp.MustCompile(`LogNet:\s+Login request:.*\?Name=([^\?&]+?)(?:\?|\s+userId:\s*([^\s]+))`)
	// Matches LogNet: Join succeeded: BinaryNexus
	joinSucceededRe = regexp.MustCompile(`LogNet:\s+Join succeeded:\s+(.+)$`)
	// Matches UNetConnection::Close or UChannel::Close with UniqueId
	connCloseRe = regexp.MustCompile(`(?:UNetConnection::Close|UChannel::Close|UChannel::CleanUp).*UniqueId:\s*([^\s,]+)`)
)

// ParseLogPioneers parses FactoryGame.log to determine currently online and recently seen pioneers.
func ParseLogPioneers(logPath string) (online []Pioneer, recent []Pioneer) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, nil
	}
	defer f.Close()

	type playerRecord struct {
		name      string
		online    bool
		lastEvent string
		order     int
	}

	userIDToName := make(map[string]string)
	records := make(map[string]*playerRecord)
	seq := 0

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 512*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "LogNet:") {
			continue
		}

		ts := ""
		if m := logTimestampRe.FindStringSubmatch(line); len(m) == 2 {
			if parsed, err := time.Parse("2006.01.02-15.04.05", m[1]); err == nil {
				ts = parsed.UTC().Format("2006-01-02 15:04 UTC")
			}
		}

		if m := loginRequestRe.FindStringSubmatch(line); len(m) >= 2 {
			name := strings.TrimSpace(m[1])
			uid := ""
			if len(m) >= 3 {
				uid = strings.TrimSpace(m[2])
			}
			if name != "" && uid != "" && uid != "INVALID" {
				userIDToName[uid] = name
			}
			continue
		}

		if m := joinSucceededRe.FindStringSubmatch(line); len(m) == 2 {
			name := strings.TrimSpace(m[1])
			if name == "" {
				continue
			}
			seq++
			records[name] = &playerRecord{
				name:      name,
				online:    true,
				lastEvent: ts,
				order:     seq,
			}
			continue
		}

		if m := connCloseRe.FindStringSubmatch(line); len(m) == 2 {
			uid := strings.TrimSpace(m[1])
			if name, ok := userIDToName[uid]; ok {
				if rec, exists := records[name]; exists && rec.online {
					seq++
					rec.online = false
					rec.lastEvent = ts
					rec.order = seq
				}
			}
		}
	}

	var all []*playerRecord
	for _, r := range records {
		all = append(all, r)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].order > all[j].order
	})

	for _, r := range all {
		p := Pioneer{
			Name:      r.name,
			Online:    r.online,
			LastEvent: r.lastEvent,
			Source:    "log",
		}
		if r.online {
			online = append(online, p)
		} else {
			recent = append(recent, p)
		}
	}
	return online, recent
}

// CollectSnapshot gathers the unified server + pioneer status from the 1.0 HTTPS API,
// FactoryGame.log, and optional FicsitRemoteMonitoring (FRM) endpoint.
func CollectSnapshot(ctx context.Context, apiClient *api.Client, logPath string, frmBaseURL string, maxPlayersFallback int) Snapshot {
	snap := Snapshot{
		StatusLabel: "Offline",
		PlayerLimit: maxPlayersFallback,
	}

	onlineLog, recentLog := ParseLogPioneers(logPath)

	if apiClient != nil {
		if state, err := apiClient.QueryServerState(ctx); err == nil && state != nil {
			snap.ServerOnline = true
			snap.ActiveSessionName = state.ActiveSessionName
			snap.NumConnectedPlayers = state.NumConnectedPlayers
			if state.PlayerLimit > 0 {
				snap.PlayerLimit = state.PlayerLimit
			}
			snap.TechTier = state.TechTier
			snap.GamePhase = state.GamePhase
			snap.AverageTickRate = state.AverageTickRate
			snap.IsGameRunning = state.IsGameRunning
			snap.IsGamePaused = state.IsGamePaused

			switch {
			case !state.IsGameRunning:
				snap.StatusLabel = "Idle / Lobby"
			case state.IsGamePaused:
				snap.StatusLabel = "Paused"
			default:
				snap.StatusLabel = "Online"
			}

			// Self-healing reconciliation: if the authoritative server state reports 0 connected players,
			// move any stale log-tracked online players into RecentPioneers.
			if state.NumConnectedPlayers == 0 && len(onlineLog) > 0 {
				for _, p := range onlineLog {
					p.Online = false
					recentLog = append([]Pioneer{p}, recentLog...)
				}
				onlineLog = nil
			}
		} else if health, herr := apiClient.HealthCheck(ctx); herr == nil && health != "" {
			snap.ServerOnline = true
			snap.StatusLabel = "Online"
		}
	}

	snap.OnlinePioneers = onlineLog
	if len(recentLog) > 8 {
		recentLog = recentLog[:8]
	}
	snap.RecentPioneers = recentLog

	// Optional FicsitRemoteMonitoring (FRM) enrichment
	if frmBaseURL != "" {
		if frmPioneers, power, ok := queryFRM(ctx, frmBaseURL); ok {
			snap.FRMConnected = true
			if len(frmPioneers) > 0 {
				snap.OnlinePioneers = frmPioneers
				snap.NumConnectedPlayers = len(frmPioneers)
			}
			snap.Power = power
		}
	}

	if snap.ServerOnline && snap.NumConnectedPlayers == 0 && len(snap.OnlinePioneers) > 0 && apiClient == nil {
		snap.NumConnectedPlayers = len(snap.OnlinePioneers)
	}

	return snap
}

type frmPlayer struct {
	PlayerName string  `json:"PlayerName"`
	Name       string  `json:"Name"`
	Online     bool    `json:"Online"`
	Ping       int     `json:"Ping"`
	PlayerHP   float64 `json:"PlayerHP"`
}

type frmPowerCircuit struct {
	PowerProduction float64 `json:"PowerProduction"`
	PowerConsumed   float64 `json:"PowerConsumed"`
	PowerMaxConsumed float64 `json:"PowerMaxConsumed"`
}

func queryFRM(ctx context.Context, baseURL string) ([]Pioneer, *PowerSummary, bool) {
	baseURL = strings.TrimRight(baseURL, "/")
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/getPlayer", nil)
	if err != nil {
		return nil, nil, false
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, false
	}

	var rawPlayers []frmPlayer
	if err := json.NewDecoder(resp.Body).Decode(&rawPlayers); err != nil {
		return nil, nil, false
	}

	var online []Pioneer
	for _, rp := range rawPlayers {
		name := strings.TrimSpace(rp.PlayerName)
		if name == "" {
			name = strings.TrimSpace(rp.Name)
		}
		if name == "" || !rp.Online {
			continue
		}
		online = append(online, Pioneer{
			Name:   name,
			Online: true,
			Ping:   rp.Ping,
			Health: rp.PlayerHP,
			Source: "frm",
		})
	}

	var power *PowerSummary
	pReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/getPower", nil)
	if err == nil {
		if pResp, err := client.Do(pReq); err == nil {
			defer pResp.Body.Close()
			var circuits []frmPowerCircuit
			if pResp.StatusCode == http.StatusOK && json.NewDecoder(pResp.Body).Decode(&circuits) == nil && len(circuits) > 0 {
				power = &PowerSummary{}
				for _, c := range circuits {
					power.TotalProductionMW += c.PowerProduction
					power.TotalConsumptionMW += c.PowerConsumed
					power.MaxConsumptionMW += c.PowerMaxConsumed
				}
			}
		}
	}

	return online, power, true
}
