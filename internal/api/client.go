package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client interacts with the Satisfactory Dedicated Server HTTPS API (/api/v1).
type Client struct {
	BaseURL    string
	APIToken   string
	HTTPClient *http.Client
}

// NewClient constructs a Satisfactory HTTPS API client.
// Since dedicated servers often use self-signed TLS certificates on localhost,
// InsecureSkipVerify is enabled by default for internal health/state polling.
func NewClient(baseURL, apiToken string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://127.0.0.1:7777"
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	return &Client{
		BaseURL:  baseURL,
		APIToken: strings.TrimSpace(apiToken),
		HTTPClient: &http.Client{
			Transport: tr,
			Timeout:   8 * time.Second,
		},
	}
}

type rpcRequest struct {
	Function string `json:"function"`
	Data     any    `json:"data,omitempty"`
}

type apiErrorResponse struct {
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

// HealthCheckResponse represents the data returned by HealthCheck.
type HealthCheckResponse struct {
	Data struct {
		Health           string `json:"health"`
		ServerCustomData string `json:"serverCustomData"`
	} `json:"data"`
}

// ServerGameState represents the live server state returned by QueryServerState.
type ServerGameState struct {
	ActiveSessionName   string  `json:"activeSessionName"`
	NumConnectedPlayers int     `json:"numConnectedPlayers"`
	PlayerLimit         int     `json:"playerLimit"`
	TechTier            int     `json:"techTier"`
	ActiveSchematic     string  `json:"activeSchematic"`
	GamePhase           string  `json:"gamePhase"`
	IsGameRunning       bool    `json:"isGameRunning"`
	TotalGameDuration   int     `json:"totalGameDuration"`
	IsGamePaused        bool    `json:"isGamePaused"`
	AverageTickRate     float64 `json:"averageTickRate"`
	AutoLoadSessionName string  `json:"autoLoadSessionName"`
}

type queryServerStateResponse struct {
	Data struct {
		ServerGameState ServerGameState `json:"serverGameState"`
	} `json:"data"`
}

// HealthCheck queries the unauthenticated HealthCheck endpoint.
func (c *Client) HealthCheck(ctx context.Context) (string, error) {
	reqBody := rpcRequest{
		Function: "HealthCheck",
		Data: map[string]string{
			"ClientCustomData": "",
		},
	}

	var out HealthCheckResponse
	if err := c.do(ctx, reqBody, &out, false); err != nil {
		return "", err
	}
	if out.Data.Health == "" {
		return "unknown", nil
	}
	return out.Data.Health, nil
}

// QueryServerState retrieves live game metrics (players, tick rate, tier, session).
func (c *Client) QueryServerState(ctx context.Context) (*ServerGameState, error) {
	reqBody := rpcRequest{
		Function: "QueryServerState",
		Data:     map[string]any{},
	}

	var out queryServerStateResponse
	if err := c.do(ctx, reqBody, &out, false); err != nil {
		return nil, err
	}
	return &out.Data.ServerGameState, nil
}

// SaveGame triggers an immediate world save on the server (requires an Admin API token).
func (c *Client) SaveGame(ctx context.Context, saveName string) error {
	if c.APIToken == "" {
		return fmt.Errorf("SERVER_API_TOKEN is required to call SaveGame")
	}
	reqBody := rpcRequest{
		Function: "SaveGame",
		Data: map[string]string{
			"SaveName": saveName,
		},
	}
	return c.do(ctx, reqBody, nil, true)
}

func (c *Client) do(ctx context.Context, payload rpcRequest, out any, requireAuth bool) error {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	endpoint := c.BaseURL + "/api/v1"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIToken)
	} else if requireAuth {
		return fmt.Errorf("authentication token required for %s", payload.Function)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s failed: %w", payload.Function, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr apiErrorResponse
		if json.Unmarshal(respBytes, &apiErr) == nil && apiErr.ErrorCode != "" {
			return fmt.Errorf("API %s error (%d): %s - %s", payload.Function, resp.StatusCode, apiErr.ErrorCode, apiErr.ErrorMessage)
		}
		return fmt.Errorf("API %s returned HTTP %d", payload.Function, resp.StatusCode)
	}

	if out != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, out); err != nil {
			return fmt.Errorf("decode %s response: %w", payload.Function, err)
		}
	}
	return nil
}
