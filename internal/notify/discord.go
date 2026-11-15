package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	ColorSuccess = 0x2ECC71 // Green
	ColorInfo    = 0x3B82F6 // Blue
	ColorWarning = 0xFA8029 // FICSIT Orange
	ColorError   = 0xE74C3C // Red
)

type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type Embed struct {
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
}

type EmbedFooter struct {
	Text string `json:"text"`
}

type webhookPayload struct {
	Username string  `json:"username,omitempty"`
	Embeds   []Embed `json:"embeds"`
}

type Client struct {
	WebhookURL string
	ServerName string
	HTTPClient *http.Client
}

func NewClient(webhookURL, serverName string) *Client {
	if serverName == "" {
		serverName = "Satisfactory Server"
	}
	return &Client{
		WebhookURL: webhookURL,
		ServerName: serverName,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send posts a rich embed to Discord if WebhookURL is non-empty.
func (c *Client) Send(ctx context.Context, embed Embed) error {
	if c == nil || c.WebhookURL == "" {
		return nil
	}

	if embed.Timestamp == "" {
		embed.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if embed.Footer == nil {
		embed.Footer = &EmbedFooter{Text: fmt.Sprintf("FICSIT Server Monitor • %s", c.ServerName)}
	}

	payload := webhookPayload{
		Username: "ADA • " + c.ServerName,
		Embeds:   []Embed{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal discord webhook: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create discord request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("post discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}
