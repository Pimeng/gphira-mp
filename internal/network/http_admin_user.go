// /admin/users/:id and its sub-paths (disconnect, move) — read or mutate a
// single user. `move` is the only path that touches game-layer Room state
// directly from the HTTP layer; it deliberately refuses to move users that are
// online or in rooms not in select_chart, to avoid mid-game disruption.
package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

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
		writeError(w, http.StatusBadRequest, "invalid-user-id")
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
			writeError(w, http.StatusNotFound, "user-not-found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": out})
		return
	}

	// POST /admin/users/:id/disconnect
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "disconnect" {
		var user *game.User
		var sess *Session
		h.state.WithRLock(func() {
			if u := h.state.Users[userID]; u != nil {
				user = u
				if s, ok := u.GetSession().(*Session); ok {
					sess = s
				}
			}
		})
		if sess == nil {
			writeError(w, http.StatusNotFound, "user-not-connected")
			return
		}
		if user != nil && user.GetRoom() != nil {
			h.abortPlayingUserAndCheckReady(user, user.GetRoom())
		}
		sess.adminDisconnect(false)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	// POST /admin/users/:id/move
	if r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "move" {
		var req struct {
			RoomID  string `json:"roomId"`
			Monitor bool   `json:"monitor"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid-json")
			return
		}
		targetID, err := roomid.Parse(req.RoomID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad-room-id")
			return
		}

		var user *game.User
		var from *game.Room
		var to *game.Room
		h.state.WithRLock(func() {
			user = h.state.Users[userID]
			if user != nil {
				from = user.GetRoom()
			}
			to = h.state.Rooms[targetID]
		})
		if user == nil {
			writeError(w, http.StatusNotFound, "user-not-found")
			return
		}
		if user.HasSession() {
			writeError(w, http.StatusBadRequest, "user-must-be-disconnected")
			return
		}
		if from == nil {
			writeError(w, http.StatusBadRequest, "user-not-in-room")
			return
		}
		if from.Snapshot().State.Type != "select_chart" {
			writeError(w, http.StatusBadRequest, "cannot-move-while-playing")
			return
		}
		if to == nil {
			writeError(w, http.StatusNotFound, "room-not-found")
			return
		}
		if to.Snapshot().State.Type != "select_chart" {
			writeError(w, http.StatusBadRequest, "target-room-not-idle")
			return
		}

		runtime := h.state.SnapshotRuntime()
		if err := to.ValidateJoin(user.ID, req.Monitor, runtime.Config.Monitors, nil); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !to.AddUser(user.ID, req.Monitor) {
			writeError(w, http.StatusBadRequest, "room-full")
			return
		}

		participantIDs := from.AllParticipantIDs()
		shouldDisband := from.OnUserLeave(user, &game.RoomCallbacks{
			Broadcast: func(cmd protocol.ServerCommand) error {
				for _, id := range participantIDs {
					if u := h.findUser(id); u != nil {
						_ = u.TrySend(cmd)
					}
				}
				return nil
			},
			UsersById:        h.findUser,
			PickRandomUserId: utils.RandomPickInt32,
			Lang:             runtime.ServerLang,
			Logger:           h.state.Logger,
			NotifyWebSocket: func(rid roomid.RoomID) {
				if h.state.WSServer != nil {
					h.state.WSServer.BroadcastRoomUpdate(rid, nil)
				}
			},
		})
		if shouldDisband {
			h.state.WithLock(func() {
				delete(h.state.Rooms, from.ID)
			})
		} else {
			from.RefreshLive(runtime.ReplayEnabled)
		}
		user.SetRoom(to, req.Monitor)
		to.RefreshLive(runtime.ReplayEnabled)
		if h.state.WSServer != nil {
			h.state.WSServer.BroadcastRoomUpdate(to.ID, nil)
		}

		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	http.Error(w, "Not Found", http.StatusNotFound)
}
