package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendWebhook(t *testing.T) {
	var received webhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "UNSC-Section-III")
	err := client.Send(context.Background(), Embed{
		Title:       "Mods Updated",
		Description: "Applied 2 mod updates",
		Color:       ColorSuccess,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(received.Embeds) != 1 || received.Embeds[0].Title != "Mods Updated" {
		t.Fatalf("unexpected payload: %+v", received)
	}
}

func TestSendNoopWhenEmptyURL(t *testing.T) {
	client := NewClient("", "UNSC-Foundry")
	if err := client.Send(context.Background(), Embed{Title: "Noop"}); err != nil {
		t.Fatalf("expected nil error for empty webhook url, got %v", err)
	}
}
