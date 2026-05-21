package network

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/utils"
)

const (
	defaultPhiraAPIEndpoint = "https://phira.5wyxi.com"
	fetchTimeoutMs          = 15000
	fetchMaxRetries         = 5
)

// PhiraUserInfo is the response from /me.
type PhiraUserInfo struct {
	ID       int32  `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language"`
}

// PhiraChart is the response from /chart/:id.
type PhiraChart struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

// PhiraRecord is the response from /record/:id.
type PhiraRecord struct {
	ID        int32   `json:"id"`
	Player    int32   `json:"player"`
	Score     int     `json:"score"`
	Perfect   int     `json:"perfect"`
	Good      int     `json:"good"`
	Bad       int     `json:"bad"`
	Miss      int     `json:"miss"`
	MaxCombo  int     `json:"max_combo"`
	Accuracy  float32 `json:"accuracy"`
	FullCombo bool    `json:"full_combo"`
	Std       float32 `json:"std"`
	StdScore  float32 `json:"std_score"`
}

// FetchPhiraUserInfo calls the Phira /me endpoint.
func FetchPhiraUserInfo(endpoint, token, proxy string) (*PhiraUserInfo, error) {
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	req, err := http.NewRequest("GET", endpoint+"/me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := utils.NewHTTPClient(proxy, fetchTimeoutMs*time.Millisecond)
	resp, err := utils.DoWithRetry(client, req, fetchMaxRetries)
	if err != nil {
		return nil, fmt.Errorf("auth-fetch-me-failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("auth-fetch-me-failed")
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("auth-invalid-response")
	}

	idRaw, ok := data["id"]
	if !ok {
		return nil, fmt.Errorf("auth-invalid-user-id")
	}
	var id int32
	switch v := idRaw.(type) {
	case float64:
		id = int32(v)
	case int:
		id = int32(v)
	case int32:
		id = v
	default:
		return nil, fmt.Errorf("auth-invalid-user-id")
	}

	nameRaw, ok := data["name"]
	if !ok {
		return nil, fmt.Errorf("auth-invalid-user-name")
	}
	name, ok := nameRaw.(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("auth-invalid-user-name")
	}

	lang := ""
	if l, ok := data["language"].(string); ok {
		lang = l
	}

	return &PhiraUserInfo{ID: id, Name: name, Language: lang}, nil
}

// FetchPhiraChart calls the Phira /chart/:id endpoint.
func FetchPhiraChart(endpoint string, id int32, proxy string) (*PhiraChart, error) {
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/chart/%d", endpoint, id), nil)
	if err != nil {
		return nil, err
	}
	client := utils.NewHTTPClient(proxy, fetchTimeoutMs*time.Millisecond)
	resp, err := utils.DoWithRetry(client, req, fetchMaxRetries)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("chart-fetch-failed")
	}

	var chart PhiraChart
	if err := json.NewDecoder(resp.Body).Decode(&chart); err != nil {
		return nil, err
	}
	return &chart, nil
}

// FetchPhiraRecord calls the Phira /record/:id endpoint.
func FetchPhiraRecord(endpoint string, id int32, proxy string) (*PhiraRecord, error) {
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/record/%d", endpoint, id), nil)
	if err != nil {
		return nil, err
	}
	client := utils.NewHTTPClient(proxy, fetchTimeoutMs*time.Millisecond)
	resp, err := utils.DoWithRetry(client, req, fetchMaxRetries)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("record-fetch-failed")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var record PhiraRecord
	if err := json.Unmarshal(body, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

// VerifyUserToken validates a Phira user token by calling /me. It mirrors the
// TS verifyUserToken helper in tphira-mp-main/src/server/network/httpHelpers.ts
// and applies the shared retry/backoff policy via DoWithRetry. Returns the
// resolved user on success or nil with an error when the token is invalid or
// the upstream is unreachable after retries.
func VerifyUserToken(endpoint, token, proxy string) (*PhiraUserInfo, error) {
	return FetchPhiraUserInfo(endpoint, token, proxy)
}
