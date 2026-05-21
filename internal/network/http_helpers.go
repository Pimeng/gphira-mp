// Common HTTP helpers shared across http.go, http_admin*.go, http_replay.go,
// and http_admin_auth.go. Keeping requireMethod/writeError/decodeJSONBody and
// the session-TTL constants centralized here is what lets each handler file
// stay a thin, single-purpose layer.
package network

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Shared HTTP session/token TTLs used across replay/admin/OTP flows.
const (
	replaySessionTTL  = 30 * time.Minute
	otpTTL            = time.Minute
	tempAdminTokenTTL = 4 * time.Hour
)

// requireMethod writes 405 and returns false when r.Method does not match any
// of the allowed methods. Use at the top of single-method handlers to replace
// the `if r.Method != ... { http.Error(...) }` boilerplate.
func requireMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if r.Method == m {
			return true
		}
	}
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	return false
}

// writeError writes a standard error JSON body: {"ok": false, "error": code}.
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]any{"ok": false, "error": code})
}

// decodeJSONBody decodes the request body into v. On failure it writes a 400
// bad-request error and returns false. Callers that accept an absent/invalid
// body (e.g. handleAdminOTPRequest) must NOT use this helper.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad-request")
		return false
	}
	return true
}

// newHTTPToken returns a 16-byte random token formatted as a UUID-like string.
// Falls back to a timestamp-derived string if the entropy source fails.
func newHTTPToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}
