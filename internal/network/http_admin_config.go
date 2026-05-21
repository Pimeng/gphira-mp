// /admin/replay/config and /admin/room-creation/config — runtime toggles for
// the global replay-recording flag and the room-creation gate. Disabling
// replay ends recording on every live room and writes the change back to the
// on-disk config file so it survives restart.
package network

import (
	"net/http"
	"os"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
	"gopkg.in/yaml.v3"
)

func (h *HTTPServer) handleAdminRoomCreationConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		runtime := h.state.SnapshotRuntime()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": runtime.RoomCreationEnabled})
	case http.MethodPost:
		enabled, ok := decodeEnabled(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "bad-enabled")
			return
		}
		h.state.WithLock(func() {
			h.state.RoomCreationEnabled = enabled
			h.state.Config.RoomCreationEnabled = config.Bool(enabled)
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (h *HTTPServer) handleAdminReplayConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		runtime := h.state.SnapshotRuntime()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": runtime.ReplayEnabled})
	case http.MethodPost:
		enabled, ok := decodeEnabled(r)
		if !ok {
			writeError(w, http.StatusBadRequest, "bad-enabled")
			return
		}
		var roomsToEnd []roomid.RoomID
		h.state.WithLock(func() {
			h.state.ReplayEnabled = enabled
			h.state.Config.ReplayEnabled = config.Bool(enabled)
			if !enabled {
				roomsToEnd = make([]roomid.RoomID, 0, len(h.state.Rooms))
				for rid := range h.state.Rooms {
					roomsToEnd = append(roomsToEnd, rid)
				}
			}
			for _, room := range h.state.Rooms {
				room.RefreshLive(enabled)
			}
		})
		if !enabled && h.state.ReplayRecorder != nil {
			for _, rid := range roomsToEnd {
				h.state.ReplayRecorder.EndRoom(rid)
			}
		}
		if err := h.persistReplayEnabled(enabled); err != nil && h.state.Logger != nil {
			h.state.Logger.Warn("failed to persist replay config", "err", err)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (h *HTTPServer) persistReplayEnabled(enabled bool) error {
	if h.state.ConfigPath == "" {
		return nil
	}
	configObj := map[string]any{}
	if data, err := os.ReadFile(h.state.ConfigPath); err == nil && len(data) > 0 {
		_ = yaml.Unmarshal(data, &configObj)
	}
	delete(configObj, "replay_enabled")
	delete(configObj, "replayEnabled")
	configObj["REPLAY_ENABLED"] = enabled
	data, err := yaml.Marshal(configObj)
	if err != nil {
		return err
	}
	return os.WriteFile(h.state.ConfigPath, data, 0644)
}
