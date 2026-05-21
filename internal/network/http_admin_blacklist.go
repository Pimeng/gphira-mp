// /admin/ip-blacklist (list/remove/clear) — read and mutate the connection-log
// rate limiter's IP blacklist (utils.Logger). Banned IPs are auto-aged-out by
// the logger, not by these endpoints.
package network

import (
	"encoding/json"
	"net/http"
)

func (h *HTTPServer) handleAdminIPBlacklist(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
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
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid-json")
		return
	}

	if h.state.Logger != nil {
		h.state.Logger.RemoveFromBlacklist(req.IP)
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *HTTPServer) handleAdminIPBlacklistClear(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	if h.state.Logger != nil {
		h.state.Logger.ClearBlacklist()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
