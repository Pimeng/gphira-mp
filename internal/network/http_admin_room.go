package network

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

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
		writeError(w, http.StatusBadRequest, "invalid-room-id")
		return
	}

	// POST /admin/rooms/:id/max_users
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "max_users" {
		var req struct {
			MaxUsers int `json:"maxUsers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid-json")
			return
		}
		if req.MaxUsers < 1 || req.MaxUsers > 64 {
			writeError(w, http.StatusBadRequest, "bad-max-users")
			return
		}
		var updated bool
		h.state.WithLock(func() {
			room := h.state.Rooms[rid]
			if room == nil {
				return
			}
			room.SetMaxUsers(req.MaxUsers)
			updated = true
		})
		if !updated {
			writeError(w, http.StatusNotFound, "room-not-found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "roomid": string(rid), "max_users": req.MaxUsers})
		return
	}

	// POST /admin/rooms/:id/disband
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "disband" {
		var room *game.Room
		h.state.WithRLock(func() {
			room = h.state.Rooms[rid]
		})
		if room == nil {
			writeError(w, http.StatusNotFound, "room-not-found")
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
			writeError(w, http.StatusBadRequest, "invalid-json")
			return
		}
		msg := strings.TrimSpace(req.Message)
		if msg == "" {
			writeError(w, http.StatusBadRequest, "empty-message")
			return
		}
		if len(msg) > 200 {
			writeError(w, http.StatusBadRequest, "message-too-long")
			return
		}

		var room *game.Room
		h.state.WithRLock(func() {
			room = h.state.Rooms[rid]
		})
		if room == nil {
			writeError(w, http.StatusNotFound, "room-not-found")
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
