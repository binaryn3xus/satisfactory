package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthCheckAndQueryState(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Function {
		case "HealthCheck":
			_, _ = w.Write([]byte(`{"data":{"health":"healthy","serverCustomData":""}}`))
		case "QueryServerState":
			_, _ = w.Write([]byte(`{"data":{"serverGameState":{"activeSessionName":"Project-MJOLNIR","numConnectedPlayers":3,"playerLimit":8,"techTier":6,"averageTickRate":29.95,"isGameRunning":true}}}`))
		case "SaveGame":
			if r.Header.Get("Authorization") != "Bearer secret-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret-token")

	health, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}
	if health != "healthy" {
		t.Fatalf("expected healthy, got %s", health)
	}

	state, err := client.QueryServerState(context.Background())
	if err != nil {
		t.Fatalf("QueryServerState error: %v", err)
	}
	if state.ActiveSessionName != "Project-MJOLNIR" || state.NumConnectedPlayers != 3 {
		t.Fatalf("unexpected state: %+v", state)
	}

	if err := client.SaveGame(context.Background(), "pre_shutdown"); err != nil {
		t.Fatalf("SaveGame error: %v", err)
	}
}
