package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
)

// HTTPServer provides HTTP endpoints for the Phira MP server.
type HTTPServer struct {
	server  *http.Server
	state   *state.ServerState
	logger  *utils.Logger
	wsServer *WSServer
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
	mux.HandleFunc("/status", h.handleStatus)
	mux.HandleFunc("/chart/", h.handleChartProxy)
	h.registerAdminRoutes(mux)
	mux.HandleFunc("/replay/download", h.adminAuthMiddleware(h.handleReplayDownload))

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

	logger.Mark(state.ServerLang.Format("log-http-started", map[string]string{"addr": addr}))
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	h.logger.Debug("http request", "path", "/room", "remote", r.RemoteAddr)

	type playerInfo struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	}

	type roomInfo struct {
		RoomID  string       `json:"roomid"`
		Cycle   bool         `json:"cycle"`
		Lock    bool         `json:"lock"`
		Host    playerInfo   `json:"host"`
		State   string       `json:"state"`
		Chart   *struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"chart,omitempty"`
		Players []playerInfo `json:"players"`
	}

	var rooms []roomInfo
	var total int

	h.state.WithRLock(func() {
		for rid, room := range h.state.Rooms {
			if strings.HasPrefix(string(rid), "_") {
				continue
			}

			hostUser := h.state.Users[room.HostID]
			hostName := ""
			if hostUser != nil {
				hostName = hostUser.Name
			}

			var players []playerInfo
			for _, uid := range room.UserIDs() {
				total++
				u := h.state.Users[uid]
				name := ""
				if u != nil {
					name = u.Name
				}
				players = append(players, playerInfo{ID: uid, Name: name})
			}

			stateStr := "select_chart"
			switch room.State.(type) {
			case *game.StateWaitForReady:
				stateStr = "waiting_for_ready"
			case *game.StatePlaying:
				stateStr = "playing"
			}

			var chart *struct {
				Name string `json:"name"`
				ID   string `json:"id"`
			}
			if room.Chart != nil {
				chart = &struct {
					Name string `json:"name"`
					ID   string `json:"id"`
				}{
					Name: room.Chart.Name,
					ID:   fmt.Sprintf("%d", room.Chart.ID),
				}
			}

			rooms = append(rooms, roomInfo{
				RoomID:  string(rid),
				Cycle:   room.Cycle,
				Lock:    room.Locked,
				Host:    playerInfo{ID: room.HostID, Name: hostName},
				State:   stateStr,
				Chart:   chart,
				Players: players,
			})
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"rooms": rooms,
		"total": total,
	})
}

func (h *HTTPServer) handleRoomCreationConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": h.state.RoomCreationEnabled,
	})
}

func (h *HTTPServer) handleReplayConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"enabled": h.state.ReplayEnabled,
	})
}

func (h *HTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	h.logger.Debug("http request", "path", "/status", "remote", r.RemoteAddr)

	var onlineCount int
	var roomCount int
	h.state.WithRLock(func() {
		onlineCount = len(h.state.Users)
		roomCount = len(h.state.Rooms)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"server_name": h.state.ServerName,
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}


func (h *HTTPServer) handleChartProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
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
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-chart-id"})
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
	endpoint := h.state.Config.PhiraAPIEndpoint
	if endpoint == "" {
		endpoint = defaultPhiraAPIEndpoint
	}
	chart, err := FetchPhiraChart(endpoint, int32(id), h.state.Config.OutboundProxy)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": "chart-fetch-failed"})
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

func (h *HTTPServer) handleReplayDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
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

	path := replay.ReplayFilePath(h.state.Config.ReplayBaseDir, int32(userID), int32(chartID), timestamp)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "not-found"})
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%d.phirarec"`, timestamp))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	http.ServeFile(w, r, path)
}
