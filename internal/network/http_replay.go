package network

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
)

const replaySessionTTL = 30 * time.Minute

type replaySession struct {
	UserID    int32
	ExpiresAt int64
}

func (h *HTTPServer) ensureReplaySessionsLocked() {
	if h.replaySessions == nil {
		h.replaySessions = make(map[string]replaySession)
	}
}

func (h *HTTPServer) cleanupReplaySessionsLocked(now int64) {
	h.ensureReplaySessionsLocked()
	for token, sess := range h.replaySessions {
		if now > sess.ExpiresAt {
			delete(h.replaySessions, token)
		}
	}
}

func (h *HTTPServer) getReplaySession(token string) (replaySession, bool) {
	now := time.Now().UnixMilli()
	h.replayMu.Lock()
	defer h.replayMu.Unlock()
	h.cleanupReplaySessionsLocked(now)
	sess, ok := h.replaySessions[token]
	if !ok || now > sess.ExpiresAt {
		return replaySession{}, false
	}
	return sess, true
}

func (h *HTTPServer) putReplaySession(userID int32) (string, int64) {
	token := newHTTPToken()
	expiresAt := time.Now().Add(replaySessionTTL).UnixMilli()
	h.replayMu.Lock()
	defer h.replayMu.Unlock()
	h.ensureReplaySessionsLocked()
	h.replaySessions[token] = replaySession{UserID: userID, ExpiresAt: expiresAt}
	return token, expiresAt
}

func newHTTPToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

func (h *HTTPServer) handleReplayAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad-token"})
		return
	}

	runtime := h.state.SnapshotRuntime()
	endpoint := runtime.Config.PhiraAPIEndpoint
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	me, err := FetchPhiraUserInfo(endpoint, strings.TrimSpace(req.Token), runtime.Config.OutboundProxy)
	if err != nil || me == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	charts := h.buildReplayCharts(me.ID)
	sessionToken, expiresAt := h.putReplaySession(me.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"userId":       me.ID,
		"charts":       charts,
		"sessionToken": sessionToken,
		"expiresAt":    expiresAt,
	})
}

type replayChartResponse struct {
	ChartID int32                 `json:"chartId"`
	Replays []replayEntryResponse `json:"replays"`
}

type replayEntryResponse struct {
	Timestamp   int64   `json:"timestamp"`
	RecordID    int32   `json:"recordId"`
	ScoreID     *int32  `json:"scoreId,omitempty"`
	DownloadURL *string `json:"downloadUrl,omitempty"`
}

func (h *HTTPServer) buildReplayCharts(userID int32) []replayChartResponse {
	runtime := h.state.SnapshotRuntime()
	listed, _ := replay.ListReplaysForUser(runtime.Config.ReplayBaseDir, userID)
	byChart := make(map[int32][]replayEntryResponse, len(listed))
	for chartID, entries := range listed {
		for _, entry := range entries {
			byChart[chartID] = append(byChart[chartID], replayEntryResponse{
				Timestamp: entry.Timestamp,
				RecordID:  entry.RecordID,
			})
		}
	}

	if h.state.UploadedReplayMeta != nil {
		userMeta := h.state.UploadedReplayMeta.GetUser(userID)
		for chartID, entries := range userMeta {
			for _, meta := range entries {
				if meta == nil {
					continue
				}
				item := replayEntryResponse{
					Timestamp: meta.Timestamp,
					RecordID:  0,
					ScoreID:   &meta.ScoreID,
				}
				if runtime.Config.ShareStation != nil && runtime.Config.ShareStation.URL != "" {
					url := fmt.Sprintf("%s/download/replay/%d", strings.TrimRight(runtime.Config.ShareStation.URL, "/"), meta.ScoreID)
					item.DownloadURL = &url
				}
				byChart[chartID] = append(byChart[chartID], item)
			}
		}
	}

	chartIDs := make([]int32, 0, len(byChart))
	for chartID := range byChart {
		chartIDs = append(chartIDs, chartID)
	}
	sort.Slice(chartIDs, func(i, j int) bool { return chartIDs[i] < chartIDs[j] })

	out := make([]replayChartResponse, 0, len(chartIDs))
	for _, chartID := range chartIDs {
		replays := byChart[chartID]
		sort.Slice(replays, func(i, j int) bool { return replays[i].Timestamp > replays[j].Timestamp })
		out = append(out, replayChartResponse{ChartID: chartID, Replays: replays})
	}
	return out
}

func (h *HTTPServer) handleReplayDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	sessionToken := strings.TrimSpace(q.Get("sessionToken"))
	if sessionToken == "" {
		h.handleAdminReplayDownload(w, r)
		return
	}

	chartID, err1 := strconv.ParseInt(q.Get("chartId"), 10, 32)
	timestamp, err2 := strconv.ParseInt(q.Get("timestamp"), 10, 64)
	if err1 != nil || err2 != nil || chartID < 0 || timestamp <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad-request"})
		return
	}
	sess, ok := h.getReplaySession(sessionToken)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	if h.serveReplayFile(w, r, sess.UserID, int32(chartID), timestamp) {
		return
	}

	if h.state.UploadedReplayMeta != nil {
		metas := h.state.UploadedReplayMeta.Get(sess.UserID, int32(chartID))
		runtime := h.state.SnapshotRuntime()
		for _, meta := range metas {
			if meta != nil && meta.Timestamp == timestamp && runtime.Config.ShareStation != nil && runtime.Config.ShareStation.URL != "" {
				http.Redirect(w, r, fmt.Sprintf("%s/download/replay/%d", strings.TrimRight(runtime.Config.ShareStation.URL, "/"), meta.ScoreID), http.StatusFound)
				return
			}
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not-found"})
}

func (h *HTTPServer) handleAdminReplayDownload(w http.ResponseWriter, r *http.Request) {
	if !h.adminTokenOK(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	q := r.URL.Query()
	userID, err1 := strconv.ParseInt(q.Get("userId"), 10, 32)
	chartID, err2 := strconv.ParseInt(q.Get("chartId"), 10, 32)
	timestamp, err3 := strconv.ParseInt(q.Get("timestamp"), 10, 64)
	if err1 != nil || err2 != nil || err3 != nil || userID < 0 || chartID < 0 || timestamp <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad-request"})
		return
	}
	if !h.serveReplayFile(w, r, int32(userID), int32(chartID), timestamp) {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not-found"})
	}
}

func (h *HTTPServer) serveReplayFile(w http.ResponseWriter, r *http.Request, userID, chartID int32, timestamp int64) bool {
	runtime := h.state.SnapshotRuntime()
	path := replay.ReplayFilePath(runtime.Config.ReplayBaseDir, userID, chartID, timestamp)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	header, err := replay.ReadReplayHeader(path)
	if err != nil || header == nil || header.UserID != userID || header.ChartID != chartID {
		return false
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%d.phirarec"`, timestamp))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	http.ServeFile(w, r, path)
	return true
}

func (h *HTTPServer) handleReplayDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionToken string `json:"sessionToken"`
		ChartID      int32  `json:"chartId"`
		Timestamp    int64  `json:"timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.SessionToken) == "" || req.ChartID < 0 || req.Timestamp <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad-request"})
		return
	}
	sess, ok := h.getReplaySession(strings.TrimSpace(req.SessionToken))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	runtime := h.state.SnapshotRuntime()
	path := replay.ReplayFilePath(runtime.Config.ReplayBaseDir, sess.UserID, req.ChartID, req.Timestamp)
	if header, err := replay.ReadReplayHeader(path); err == nil && header != nil && header.UserID == sess.UserID && header.ChartID == req.ChartID {
		if deleted, err := replay.DeleteReplayForUser(runtime.Config.ReplayBaseDir, sess.UserID, req.ChartID, req.Timestamp); err == nil && deleted {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
	}
	if h.state.UploadedReplayMeta != nil && h.state.UploadedReplayMeta.Delete(sess.UserID, req.ChartID, req.Timestamp) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not-found"})
}

func (h *HTTPServer) handleReplayUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Token     string `json:"token"`
		ChartID   int32  `json:"chartId"`
		Timestamp int64  `json:"timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" || req.ChartID < 0 || req.Timestamp <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad-request"})
		return
	}
	runtime := h.state.SnapshotRuntime()
	if !shareStationConfigured(runtime.Config) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "share-station-not-configured"})
		return
	}
	endpoint := runtime.Config.PhiraAPIEndpoint
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	me, err := FetchPhiraUserInfo(endpoint, strings.TrimSpace(req.Token), runtime.Config.OutboundProxy)
	if err != nil || me == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}

	path := replay.ReplayFilePath(runtime.Config.ReplayBaseDir, me.ID, req.ChartID, req.Timestamp)
	header, err := replay.ReadReplayHeader(path)
	if err != nil || header == nil || header.UserID != me.ID || header.ChartID != req.ChartID {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not-found"})
		return
	}
	fileData, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "upload-failed"})
		return
	}
	result, err := utils.UploadToShareStation(fileData, fmt.Sprintf("%d.phirarec", req.Timestamp), header.ChartName, header.UserName, runtime.Config.ShareStation, runtime.Config.OutboundProxy)
	if err != nil || result == nil || !result.Success {
		msg := "upload-failed"
		if result != nil && result.Message != "" {
			msg = result.Message
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": msg})
		return
	}
	if result.ScoreID != 0 && h.state.UploadedReplayMeta != nil {
		h.state.UploadedReplayMeta.Add(me.ID, req.ChartID, result.ScoreID, req.Timestamp)
		_ = utils.SetReplayVisibility(result.ScoreID, true, runtime.Config.ShareStation, runtime.Config.OutboundProxy)
		_ = os.Remove(path)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"userId":   me.ID,
		"chartId":  req.ChartID,
		"recordId": header.RecordID,
		"scoreId":  result.ScoreID,
		"message":  "upload-success",
	})
}

func (h *HTTPServer) handleReplayAutoUploadConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		token := strings.TrimSpace(r.URL.Query().Get("token"))
		h.handleReplayAutoUploadConfigWithToken(w, token, nil)
	case http.MethodPost:
		var req struct {
			Token string `json:"token"`
			Show  *bool  `json:"show"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad-token"})
			return
		}
		h.handleReplayAutoUploadConfigWithToken(w, strings.TrimSpace(req.Token), req.Show)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (h *HTTPServer) handleReplayAutoUploadConfigWithToken(w http.ResponseWriter, token string, show *bool) {
	if token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad-token"})
		return
	}
	runtime := h.state.SnapshotRuntime()
	endpoint := runtime.Config.PhiraAPIEndpoint
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	me, err := FetchPhiraUserInfo(endpoint, token, runtime.Config.OutboundProxy)
	if err != nil || me == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	if show != nil && h.state.AutoUploadConfigs != nil {
		h.state.AutoUploadConfigs.SetShow(me.ID, *show)
	}
	currentShow := false
	if h.state.AutoUploadConfigs != nil {
		currentShow = h.state.AutoUploadConfigs.GetShow(me.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                     true,
		"userId":                 me.ID,
		"show":                   currentShow,
		"shareStationConfigured": shareStationConfigured(runtime.Config),
		"autoUploadEnabled":      config.DerefBool(runtime.Config.ReplayAutoUpload, false),
	})
}

func shareStationConfigured(cfg *config.ServerConfig) bool {
	return cfg != nil && cfg.ShareStation != nil && cfg.ShareStation.URL != "" && cfg.ShareStation.Token != ""
}
