package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func (h *HTTPServer) handleAdminContest(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
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
		writeError(w, http.StatusNotFound, "invalid-path")
		return
	}

	ridStr, err := roomid.Parse(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid-room-id")
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
		writeError(w, http.StatusNotFound, "invalid-path")
	}
}

func (h *HTTPServer) handleAdminContestConfig(w http.ResponseWriter, r *http.Request, rid roomid.RoomID) {
	var req struct {
		Enabled   bool  `json:"enabled"`
		Whitelist []int `json:"whitelist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-json")
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
		}
		for _, id := range room.UserIDs() {
			whitelist[id] = struct{}{}
		}
		for _, id := range room.MonitorIDs() {
			whitelist[id] = struct{}{}
		}
		room.SetContest(&game.ContestConfig{
			Whitelist:   whitelist,
			ManualStart: true,
			AutoDisband: true,
		})
		ok = true
	})

	if !ok {
		writeError(w, http.StatusNotFound, "room-not-found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) handleAdminContestWhitelist(w http.ResponseWriter, r *http.Request, rid roomid.RoomID) {
	var req struct {
		UserIDs []int `json:"userIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-json")
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
		writeError(w, http.StatusNotFound, "contest-room-not-found")
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
		writeError(w, status, result.state)
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
		if runtime.ReplayEnabled && h.state.ReplayRecorder != nil && room.ReplayEligible && room.Chart != nil {
			var participants []replay.Participant
			for _, uid := range users {
				name := fmt.Sprintf("%d", uid)
				if u := h.state.Users[uid]; u != nil {
					name = u.GetName()
				}
				participants = append(participants, replay.Participant{ID: uid, Name: name})
			}
			h.state.ReplayRecorder.StartRoom(room.ID, room.Chart.ID, room.Chart.Name, participants)
		}
	})
	if err := room.ForceStartPlaying(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_ = h.broadcastRoomAll(rid, protocol.ServerCommand{
		Type:  protocol.ServerCmdChangeState,
		State: room.ClientRoomState(),
	})
	if h.state.WSServer != nil {
		h.state.WSServer.BroadcastRoomUpdate(rid, nil)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
