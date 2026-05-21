package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// adminAuthMiddleware checks the configured admin token or a temporary admin token.
func (h *HTTPServer) adminAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := h.checkAdminRequest(r)
		if !result.OK {
			writeError(w, result.Status, result.Error)
			return
		}
		next(w, r)
	}
}

func (h *HTTPServer) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin/status", h.adminAuthMiddleware(h.handleAdminStatus))
	mux.HandleFunc("/admin/replay/config", h.adminAuthMiddleware(h.handleAdminReplayConfig))
	mux.HandleFunc("/admin/room-creation/config", h.adminAuthMiddleware(h.handleAdminRoomCreationConfig))
	mux.HandleFunc("/admin/broadcast", h.adminAuthMiddleware(h.handleAdminBroadcast))
	mux.HandleFunc("/admin/rooms", h.adminAuthMiddleware(h.handleAdminRooms))
	mux.HandleFunc("/admin/rooms/", h.adminAuthMiddleware(h.handleAdminRoomDetail))
	mux.HandleFunc("/admin/users", h.adminAuthMiddleware(h.handleAdminUsers))
	mux.HandleFunc("/admin/users/", h.adminAuthMiddleware(h.handleAdminUserDetail))
	mux.HandleFunc("/admin/sessions", h.adminAuthMiddleware(h.handleAdminSessions))
	mux.HandleFunc("/admin/logs", h.adminAuthMiddleware(h.handleAdminLogs))
	mux.HandleFunc("/admin/log-rate", h.adminAuthMiddleware(h.handleAdminLogRate))
	mux.HandleFunc("/admin/ban/user", h.adminAuthMiddleware(h.handleAdminBanUser))
	mux.HandleFunc("/admin/ban/room", h.adminAuthMiddleware(h.handleAdminBanRoom))
	mux.HandleFunc("/admin/ip-blacklist", h.adminAuthMiddleware(h.handleAdminIPBlacklist))
	mux.HandleFunc("/admin/ip-blacklist/remove", h.adminAuthMiddleware(h.handleAdminIPBlacklistRemove))
	mux.HandleFunc("/admin/ip-blacklist/clear", h.adminAuthMiddleware(h.handleAdminIPBlacklistClear))
	mux.HandleFunc("/admin/contest/rooms/", h.adminAuthMiddleware(h.handleAdminContest))
}

// decodeEnabled extracts the `enabled` field from a JSON request body, returning
// (value, ok). The boolean ok signals successful parse; the actual value is
// truthy under typical JSON conventions (true / non-empty string / non-zero number).
func decodeEnabled(r *http.Request) (bool, bool) {
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return false, false
	}
	v, ok := raw["enabled"]
	if !ok {
		return false, false
	}
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		return x != "", true
	case float64:
		return x != 0, true
	case nil:
		return false, true
	default:
		return true, true
	}
}

func (h *HTTPServer) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-json")
		return
	}

	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		writeError(w, http.StatusBadRequest, "bad-message")
		return
	}
	if len(msg) > 200 {
		writeError(w, http.StatusBadRequest, "message-too-long")
		return
	}

	cmd := protocol.ServerCommand{
		Type: protocol.ServerCmdMessage,
		Message: protocol.Message{
			Type:    protocol.MessageChat,
			User:    0,
			Content: msg,
		},
	}

	var roomIDs []roomid.RoomID
	h.state.WithRLock(func() {
		roomIDs = make([]roomid.RoomID, 0, len(h.state.Rooms))
		for rid := range h.state.Rooms {
			roomIDs = append(roomIDs, rid)
		}
	})
	for _, rid := range roomIDs {
		var room *game.Room
		h.state.WithRLock(func() {
			room = h.state.Rooms[rid]
		})
		if room != nil {
			room.AddLog(msg)
			_ = h.broadcastRoomAll(rid, cmd)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rooms": len(roomIDs)})
}

func (h *HTTPServer) handleAdminRooms(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	details := buildAdminRoomsData(h.state)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"total_rooms": len(details),
		"rooms":       details,
	})
}

func (h *HTTPServer) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
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
	if !requireMethod(w, r, http.MethodGet) {
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

// broadcastRoomAll sends a server command to every participant and monitor in a
// room, using a read-lock. Errors from individual user.TrySend are ignored.
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

func (h *HTTPServer) findUser(id int32) *game.User {
	var u *game.User
	h.state.WithRLock(func() {
		u = h.state.Users[id]
	})
	return u
}

func (h *HTTPServer) abortPlayingUserAndCheckReady(user *game.User, room *game.Room) {
	if room == nil {
		return
	}
	err := room.SetAborted(user.ID)
	if err != nil {
		return
	}
	_ = h.broadcastRoomAll(room.ID, protocol.ServerCommand{
		Type:    protocol.ServerCmdMessage,
		Message: protocol.Message{Type: protocol.MessageAbort, User: user.ID},
	})
	h.checkRoomAllReady(room)
}

func (h *HTTPServer) checkRoomAllReady(room *game.Room) {
	runtime := h.state.SnapshotRuntime()
	participantIDs := room.AllParticipantIDs()
	userIDs := room.UserIDs()
	monitorIDs := room.MonitorIDs()
	_ = room.CheckAllReady(&game.RoomCallbacks{
		UsersById: h.findUser,
		Broadcast: func(cmd protocol.ServerCommand) error {
			for _, id := range participantIDs {
				if u := h.findUser(id); u != nil {
					_ = u.TrySend(cmd)
				}
			}
			return nil
		},
		BroadcastToMonitors: func(cmd protocol.ServerCommand) {
			for _, id := range monitorIDs {
				if u := h.findUser(id); u != nil {
					_ = u.TrySend(cmd)
				}
			}
		},
		PickRandomUserId: utils.RandomPickInt32,
		Lang:             runtime.ServerLang,
		Logger:           h.state.Logger,
		OnEnterPlaying: func(r *game.Room) {
			if runtime.ReplayEnabled && h.state.ReplayRecorder != nil && r.ReplayEligible {
				var participants []replay.Participant
				for _, uid := range userIDs {
					name := ""
					if u := h.findUser(uid); u != nil {
						name = u.GetName()
					}
					participants = append(participants, replay.Participant{ID: uid, Name: name})
				}
				chartID := 0
				chartName := ""
				if r.Chart != nil {
					chartID = r.Chart.ID
					chartName = r.Chart.Name
				}
				h.state.ReplayRecorder.StartRoom(r.ID, chartID, chartName, participants)
			}
		},
		OnGameEnd: func(r *game.Room) {
			if runtime.ReplayEnabled && h.state.ReplayRecorder != nil && r.ReplayEligible {
				h.state.ReplayRecorder.EndRoom(r.ID)
			}
			if h.state.AutoUploadCallback != nil && r.Chart != nil {
				if playing, ok := r.State.(*game.StatePlaying); ok {
					chartID := int32(r.Chart.ID)
					var roomFiles []replay.FileInfo
					if h.state.ReplayRecorder != nil {
						roomFiles = h.state.ReplayRecorder.ListRoomFiles(r.ID)
					}
					for userID, recordData := range playing.Results {
						for _, fi := range roomFiles {
							if fi.UserID == userID {
								h.state.AutoUploadCallback(userID, chartID, fi.Timestamp, recordData.ID)
								break
							}
						}
					}
				}
			}
			if h.state.ReplayRecorder != nil {
				h.state.ReplayRecorder.ClearRoomFiles(r.ID)
			}
		},
		DisbandRoom: func(r *game.Room) {
			h.state.WithLock(func() {
				delete(h.state.Rooms, r.ID)
			})
		},
		NotifyWebSocket: func(rid roomid.RoomID) {
			if h.state.WSServer != nil {
				h.state.WSServer.BroadcastRoomUpdate(rid, nil)
			}
		},
	})
}
