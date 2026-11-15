package players

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseLogPioneersJoinAndLeave(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, "FactoryGame.log")

	logContent := `[2026.10.10-18.00.01:100][120]LogNet: Login request: ?Name=BinaryNexus userId: Steam:1 (Steam:76561198000000001)
[2026.10.10-18.00.02:200][125]LogNet: Join succeeded: BinaryNexus
[2026.10.10-18.05.00:100][300]LogNet: Login request: ?Name=Cortana userId: Steam:2 (Steam:76561198000000002)
[2026.10.10-18.05.01:200][305]LogNet: Join succeeded: Cortana
[2026.10.10-18.30.00:500][900]LogNet: UNetConnection::Close: [UNetConnection] RemoteAddr: 100.64.36.12:54321, Driver: GameNetDriver, UniqueId: Steam:2, Channels: 15
`
	if err := os.WriteFile(logPath, []byte(logContent), 0o644); err != nil {
		t.Fatal(err)
	}

	online, recent := ParseLogPioneers(logPath)
	if len(online) != 1 || online[0].Name != "BinaryNexus" {
		t.Fatalf("expected BinaryNexus online, got %+v", online)
	}
	if len(recent) != 1 || recent[0].Name != "Cortana" {
		t.Fatalf("expected Cortana in recent pioneers, got %+v", recent)
	}
}

func TestCollectSnapshotWithFRMEnrichment(t *testing.T) {
	frmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/getPlayer":
			_, _ = w.Write([]byte(`[{"PlayerName":"BinaryNexus","Online":true,"Ping":18,"PlayerHP":100}]`))
		case "/getPower":
			_, _ = w.Write([]byte(`[{"PowerProduction":12500.5,"PowerConsumed":8400.0,"PowerMaxConsumed":10200.0}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer frmSrv.Close()

	snap := CollectSnapshot(context.Background(), nil, "/nonexistent/FactoryGame.log", frmSrv.URL, 8)
	if !snap.FRMConnected {
		t.Fatalf("expected FRMConnected=true")
	}
	if len(snap.OnlinePioneers) != 1 || snap.OnlinePioneers[0].Name != "BinaryNexus" || snap.OnlinePioneers[0].Ping != 18 {
		t.Fatalf("unexpected online pioneers from FRM: %+v", snap.OnlinePioneers)
	}
	if snap.Power == nil || snap.Power.TotalProductionMW != 12500.5 {
		t.Fatalf("unexpected FRM power telemetry: %+v", snap.Power)
	}
}
