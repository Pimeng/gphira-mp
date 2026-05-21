// /admin/ban/(user|room) — manage server-wide and per-room user bans.
// Mutations are persisted via state.SaveAdminData so bans survive restart.
package network

import (
	"encoding/json"
	"net/http"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func (h *HTTPServer) handleAdminBanUser(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		UserID     int32 `json:"userId"`
		Banned     bool  `json:"banned"`
		Disconnect bool  `json:"disconnect"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-json")
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
		var user *game.User
		var sess *Session
		h.state.WithRLock(func() {
			if u := h.state.Users[req.UserID]; u != nil {
				user = u
				if s, ok := u.GetSession().(*Session); ok {
					sess = s
				}
			}
		})
		if sess != nil {
			if user != nil && user.GetRoom() != nil {
				h.abortPlayingUserAndCheckReady(user, user.GetRoom())
			}
			sess.adminDisconnect(true)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) handleAdminBanRoom(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		UserID int32  `json:"userId"`
		RoomID string `json:"roomId"`
		Banned bool   `json:"banned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-json")
		return
	}

	rid, err := roomid.Parse(req.RoomID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid-room-id")
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
