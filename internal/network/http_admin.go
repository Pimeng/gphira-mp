package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// adminAuthMiddleware checks the X-Admin-Token header against the configured admin token.
func (h *HTTPServer) adminAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Admin-Token")
		if token == "" {
			// Also check query parameter for websocket compatibility
			token = r.URL.Query().Get("token")
		}
		runtime := h.state.SnapshotRuntime()
		expected := runtime.Config.AdminToken
		if expected == "" || token != expected {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"ok":    false,
				"error": "unauthorized",
			})
			return
		}
		next(w, r)
	}
}

func (h *HTTPServer) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin/status", h.adminAuthMiddleware(h.handleAdminStatus))
	mux.HandleFunc("/admin/broadcast", h.adminAuthMiddleware(h.handleAdminBroadcast))
	mux.HandleFunc("/admin/rooms", h.adminAuthMiddleware(h.handleAdminRooms))
	mux.HandleFunc("/admin/rooms/", h.adminAuthMiddleware(h.handleAdminRoomDetail))
	mux.HandleFunc("/admin/users", h.adminAuthMiddleware(h.handleAdminUsers))
	mux.HandleFunc("/admin/users/", h.adminAuthMiddleware(h.handleAdminUserDetail))
	mux.HandleFunc("/admin/sessions", h.adminAuthMiddleware(h.handleAdminSessions))
	mux.HandleFunc("/admin/logs", h.adminAuthMiddleware(h.handleAdminLogs))
	mux.HandleFunc("/admin/ban/user", h.adminAuthMiddleware(h.handleAdminBanUser))
	mux.HandleFunc("/admin/ban/room", h.adminAuthMiddleware(h.handleAdminBanRoom))
	mux.HandleFunc("/admin/ip-blacklist", h.adminAuthMiddleware(h.handleAdminIPBlacklist))
	mux.HandleFunc("/admin/ip-blacklist/remove", h.adminAuthMiddleware(h.handleAdminIPBlacklistRemove))
	mux.HandleFunc("/admin/ip-blacklist/clear", h.adminAuthMiddleware(h.handleAdminIPBlacklistClear))
	mux.HandleFunc("/admin/contest/rooms/", h.adminAuthMiddleware(h.handleAdminContest))
}

func (h *HTTPServer) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var onlineCount, roomCount, sessionCount int
	var rooms []string
	h.state.WithRLock(func() {
		onlineCount = len(h.state.Users)
		roomCount = len(h.state.Rooms)
		sessionCount = len(h.state.Sessions)
		for rid := range h.state.Rooms {
			rooms = append(rooms, string(rid))
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"server_name": h.state.SnapshotRuntime().ServerName,
		"online":      onlineCount,
		"rooms":       roomCount,
		"sessions":    sessionCount,
		"room_ids":    rooms,
	})
}

func (h *HTTPServer) handleAdminBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-json"})
		return
	}

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty-message"})
		return
	}

	cmd := protocol.ServerCommand{
		Type: protocol.ServerCmdMessage,
		Message: protocol.Message{
			Type:    protocol.MessageChat,
			User:    0,
			Content: "[Broadcast] " + msg,
		},
	}

	h.state.WithRLock(func() {
		for _, sess := range h.state.Sessions {
			if s, ok := sess.(*Session); ok {
				_ = s.Send(cmd)
			}
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) handleAdminRooms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	type contestInfo struct {
		WhitelistCount int32   `json:"whitelist_count"`
		Whitelist      []int32 `json:"whitelist"`
		ManualStart    bool    `json:"manual_start"`
		AutoDisband    bool    `json:"auto_disband"`
	}

	type roomDetail struct {
		ID       string       `json:"id"`
		HostID   int32        `json:"host_id"`
		Users    []int32      `json:"users"`
		Monitors []int32      `json:"monitors"`
		State    string       `json:"state"`
		Locked   bool         `json:"locked"`
		Cycle    bool         `json:"cycle"`
		Contest  *contestInfo `json:"contest,omitempty"`
	}

	var details []roomDetail
	h.state.WithRLock(func() {
		for rid, room := range h.state.Rooms {
			stateStr := "select_chart"
			switch room.State.(type) {
			case *game.StateWaitForReady:
				stateStr = "waiting_for_ready"
			case *game.StatePlaying:
				stateStr = "playing"
			}

			d := roomDetail{
				ID:       string(rid),
				HostID:   room.HostID,
				Users:    room.UserIDs(),
				Monitors: room.MonitorIDs(),
				State:    stateStr,
				Locked:   room.Locked,
				Cycle:    room.Cycle,
			}
			if room.Contest != nil {
				whitelist := make([]int32, 0, len(room.Contest.Whitelist))
				for id := range room.Contest.Whitelist {
					whitelist = append(whitelist, id)
				}
				sort.Slice(whitelist, func(i, j int) bool { return whitelist[i] < whitelist[j] })
				d.Contest = &contestInfo{
					WhitelistCount: int32(len(whitelist)),
					Whitelist:      whitelist,
					ManualStart:    room.Contest.ManualStart,
					AutoDisband:    room.Contest.AutoDisband,
				}
			}
			details = append(details, d)
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"rooms": details,
	})
}

func (h *HTTPServer) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	type userInfo struct {
		ID       int32  `json:"id"`
		Name     string `json:"name"`
		RoomID   string `json:"room_id,omitempty"`
		Monitor  bool   `json:"monitor"`
		RemoteIP string `json:"remote_ip,omitempty"`
	}

	var users []userInfo
	h.state.WithRLock(func() {
		for _, u := range h.state.Users {
			info := userInfo{
				ID:      u.ID,
				Name:    u.GetName(),
				Monitor: u.IsMonitor(),
			}
			if u.GetRoom() != nil {
				info.RoomID = string(u.GetRoom().ID)
			}
			if sess, ok := u.GetSession().(*Session); ok {
				info.RemoteIP = sess.RemoteIP
			}
			users = append(users, info)
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"users": users,
	})
}

func (h *HTTPServer) handleAdminSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	type sessionInfo struct {
		ID       string `json:"id"`
		UserID   int32  `json:"user_id,omitempty"`
		UserName string `json:"user_name,omitempty"`
		RemoteIP string `json:"remote_ip"`
	}

	var sessions []sessionInfo
	h.state.WithRLock(func() {
		for _, sess := range h.state.Sessions {
			if s, ok := sess.(*Session); ok {
				info := sessionInfo{
					ID:       s.ID,
					RemoteIP: s.RemoteIP,
				}
				if s.user != nil {
					info.UserID = s.user.ID
					info.UserName = s.user.GetName()
				}
				sessions = append(sessions, info)
			}
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"sessions": sessions,
	})
}

func (h *HTTPServer) handleAdminContest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	prefix := "/admin/contest/rooms/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "invalid-path"})
		return
	}

	ridStr, err := roomid.Parse(parts[0])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-room-id"})
		return
	}

	subPath := parts[1]

	switch subPath {
	case "config":
		h.handleAdminContestConfig(w, r, ridStr)
	case "whitelist":
		h.handleAdminContestWhitelist(w, r, ridStr)
	case "start":
		h.handleAdminContestStart(w, r, ridStr)
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "invalid-path"})
	}
}

func (h *HTTPServer) handleAdminContestConfig(w http.ResponseWriter, r *http.Request, rid roomid.RoomID) {
	var req struct {
		Enabled   bool  `json:"enabled"`
		Whitelist []int `json:"whitelist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-json"})
		return
	}

	var ok bool
	h.state.WithLock(func() {
		room, exists := h.state.Rooms[rid]
		if !exists {
			return
		}
		if !req.Enabled {
			room.ClearContest()
			ok = true
			return
		}
		whitelist := make(map[int32]struct{})
		if len(req.Whitelist) > 0 {
			for _, id := range req.Whitelist {
				whitelist[int32(id)] = struct{}{}
			}
		} else {
			for _, id := range room.UserIDs() {
				whitelist[id] = struct{}{}
			}
			for _, id := range room.MonitorIDs() {
				whitelist[id] = struct{}{}
			}
		}
		room.SetContest(&game.ContestConfig{
			Whitelist:   whitelist,
			ManualStart: true,
			AutoDisband: true,
		})
		ok = true
	})

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "room-not-found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) handleAdminContestWhitelist(w http.ResponseWriter, r *http.Request, rid roomid.RoomID) {
	var req struct {
		UserIDs []int `json:"userIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-json"})
		return
	}

	var ok bool
	h.state.WithLock(func() {
		room, exists := h.state.Rooms[rid]
		if !exists || room.Contest == nil {
			return
		}
		whitelist := make(map[int32]struct{})
		for _, id := range req.UserIDs {
			whitelist[int32(id)] = struct{}{}
		}
		for _, id := range room.UserIDs() {
			whitelist[id] = struct{}{}
		}
		for _, id := range room.MonitorIDs() {
			whitelist[id] = struct{}{}
		}
		_ = room.SetContestWhitelist(whitelist)
		ok = true
	})

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "contest-room-not-found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) handleAdminContestStart(w http.ResponseWriter, r *http.Request, rid roomid.RoomID) {
	var req struct {
		Force bool `json:"force"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	var result struct {
		ok    bool
		room  *game.Room
		state string
	}
	h.state.WithLock(func() {
		room, exists := h.state.Rooms[rid]
		if !exists || room.Contest == nil {
			result.state = "contest-room-not-found"
			return
		}
		st, ok := room.State.(*game.StateWaitForReady)
		if !ok {
			result.state = "room-not-waiting"
			return
		}
		if room.Chart == nil {
			result.state = "no-chart-selected"
			return
		}
		allIds := room.AllParticipantIDs()
		allReady := true
		for _, id := range allIds {
			if _, ok := st.Started[id]; !ok {
				allReady = false
				break
			}
		}
		if !allReady && !req.Force {
			result.state = "not-all-ready"
			return
		}
		result.ok = true
		result.room = room
	})

	if !result.ok {
		status := http.StatusNotFound
		if result.state == "room-not-waiting" || result.state == "no-chart-selected" || result.state == "not-all-ready" {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]any{"ok": false, "error": result.state})
		return
	}

	room := result.room
	users := room.UserIDs()
	monitors := room.MonitorIDs()

	runtime := h.state.SnapshotRuntime()
	if h.state.Logger != nil && runtime.ServerLang != nil {
		sep := ", "
		if runtime.ServerLang.Format("lang-check", nil) == "zh" {
			sep = "、"
		}
		usersText := joinInt32s(users, sep)
		var monitorsSuffix string
		if len(monitors) > 0 {
			monitorsText := joinInt32s(monitors, sep)
			monitorsSuffix = runtime.ServerLang.Format("log-room-game-start-monitors", map[string]string{"monitors": monitorsText})
		}
		h.state.Logger.Info(runtime.ServerLang.Format("log-room-game-start", map[string]string{"users": usersText, "monitorsSuffix": monitorsSuffix}))
	}

	_ = h.broadcastRoomAll(rid, protocol.ServerCommand{
		Type:    protocol.ServerCmdMessage,
		Message: protocol.Message{Type: protocol.MessageStartPlaying},
	})

	h.state.WithLock(func() {
		for _, uid := range users {
			if u := h.state.Users[uid]; u != nil {
				u.ResetGameTime()
			}
		}
	})
	if err := room.ForceStartPlaying(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	_ = h.broadcastRoomAll(rid, protocol.ServerCommand{
		Type:  protocol.ServerCmdChangeState,
		State: room.ClientRoomState(),
	})

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) broadcastRoomAll(rid roomid.RoomID, cmd protocol.ServerCommand) error {
	h.state.WithRLock(func() {
		room, ok := h.state.Rooms[rid]
		if !ok {
			return
		}
		for _, uid := range room.UserIDs() {
			if u := h.state.Users[uid]; u != nil {
				_ = u.TrySend(cmd)
			}
		}
		for _, mid := range room.MonitorIDs() {
			if u := h.state.Users[mid]; u != nil {
				_ = u.TrySend(cmd)
			}
		}
	})
	return nil
}

func joinInt32s(ids []int32, sep string) string {
	if len(ids) == 0 {
		return ""
	}
	var parts []string
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, sep)
}

// ========== Admin Logs ==========

func (h *HTTPServer) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	roomIDStr := r.URL.Query().Get("roomId")
	if roomIDStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "missing-room-id"})
		return
	}

	rid, err := roomid.Parse(roomIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-room-id"})
		return
	}

	var logs []game.RoomLog
	h.state.WithRLock(func() {
		if room, ok := h.state.Rooms[rid]; ok {
			logs = room.GetRecentLogs()
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"logs": logs,
	})
}

// ========== Admin Ban ==========

func (h *HTTPServer) handleAdminBanUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID     int32 `json:"userId"`
		Banned     bool  `json:"banned"`
		Disconnect bool  `json:"disconnect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-json"})
		return
	}

	h.state.WithLock(func() {
		if req.Banned {
			h.state.BannedUsers[req.UserID] = struct{}{}
		} else {
			delete(h.state.BannedUsers, req.UserID)
		}
	})
	_ = h.state.SaveAdminData()

	if req.Disconnect && req.Banned {
		var sess *Session
		h.state.WithRLock(func() {
			if u := h.state.Users[req.UserID]; u != nil {
				if s, ok := u.GetSession().(*Session); ok {
					sess = s
				}
			}
		})
		if sess != nil {
			sess.adminDisconnect(true)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) handleAdminBanRoom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		UserID int32  `json:"userId"`
		RoomID string `json:"roomId"`
		Banned bool   `json:"banned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-json"})
		return
	}

	rid, err := roomid.Parse(req.RoomID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-room-id"})
		return
	}

	h.state.WithLock(func() {
		set, ok := h.state.BannedRoomUsers[rid]
		if !ok {
			set = make(map[int32]struct{})
		}
		if req.Banned {
			set[req.UserID] = struct{}{}
		} else {
			delete(set, req.UserID)
		}
		if len(set) > 0 {
			h.state.BannedRoomUsers[rid] = set
		} else {
			delete(h.state.BannedRoomUsers, rid)
		}
	})
	_ = h.state.SaveAdminData()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ========== Admin IP Blacklist ==========

func (h *HTTPServer) handleAdminIPBlacklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var blacklist []struct {
		IP        string `json:"ip"`
		ExpiresIn int64  `json:"expiresIn"`
	}
	if h.state.Logger != nil {
		for _, entry := range h.state.Logger.GetBlacklistedIPs() {
			blacklist = append(blacklist, struct {
				IP        string `json:"ip"`
				ExpiresIn int64  `json:"expiresIn"`
			}{IP: entry.IP, ExpiresIn: entry.ExpiresIn})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"blacklist": blacklist,
	})
}

func (h *HTTPServer) handleAdminIPBlacklistRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-json"})
		return
	}

	if h.state.Logger != nil {
		h.state.Logger.RemoveFromBlacklist(req.IP)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) handleAdminIPBlacklistClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.state.Logger != nil {
		h.state.Logger.ClearBlacklist()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ========== Admin User Detail ==========

func (h *HTTPServer) handleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	prefix := "/admin/users/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.SplitN(rest, "/", 2)
	var userID int32
	if _, err := fmt.Sscanf(parts[0], "%d", &userID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-user-id"})
		return
	}

	// GET /admin/users/:id
	if r.Method == http.MethodGet && len(parts) == 1 {
		var out map[string]any
		h.state.WithRLock(func() {
			u := h.state.Users[userID]
			if u == nil {
				return
			}
			connected := u.HasSession()
			roomID := ""
			if u.GetRoom() != nil {
				roomID = string(u.GetRoom().ID)
			}
			_, banned := h.state.BannedUsers[userID]
			out = map[string]any{
				"id":        u.ID,
				"name":      u.GetName(),
				"monitor":   u.IsMonitor(),
				"connected": connected,
				"room":      roomID,
				"banned":    banned,
			}
		})
		if out == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "user-not-found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": out})
		return
	}

	// POST /admin/users/:id/disconnect
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "disconnect" {
		var sess *Session
		h.state.WithRLock(func() {
			if u := h.state.Users[userID]; u != nil {
				if s, ok := u.GetSession().(*Session); ok {
					sess = s
				}
			}
		})
		if sess == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "user-not-connected"})
			return
		}
		sess.adminDisconnect(false)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	http.Error(w, "Not Found", http.StatusNotFound)
}

// ========== Admin Room Detail ==========

func (h *HTTPServer) handleAdminRoomDetail(w http.ResponseWriter, r *http.Request) {
	prefix := "/admin/rooms/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, prefix)
	parts := strings.SplitN(rest, "/", 2)
	rid, err := roomid.Parse(parts[0])
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-room-id"})
		return
	}

	// POST /admin/rooms/:id/disband
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "disband" {
		var room *game.Room
		h.state.WithRLock(func() {
			room = h.state.Rooms[rid]
		})
		if room == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "room-not-found"})
			return
		}
		runtime := h.state.SnapshotRuntime()

		// Disconnect all participants
		allIDs := room.AllParticipantIDs()
		for _, uid := range allIDs {
			var sess *Session
			h.state.WithRLock(func() {
				if u := h.state.Users[uid]; u != nil {
					if s, ok := u.GetSession().(*Session); ok {
						sess = s
					}
				}
			})
			if sess != nil {
				sess.adminDisconnect(false)
			}
		}

		h.state.WithLock(func() {
			delete(h.state.Rooms, rid)
		})

		if runtime.ReplayEnabled && h.state.ReplayRecorder != nil && room.ReplayEligible {
			h.state.ReplayRecorder.EndRoom(rid)
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "roomid": string(rid)})
		return
	}

	// POST /admin/rooms/:id/chat
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "chat" {
		var req struct {
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid-json"})
			return
		}
		msg := strings.TrimSpace(req.Message)
		if msg == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "empty-message"})
			return
		}

		var room *game.Room
		h.state.WithRLock(func() {
			room = h.state.Rooms[rid]
		})
		if room == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "room-not-found"})
			return
		}

		room.AddLog(msg)
		cmd := protocol.ServerCommand{
			Type: protocol.ServerCmdMessage,
			Message: protocol.Message{
				Type:    protocol.MessageChat,
				User:    0,
				Content: msg,
			},
		}
		for _, uid := range room.AllParticipantIDs() {
			if u := h.findUser(uid); u != nil {
				_ = u.TrySend(cmd)
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	http.Error(w, "Not Found", http.StatusNotFound)
}

func (h *HTTPServer) findUser(id int32) *game.User {
	var u *game.User
	h.state.WithRLock(func() {
		u = h.state.Users[id]
	})
	return u
}
