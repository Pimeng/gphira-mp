package network

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/state"
)

const (
	adminMaxFailedAttemptsPerIP = 5
	otpMaxAttempts              = 3
)

type otpSession struct {
	OTP       string
	ExpiresAt int64
}

type adminAuthResult struct {
	OK     bool
	Status int
	Error  string
}

func (h *HTTPServer) ensureAdminAuthLocked() {
	if h.adminFailedAttemptsByIP == nil {
		h.adminFailedAttemptsByIP = make(map[string]int)
	}
	if h.adminBannedIPs == nil {
		h.adminBannedIPs = make(map[string]struct{})
	}
}

func (h *HTTPServer) ensureOTPStateLocked() {
	if h.otpSessions == nil {
		h.otpSessions = make(map[string]otpSession)
	}
	if h.otpAttemptsByIP == nil {
		h.otpAttemptsByIP = make(map[string]int)
	}
	if h.otpAttemptsBySSID == nil {
		h.otpAttemptsBySSID = make(map[string]int)
	}
	if h.otpBannedIPs == nil {
		h.otpBannedIPs = make(map[string]struct{})
	}
	if h.otpBannedSSIDs == nil {
		h.otpBannedSSIDs = make(map[string]struct{})
	}
}

func (h *HTTPServer) cleanupOTPSessionsLocked(now int64) {
	h.ensureOTPStateLocked()
	for ssid, sess := range h.otpSessions {
		if now > sess.ExpiresAt {
			delete(h.otpSessions, ssid)
			delete(h.otpAttemptsBySSID, ssid)
		}
	}
}

func (h *HTTPServer) cleanupExpiringAdminState(now int64) {
	h.otpMu.Lock()
	h.cleanupOTPSessionsLocked(now)
	h.otpMu.Unlock()

	h.state.WithLock(func() {
		for token, data := range h.state.TempAdminTokens {
			if data == nil || now > data.ExpiresAt {
				delete(h.state.TempAdminTokens, token)
			}
		}
		for ssid, data := range h.state.CLIApprovalSessions {
			if data == nil || now > data.ExpiresAt {
				delete(h.state.CLIApprovalSessions, ssid)
			}
		}
	})
}

func (h *HTTPServer) checkAdminRequest(r *http.Request) adminAuthResult {
	return h.checkAdminToken(requestAdminToken(r), h.clientIPFromRequest(r))
}

func (h *HTTPServer) adminTokenOK(r *http.Request) bool {
	return h.checkAdminRequest(r).OK
}

func (h *HTTPServer) adminTokenStringOK(token, clientIP string) bool {
	return h.checkAdminToken(strings.TrimSpace(token), clientIP).OK
}

func (h *HTTPServer) checkAdminToken(token, clientIP string) adminAuthResult {
	h.adminAuthMu.Lock()
	h.ensureAdminAuthLocked()
	if _, banned := h.adminBannedIPs[clientIP]; banned {
		h.adminAuthMu.Unlock()
		return adminAuthResult{Status: http.StatusUnauthorized, Error: "unauthorized"}
	}
	h.adminAuthMu.Unlock()

	expected := strings.TrimSpace(h.state.SnapshotRuntime().Config.AdminToken)
	if expected != "" && token == expected {
		h.adminAuthMu.Lock()
		delete(h.adminFailedAttemptsByIP, clientIP)
		h.adminAuthMu.Unlock()
		return adminAuthResult{OK: true, Status: http.StatusOK}
	}

	now := time.Now().UnixMilli()
	h.cleanupExpiringAdminState(now)

	if token != "" {
		var tempOK bool
		var tempRejected bool
		h.state.WithLock(func() {
			temp := h.state.TempAdminTokens[token]
			if temp == nil {
				return
			}
			tempRejected = true
			if temp.Banned {
				return
			}
			if now > temp.ExpiresAt {
				delete(h.state.TempAdminTokens, token)
				return
			}
			if temp.IP != clientIP {
				temp.Banned = true
				return
			}
			tempOK = true
			tempRejected = false
		})
		if tempOK {
			return adminAuthResult{OK: true, Status: http.StatusOK}
		}
		if tempRejected {
			return adminAuthResult{Status: http.StatusUnauthorized, Error: "token-expired"}
		}
	}

	if expected == "" {
		return adminAuthResult{Status: http.StatusForbidden, Error: "admin-disabled"}
	}

	h.adminAuthMu.Lock()
	h.ensureAdminAuthLocked()
	next := h.adminFailedAttemptsByIP[clientIP] + 1
	h.adminFailedAttemptsByIP[clientIP] = next
	if next >= adminMaxFailedAttemptsPerIP {
		h.adminBannedIPs[clientIP] = struct{}{}
	}
	h.adminAuthMu.Unlock()
	return adminAuthResult{Status: http.StatusUnauthorized, Error: "unauthorized"}
}

func requestAdminToken(r *http.Request) string {
	token := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	if token != "" {
		return token
	}
	token = strings.TrimSpace(extractBearerToken(r.Header.Get("Authorization")))
	if token != "" {
		return token
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func extractBearerToken(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) > len("Bearer ") && strings.EqualFold(trimmed[:len("Bearer ")], "Bearer ") {
		return strings.TrimSpace(trimmed[len("Bearer "):])
	}
	return trimmed
}

func (h *HTTPServer) clientIPFromRequest(r *http.Request) string {
	runtime := h.state.SnapshotRuntime()
	headerName := strings.TrimSpace(runtime.Config.RealIPHeader)
	if headerName == "" {
		headerName = "X-Forwarded-For"
	}
	if raw := strings.TrimSpace(r.Header.Get(headerName)); raw != "" {
		first := strings.TrimSpace(strings.Split(raw, ",")[0])
		if first != "" {
			return normalizeClientIP(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return normalizeClientIP(host)
}

func normalizeClientIP(ip string) string {
	ip = strings.TrimSpace(ip)
	ip = strings.TrimPrefix(ip, "::ffff:")
	return ip
}

func (h *HTTPServer) handleAdminOTPRequest(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if strings.TrimSpace(h.state.SnapshotRuntime().Config.AdminToken) != "" {
		writeError(w, http.StatusForbidden, "otp-disabled-when-token-configured")
		return
	}

	mode := "otp"
	var req struct {
		Mode string `json:"mode"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if m := strings.ToLower(strings.TrimSpace(req.Mode)); m == "cli" || m == "otp" {
				mode = m
			}
		}
	}

	now := time.Now()
	h.cleanupExpiringAdminState(now.UnixMilli())
	ssid := newHTTPToken()
	expiresAt := now.Add(otpTTL).UnixMilli()
	clientIP := h.clientIPFromRequest(r)

	if mode == "cli" {
		h.state.WithLock(func() {
			h.state.CLIApprovalSessions[ssid] = &state.CLIApprovalSession{
				IP:          clientIP,
				ExpiresAt:   expiresAt,
				Status:      "pending",
				RequestedAt: now.UnixMilli(),
			}
		})
		h.printOTPLog("INFO", fmt.Sprintf("[OTP CLI Request] admin approval requested from %s, session %s, valid for 1 minute", clientIP, ssid))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ssid": ssid, "expiresIn": otpTTL.Milliseconds(), "mode": "cli"})
		return
	}

	otp := newHTTPToken()[:8]
	h.otpMu.Lock()
	h.ensureOTPStateLocked()
	h.otpSessions[ssid] = otpSession{OTP: otp, ExpiresAt: expiresAt}
	h.otpMu.Unlock()
	h.printOTPLog("INFO", fmt.Sprintf("[OTP Request] admin OTP is %s, session %s, valid for 1 minute", otp, ssid))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ssid": ssid, "expiresIn": otpTTL.Milliseconds(), "mode": "otp"})
}

func (h *HTTPServer) handleAdminOTPVerify(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if strings.TrimSpace(h.state.SnapshotRuntime().Config.AdminToken) != "" {
		writeError(w, http.StatusForbidden, "otp-disabled-when-token-configured")
		return
	}

	var req struct {
		SSID string `json:"ssid"`
		OTP  string `json:"otp"`
		Mode string `json:"mode"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	ssid := strings.TrimSpace(req.SSID)
	if ssid == "" {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}
	mode := "otp"
	if m := strings.ToLower(strings.TrimSpace(req.Mode)); m == "cli" || m == "otp" {
		mode = m
	}

	now := time.Now().UnixMilli()
	h.cleanupExpiringAdminState(now)
	clientIP := h.clientIPFromRequest(r)
	if mode == "cli" {
		h.verifyAdminCLISession(w, ssid, clientIP, now)
		return
	}

	otp := strings.TrimSpace(req.OTP)
	if otp == "" {
		writeError(w, http.StatusBadRequest, "bad-request")
		return
	}

	h.otpMu.Lock()
	h.ensureOTPStateLocked()
	if _, banned := h.otpBannedIPs[clientIP]; banned {
		h.otpMu.Unlock()
		writeError(w, http.StatusForbidden, "ip-banned-too-many-attempts")
		return
	}
	if _, banned := h.otpBannedSSIDs[ssid]; banned {
		h.otpMu.Unlock()
		writeError(w, http.StatusForbidden, "ssid-banned-too-many-attempts")
		return
	}
	sess, ok := h.otpSessions[ssid]
	if !ok || now > sess.ExpiresAt {
		h.otpMu.Unlock()
		writeError(w, http.StatusUnauthorized, "invalid-or-expired-otp")
		return
	}
	if sess.OTP != otp {
		ipAttempts := h.otpAttemptsByIP[clientIP] + 1
		ssidAttempts := h.otpAttemptsBySSID[ssid] + 1
		h.otpAttemptsByIP[clientIP] = ipAttempts
		h.otpAttemptsBySSID[ssid] = ssidAttempts
		if ipAttempts >= otpMaxAttempts {
			h.otpBannedIPs[clientIP] = struct{}{}
			h.printOTPLog("WARN", fmt.Sprintf("[OTP] IP %s banned after %d failed attempts", clientIP, ipAttempts))
		}
		if ssidAttempts >= otpMaxAttempts {
			h.otpBannedSSIDs[ssid] = struct{}{}
			delete(h.otpSessions, ssid)
			h.printOTPLog("WARN", fmt.Sprintf("[OTP] session %s banned after %d failed attempts", ssid, ssidAttempts))
		}
		h.otpMu.Unlock()
		writeError(w, http.StatusUnauthorized, "invalid-or-expired-otp")
		return
	}
	delete(h.otpAttemptsByIP, clientIP)
	delete(h.otpAttemptsBySSID, ssid)
	delete(h.otpSessions, ssid)
	h.otpMu.Unlock()

	token := newHTTPToken()
	expiresAt := time.Now().Add(tempAdminTokenTTL).UnixMilli()
	h.state.WithLock(func() {
		h.state.TempAdminTokens[token] = &state.TempAdminToken{IP: clientIP, ExpiresAt: expiresAt}
	})
	h.printOTPLog("INFO", fmt.Sprintf("[OTP] temporary admin token issued for %s: %s..., valid for 4 hours", clientIP, token[:8]))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "token": token, "expiresAt": expiresAt, "expiresIn": tempAdminTokenTTL.Milliseconds()})
}

func (h *HTTPServer) verifyAdminCLISession(w http.ResponseWriter, ssid, clientIP string, now int64) {
	var session *state.CLIApprovalSession
	var missing bool
	h.state.WithLock(func() {
		session = h.state.CLIApprovalSessions[ssid]
		if session == nil || now > session.ExpiresAt {
			delete(h.state.CLIApprovalSessions, ssid)
			missing = true
		}
	})
	if missing || session == nil {
		writeError(w, http.StatusUnauthorized, "invalid-or-expired-session")
		return
	}
	if session.IP != clientIP {
		writeError(w, http.StatusForbidden, "ip-mismatch")
		return
	}
	switch session.Status {
	case "pending", "":
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": false, "error": "pending-approval", "status": "pending"})
	case "denied":
		h.state.WithLock(func() {
			delete(h.state.CLIApprovalSessions, ssid)
		})
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "approval-denied", "status": "denied"})
	case "approved":
		if session.Token == "" || session.TokenExpiresAt == 0 {
			h.state.WithLock(func() {
				delete(h.state.CLIApprovalSessions, ssid)
			})
			writeError(w, http.StatusInternalServerError, "token-not-issued")
			return
		}
		h.state.WithLock(func() {
			delete(h.state.CLIApprovalSessions, ssid)
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"token":     session.Token,
			"expiresAt": session.TokenExpiresAt,
			"expiresIn": maxInt64(0, session.TokenExpiresAt-now),
			"mode":      "cli",
		})
	default:
		writeError(w, http.StatusInternalServerError, "invalid-approval-status")
	}
}

func (h *HTTPServer) printOTPLog(level, message string) {
	fmt.Fprintf(os.Stdout, "[%s] [%s] %s\n", time.Now().Format(time.RFC3339), level, message)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
