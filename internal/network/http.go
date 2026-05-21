package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// HTTPServer provides HTTP endpoints for the Phira MP server.
type HTTPServer struct {
	server   *http.Server
	state    *state.ServerState
	logger   *utils.Logger
	wsServer *WSServer

	replayMu       sync.Mutex
	replaySessions map[string]replaySession

	adminAuthMu             sync.Mutex
	adminFailedAttemptsByIP map[string]int
	adminBannedIPs          map[string]struct{}
	otpMu                   sync.Mutex
	otpSessions             map[string]otpSession
	otpAttemptsByIP         map[string]int
	otpAttemptsBySSID       map[string]int
	otpBannedIPs            map[string]struct{}
	otpBannedSSIDs          map[string]struct{}
}

// StartHTTPServer starts the HTTP service on the given address.
func StartHTTPServer(addr string, state *state.ServerState, logger *utils.Logger) (*HTTPServer, error) {
	h := &HTTPServer{
		state:  state,
		logger: logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/room", h.handleRoomList)
	mux.HandleFunc("/room-creation/config", h.handleRoomCreationConfig)
	mux.HandleFunc("/replay/config", h.handleReplayConfig)
	mux.HandleFunc("/replay/auth", h.handleReplayAuth)
	mux.HandleFunc("/replay/delete", h.handleReplayDelete)
	mux.HandleFunc("/replay/upload", h.handleReplayUpload)
	mux.HandleFunc("/replay/auto-upload/config", h.handleReplayAutoUploadConfig)
	mux.HandleFunc("/admin/otp/request", h.handleAdminOTPRequest)
	mux.HandleFunc("/admin/otp/verify", h.handleAdminOTPVerify)
	mux.HandleFunc("/status", h.handleStatus)
	mux.HandleFunc("/chart/", h.handleChartProxy)
	h.registerAdminRoutes(mux)
	mux.HandleFunc("/replay/download", h.handleReplayDownload)

	h.wsServer = NewWSServer(h)
	h.wsServer.Start()
	state.WSServer = h.wsServer
	mux.Handle("/ws", h.wsServer)

	h.server = &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(mux),
	}

	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "err", err)
		}
	}()

	runtime := state.SnapshotRuntime()
	logger.Mark(runtime.ServerLang.Format("log-http-started", map[string]string{"addr": addr}))
	return h, nil
}

// Close gracefully shuts down the HTTP server.
func (h *HTTPServer) Close() error {
	if h.wsServer != nil {
		h.wsServer.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.server.Shutdown(ctx)
}

func (h *HTTPServer) handleRoomList(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	h.logger.Debug("http request", "path", "/room", "remote", r.RemoteAddr)

	type playerInfo struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	}

	type hostInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	type roomInfo struct {
		RoomID string   `json:"roomid"`
		Cycle  bool     `json:"cycle"`
		Lock   bool     `json:"lock"`
		Host   hostInfo `json:"host"`
		State  string   `json:"state"`
		Chart  *struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"chart,omitempty"`
		Players []playerInfo `json:"players"`
	}

	var roomRefs []struct {
		rid  roomid.RoomID
		room *game.Room
	}
	var total int
	users := map[int32]string{}

	h.state.WithRLock(func() {
		for rid, room := range h.state.Rooms {
			roomRefs = append(roomRefs, struct {
				rid  roomid.RoomID
				room *game.Room
			}{rid: rid, room: room})
		}
		for id, user := range h.state.Users {
			users[id] = user.GetName()
		}
	})

	var rooms []roomInfo
	for _, ref := range roomRefs {
		if strings.HasPrefix(string(ref.rid), "_") {
			continue
		}
		snap := ref.room.Snapshot()
		hostName := users[snap.HostID]
		if hostName == "" {
			hostName = fmt.Sprintf("%d", snap.HostID)
		}
		players := make([]playerInfo, 0, len(snap.Users))
		for _, uid := range snap.Users {
			name := users[uid]
			if name == "" {
				name = fmt.Sprintf("%d", uid)
			}
			players = append(players, playerInfo{ID: uid, Name: name})
		}
		total += len(players)

		var chart *struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		}
		if snap.Chart != nil {
			chart = &struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			}{
				Name: snap.Chart.Name,
				ID:   fmt.Sprintf("%d", snap.Chart.ID),
			}
		}

		rooms = append(rooms, roomInfo{
			RoomID:  string(ref.rid),
			Cycle:   snap.Cycle,
			Lock:    snap.Locked,
			Host:    hostInfo{ID: fmt.Sprintf("%d", snap.HostID), Name: hostName},
			State:   snap.State.Type,
			Chart:   chart,
			Players: players,
		})
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].RoomID < rooms[j].RoomID })

	writeJSON(w, http.StatusOK, map[string]any{
		"rooms": rooms,
		"total": total,
	})
}

func (h *HTTPServer) handleRoomCreationConfig(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	runtime := h.state.SnapshotRuntime()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": runtime.RoomCreationEnabled,
	})
}

func (h *HTTPServer) handleReplayConfig(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	runtime := h.state.SnapshotRuntime()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": runtime.ReplayEnabled,
	})
}

func (h *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	h.logger.Debug("http request", "path", "/status", "remote", r.RemoteAddr)

	var onlineCount int
	var roomCount int
	h.state.WithRLock(func() {
		onlineCount = len(h.state.Users)
		roomCount = len(h.state.Rooms)
	})

	runtime := h.state.SnapshotRuntime()
	writeJSON(w, http.StatusOK, map[string]any{
		"server_name": runtime.ServerName,
		"online":      onlineCount,
		"rooms":       roomCount,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Admin-Token")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (h *HTTPServer) handleChartProxy(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	h.logger.Debug("http request", "path", r.URL.Path, "remote", r.RemoteAddr)

	prefix := "/chart/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, prefix)
	id, err := strconv.ParseInt(idStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid-chart-id")
		return
	}

	// Try cache first
	if h.state.ChartCache != nil {
		if cached := h.state.ChartCache.Get(int32(id)); cached != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":    true,
				"id":    cached.ID,
				"name":  cached.Name,
				"cache": true,
			})
			return
		}
	}

	// Fetch from Phira API
	runtime := h.state.SnapshotRuntime()
	endpoint := runtime.Config.PhiraAPIEndpoint
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	chart, err := FetchPhiraChart(endpoint, int32(id), runtime.Config.OutboundProxy)
	if err != nil {
		writeError(w, http.StatusBadGateway, "chart-fetch-failed")
		return
	}

	if h.state.ChartCache != nil {
		h.state.ChartCache.Set(chart.ID, chart.Name)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"id":    chart.ID,
		"name":  chart.Name,
		"cache": false,
	})
}
