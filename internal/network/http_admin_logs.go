package network

import (
	"net/http"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func (h *HTTPServer) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	roomIDStr := r.URL.Query().Get("roomId")
	if roomIDStr == "" {
		writeError(w, http.StatusBadRequest, "missing-room-id")
		return
	}

	rid, err := roomid.Parse(roomIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid-room-id")
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

func (h *HTTPServer) handleAdminLogRate(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	var rate float64
	if h.state.Logger != nil {
		rate = h.state.Logger.GetCurrentRate()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rate": rate})
}
