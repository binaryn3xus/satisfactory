package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/binaryn3xus/satisfactory/internal/api"
	"github.com/binaryn3xus/satisfactory/internal/mods"
	"github.com/binaryn3xus/satisfactory/internal/notify"
	"github.com/binaryn3xus/satisfactory/internal/version"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cmd := "help"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	var err error
	switch cmd {
	case "reconcile-mods":
		err = runReconcileMods()
	case "healthcheck":
		err = runHealthCheck()
	case "pre-stop":
		err = runPreStop()
	case "notify-update":
		err = runNotifyUpdate(os.Args[2:])
	case "detect-changelist":
		err = runDetectChangelist()
	default:
		fmt.Fprintf(os.Stderr, "Usage: ficsit-ctl <reconcile-mods|healthcheck|pre-stop|notify-update|detect-changelist>\n")
		os.Exit(2)
	}

	if err != nil {
		slog.Error("command failed", slog.String("cmd", cmd), slog.Any("error", err))
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		return fallback
	}
}

func runReconcileMods() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	gamefilesDir := envOr("GAMEFILES_DIR", "/config/gamefiles")
	localDir := envOr("FICSIT_LOCAL_DIR", "/config/ficsit/local")
	cacheDir := envOr("FICSIT_CACHE_DIR", "/config/ficsit/cache")
	ficsitBin := envOr("FICSIT_BIN", "ficsit-cli")
	modsEnabled := envBool("MODS_ENABLED", false)
	strictMode := envBool("MODS_STRICT_MODE", true)
	profileName := envOr("MODS_PROFILE", "UNSC-Section-III")
	updatePolicy := envOr("MODS_UPDATE_POLICY", "always")
	modsFile := envOr("MODS_FILE", "/config/mods.json")
	modsListRaw := os.Getenv("MODS_LIST")
	serverName := envOr("SERVER_NAME", "Satisfactory Server")
	discordURL := os.Getenv("DISCORD_WEBHOOK_URL")

	notifier := notify.NewClient(discordURL, serverName)

	fileSpecs, err := mods.LoadModSpecsFromFile(modsFile)
	if err != nil {
		return err
	}
	envSpecs, err := mods.ParseModList(modsListRaw)
	if err != nil {
		return err
	}
	desiredMods := mods.MergeModSpecs(fileSpecs, envSpecs)

	cl, _ := version.DetectChangelist(gamefilesDir)
	slog.Info("starting declarative mod reconciliation",
		slog.Bool("enabled", modsEnabled),
		slog.Bool("strict_mode", strictMode),
		slog.String("profile", profileName),
		slog.String("update_policy", updatePolicy),
		slog.Int("declared_mods", len(desiredMods)),
		slog.Int("game_changelist", cl),
	)

	cfg := mods.ReconcileConfig{
		GamefilesDir: gamefilesDir,
		FicsitBin:    ficsitBin,
		LocalDir:     localDir,
		CacheDir:     cacheDir,
		Enabled:      modsEnabled,
		StrictMode:   strictMode,
		ProfileName:  profileName,
		UpdatePolicy: updatePolicy,
		Mods:         desiredMods,
	}

	res, err := mods.Reconcile(ctx, cfg)
	if err != nil {
		_ = notifier.Send(ctx, notify.Embed{
			Title:       "🛑 Strict Mode: Mod Reconciliation Blocked Startup",
			Description: fmt.Sprintf("Failed to reconcile mods for **%s** (Build `CL-%d`).\nServer startup has been halted to protect world saves from missing-mod corruption.\n\n```text\n%v\n```", serverName, cl, err),
			Color:       notify.ColorError,
		})
		if strictMode {
			return fmt.Errorf("BLOCKED STARTUP (MODS_STRICT_MODE=true): %w", err)
		}
		slog.Warn("mod reconciliation failed, but MODS_STRICT_MODE=false; continuing startup", slog.Any("error", err))
		return nil
	}

	slog.Info("mod reconciliation complete",
		slog.Int("installed_mods", len(res.Installed)),
		slog.Int("updated_mods", len(res.Changes)),
	)

	if modsEnabled && len(res.Changes) > 0 {
		var lines []string
		for _, ch := range res.Changes {
			if ch.OldVersion == "" {
				lines = append(lines, fmt.Sprintf("• **%s**: installed `v%s`", ch.Ref, ch.NewVersion))
			} else {
				lines = append(lines, fmt.Sprintf("• **%s**: `v%s` → `v%s`", ch.Ref, ch.OldVersion, ch.NewVersion))
			}
		}
		_ = notifier.Send(ctx, notify.Embed{
			Title:       "🧩 Server Mods Updated",
			Description: fmt.Sprintf("Applied **%d** mod update(s) on **%s** (`CL-%d`):\n%s", len(res.Changes), serverName, cl, strings.Join(lines, "\n")),
			Color:       notify.ColorInfo,
		})
	}

	return nil
}

func runHealthCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	port := envOr("SERVER_GAME_PORT", "7777")
	apiURL := envOr("SERVER_API_URL", fmt.Sprintf("https://127.0.0.1:%s", port))
	client := api.NewClient(apiURL, os.Getenv("SERVER_API_TOKEN"))

	health, err := client.HealthCheck(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("OK (%s)\n", health)
	return nil
}

func runPreStop() error {
	token := strings.TrimSpace(os.Getenv("SERVER_API_TOKEN"))
	if token == "" {
		slog.Info("SERVER_API_TOKEN not set; skipping pre-shutdown API SaveGame call")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	port := envOr("SERVER_GAME_PORT", "7777")
	apiURL := envOr("SERVER_API_URL", fmt.Sprintf("https://127.0.0.1:%s", port))
	client := api.NewClient(apiURL, token)

	saveName := fmt.Sprintf("pre_shutdown_%s", time.Now().UTC().Format("20060102_150405"))
	slog.Info("triggering pre-shutdown world save via HTTPS API", slog.String("save_name", saveName))
	if err := client.SaveGame(ctx, saveName); err != nil {
		slog.Warn("pre-shutdown SaveGame call returned an error (server may not have a loaded session)", slog.Any("error", err))
		return nil
	}
	slog.Info("pre-shutdown world save completed")
	return nil
}

func runNotifyUpdate(args []string) error {
	oldBuild := "unknown"
	newBuild := "unknown"
	if len(args) > 0 && args[0] != "" {
		oldBuild = args[0]
	}
	if len(args) > 1 && args[1] != "" {
		newBuild = args[1]
	}

	serverName := envOr("SERVER_NAME", "Satisfactory Server")
	discordURL := os.Getenv("DISCORD_WEBHOOK_URL")
	notifier := notify.NewClient(discordURL, serverName)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return notifier.Send(ctx, notify.Embed{
		Title:       "🚀 Satisfactory Server Updated",
		Description: fmt.Sprintf("**%s** has been updated via SteamCMD (`%s` → `%s`).", serverName, oldBuild, newBuild),
		Color:       notify.ColorSuccess,
	})
}

func runDetectChangelist() error {
	gamefilesDir := envOr("GAMEFILES_DIR", "/config/gamefiles")
	cl, err := version.DetectChangelist(gamefilesDir)
	if err != nil {
		return err
	}
	fmt.Println(cl)
	return nil
}
