package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/config"
)

// UploadResult is the result of uploading a replay to the share station.
type UploadResult struct {
	Success  bool
	ReplayID string
	ScoreID  int32
	Message  string
}

// UploadToShareStation uploads a replay file to the share station.
func UploadToShareStation(fileData []byte, filename string, chartName, userName string, cfg *config.ShareStationConfig, proxy string) (*UploadResult, error) {
	if cfg == nil {
		return &UploadResult{Success: false, Message: "share-station-not-configured"}, nil
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := fileWriter.Write(fileData); err != nil {
		return nil, err
	}
	if chartName != "" {
		_ = writer.WriteField("chart_name", chartName)
	}
	if userName != "" {
		_ = writer.WriteField("username", userName)
	}
	_ = writer.Close()

	uploadURL := cfg.URL + "/upload_direct"
	req, err := http.NewRequest(http.MethodPost, uploadURL, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := NewHTTPClient(proxy, 60*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return &UploadResult{Success: false, Message: "upload-failed: " + err.Error()}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return &UploadResult{Success: false, Message: "upload-failed: " + string(respBody)}, nil
	}

	var result struct {
		ReplayID string `json:"replay_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &UploadResult{Success: false, Message: "upload-failed: invalid response"}, nil
	}

	// Parse score_id from replay_id format: "{user_id}_{chart_id}_{score_id}.phirarec"
	var scoreID int32
	if re := regexp.MustCompile(`_(\d+)\.phirarec$`); re.MatchString(result.ReplayID) {
		matches := re.FindStringSubmatch(result.ReplayID)
		if len(matches) > 1 {
			var sid int64
			fmt.Sscanf(matches[1], "%d", &sid)
			scoreID = int32(sid)
		}
	}

	return &UploadResult{
		Success:  true,
		ReplayID: result.ReplayID,
		ScoreID:  scoreID,
	}, nil
}

// SetReplayVisibility sets the visibility of a replay on the share station.
func SetReplayVisibility(scoreID int32, visible bool, cfg *config.ShareStationConfig, proxy string) bool {
	if cfg == nil {
		return false
	}

	endpoint := fmt.Sprintf("/hide/%d", scoreID)
	if visible {
		endpoint = fmt.Sprintf("/show/%d", scoreID)
	}
	url := cfg.URL + endpoint

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)

	client := NewHTTPClient(proxy, 10*time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
