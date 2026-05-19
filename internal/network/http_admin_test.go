package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func newTestHTTPServer() (*HTTPServer, *state.ServerState) {
	cfg := config.DefaultConfig()
	cfg.AdminToken = "test-token"
	logger := utils.NewLogger("INFO")
	st := state.NewServerState(cfg, logger, "Test", "./admin.json")
	return &HTTPServer{state: st, logger: logger}, st
}

func TestAdminRoomCreationConfigPost(t *testing.T) {
	h, st := newTestHTTPServer()
	req := httptest.NewRequest(http.MethodPost, "/admin/room-creation/config", strings.NewReader(`{"enabled":false}`))
	rec := httptest.NewRecorder()

	h.handleAdminRoomCreationConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if st.RoomCreationEnabled {
		t.Fatal("room creation should be disabled")
	}
	if config.DerefBool(st.Config.RoomCreationEnabled, true) {
		t.Fatal("state config should track room creation toggle")
	}
}

func TestAdminBroadcastWritesRoomLogsAndCount(t *testing.T) {
	h, st := newTestHTTPServer()
	room := game.NewRoom(roomid.RoomID("room1"), 1, 8, false)
	st.WithLock(func() {
		st.Rooms[room.ID] = room
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/broadcast", strings.NewReader(`{"message":"hello"}`))
	rec := httptest.NewRecorder()

	h.handleAdminBroadcast(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK    bool `json:"ok"`
		Rooms int  `json:"rooms"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || body.Rooms != 1 {
		t.Fatalf("unexpected response: %+v", body)
	}
	logs := room.GetRecentLogs()
	if len(logs) != 1 || logs[0].Message != "hello" {
		t.Fatalf("unexpected logs: %+v", logs)
	}
}

func TestAdminReplayConfigPostRefreshesLive(t *testing.T) {
	h, st := newTestHTTPServer()
	st.ReplayEnabled = true
	st.Config.ReplayEnabled = config.Bool(true)
	room := game.NewRoom(roomid.RoomID("room1"), 1, 8, true)
	room.RefreshLive(true)
	st.WithLock(func() {
		st.Rooms[room.ID] = room
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/replay/config", strings.NewReader(`{"enabled":false}`))
	rec := httptest.NewRecorder()

	h.handleAdminReplayConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if st.ReplayEnabled {
		t.Fatal("replay should be disabled")
	}
	if config.DerefBool(st.Config.ReplayEnabled, true) {
		t.Fatal("state config should track replay toggle")
	}
	if room.Live {
		t.Fatal("replay-only live room should become not live after replay is disabled")
	}
}

func TestAdminRoomMaxUsersPost(t *testing.T) {
	h, st := newTestHTTPServer()
	room := game.NewRoom(roomid.RoomID("room1"), 1, 8, false)
	st.WithLock(func() {
		st.Rooms[room.ID] = room
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/rooms/room1/max_users", strings.NewReader(`{"maxUsers":2}`))
	rec := httptest.NewRecorder()

	h.handleAdminRoomDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if room.MaxUsers != 2 {
		t.Fatalf("max users = %d, want 2", room.MaxUsers)
	}
}

func TestAdminRoomChatRejectsLongMessage(t *testing.T) {
	h, st := newTestHTTPServer()
	room := game.NewRoom(roomid.RoomID("room1"), 1, 8, false)
	st.WithLock(func() {
		st.Rooms[room.ID] = room
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/rooms/room1/chat", strings.NewReader(`{"message":"`+strings.Repeat("x", 201)+`"}`))
	rec := httptest.NewRecorder()
	h.handleAdminRoomDetail(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, "message-too-long") {
		t.Fatalf("body = %s, want message-too-long", got)
	}
}

func TestAdminUserMovePost(t *testing.T) {
	h, st := newTestHTTPServer()
	host1 := game.NewUser(1, "Host1", "zh-CN")
	moving := game.NewUser(2, "Mover", "zh-CN")
	host2 := game.NewUser(3, "Host2", "zh-CN")
	from := game.NewRoom(roomid.RoomID("room1"), 1, 8, false)
	from.AddUser(2, false)
	to := game.NewRoom(roomid.RoomID("room2"), 3, 8, false)
	host1.SetRoom(from, false)
	moving.SetRoom(from, false)
	host2.SetRoom(to, false)
	st.WithLock(func() {
		st.Users[1] = host1
		st.Users[2] = moving
		st.Users[3] = host2
		st.Rooms[from.ID] = from
		st.Rooms[to.ID] = to
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/users/2/move", strings.NewReader(`{"roomId":"room2","monitor":false}`))
	rec := httptest.NewRecorder()

	h.handleAdminUserDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if moving.GetRoom() != to {
		t.Fatal("moved user should point at target room")
	}
	for _, id := range from.UserIDs() {
		if id == 2 {
			t.Fatal("source room should no longer contain moved user")
		}
	}
	found := false
	for _, id := range to.UserIDs() {
		if id == 2 {
			found = true
		}
	}
	if !found {
		t.Fatal("target room should contain moved user")
	}
}

func TestAdminRoomsUsesDetailedTypescriptShape(t *testing.T) {
	h, st := newTestHTTPServer()
	host := game.NewUser(1, "Alice", "zh-CN")
	guest := game.NewUser(2, "Bob", "en-US")
	room := game.NewRoom(roomid.RoomID("room1"), 1, 8, true)
	room.AddUser(2, false)
	room.SetChart(&game.Chart{ID: 42, Name: "Chart"})
	room.AddLog("hello")
	st.WithLock(func() {
		st.Users[1] = host
		st.Users[2] = guest
		st.Rooms[room.ID] = room
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/rooms", nil)
	rec := httptest.NewRecorder()

	h.handleAdminRooms(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK         bool `json:"ok"`
		TotalRooms int  `json:"total_rooms"`
		Rooms      []struct {
			RoomID       string `json:"roomid"`
			MaxUsers     int    `json:"max_users"`
			CurrentUsers int    `json:"current_users"`
			Host         struct {
				ID   int32  `json:"id"`
				Name string `json:"name"`
			} `json:"host"`
			State struct {
				Type string `json:"type"`
			} `json:"state"`
			Chart *struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"chart"`
			Users []struct {
				ID     int32 `json:"id"`
				IsHost bool  `json:"is_host"`
			} `json:"users"`
			RecentLogs []game.RoomLog `json:"recent_logs"`
		} `json:"rooms"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || body.TotalRooms != 1 || len(body.Rooms) != 1 {
		t.Fatalf("unexpected room list: %+v", body)
	}
	got := body.Rooms[0]
	if got.RoomID != "room1" || got.MaxUsers != 8 || got.CurrentUsers != 2 {
		t.Fatalf("unexpected room summary: %+v", got)
	}
	if got.Host.ID != 1 || got.Host.Name != "Alice" {
		t.Fatalf("unexpected host: %+v", got.Host)
	}
	if got.State.Type != "select_chart" {
		t.Fatalf("state = %q, want select_chart", got.State.Type)
	}
	if got.Chart == nil || got.Chart.ID != 42 || got.Chart.Name != "Chart" {
		t.Fatalf("unexpected chart: %+v", got.Chart)
	}
	if len(got.Users) != 2 || !got.Users[0].IsHost {
		t.Fatalf("unexpected users: %+v", got.Users)
	}
	if len(got.RecentLogs) != 1 || got.RecentLogs[0].Message != "hello" {
		t.Fatalf("unexpected logs: %+v", got.RecentLogs)
	}
}

func TestBuildRoomUpdateDataMatchesTypescriptShape(t *testing.T) {
	_, st := newTestHTTPServer()
	host := game.NewUser(1, "Alice", "zh-CN")
	monitor := game.NewUser(3, "Monitor", "zh-CN")
	room := game.NewRoom(roomid.RoomID("room1"), 1, 8, false)
	room.AddUser(3, true)
	room.SetChart(&game.Chart{ID: 7, Name: "Song"})
	room.State = &game.StateWaitForReady{Started: map[int32]struct{}{1: {}}}
	st.WithLock(func() {
		st.Users[1] = host
		st.Users[3] = monitor
		st.Rooms[room.ID] = room
	})

	data := buildRoomUpdateData(st, room.ID)
	if data == nil {
		t.Fatal("room update data should not be nil")
	}
	if data.RoomID != "room1" || data.State != "waiting_for_ready" {
		t.Fatalf("unexpected update basics: %+v", data)
	}
	if data.Chart == nil || data.Chart.ID != 7 {
		t.Fatalf("unexpected chart: %+v", data.Chart)
	}
	if len(data.Users) != 1 || !data.Users[0].IsReady {
		t.Fatalf("unexpected users: %+v", data.Users)
	}
	if len(data.Monitors) != 1 || data.Monitors[0].Name != "Monitor" {
		t.Fatalf("unexpected monitors: %+v", data.Monitors)
	}
}

func TestCorsAllowsAdminTokenHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/admin/rooms", nil)
	rec := httptest.NewRecorder()
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should be handled by CORS middleware")
	}))

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers")); !strings.Contains(got, "x-admin-token") {
		t.Fatalf("allow headers = %q, want x-admin-token", got)
	}
}

func TestAdminOTPVerifyIssuesTempToken(t *testing.T) {
	h, st := newTestHTTPServer()
	st.Config.AdminToken = ""

	req := httptest.NewRequest(http.MethodPost, "/admin/otp/request", strings.NewReader(`{}`))
	req.RemoteAddr = "203.0.113.10:1234"
	rec := httptest.NewRecorder()
	h.handleAdminOTPRequest(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("request status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var requestBody struct {
		SSID string `json:"ssid"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&requestBody); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if requestBody.SSID == "" || requestBody.Mode != "otp" {
		t.Fatalf("unexpected OTP request body: %+v", requestBody)
	}

	h.otpMu.Lock()
	otp := h.otpSessions[requestBody.SSID].OTP
	h.otpMu.Unlock()
	if otp == "" {
		t.Fatal("otp should be stored for verification")
	}

	verifyReq := httptest.NewRequest(http.MethodPost, "/admin/otp/verify", strings.NewReader(fmt.Sprintf(`{"ssid":"%s","otp":"%s"}`, requestBody.SSID, otp)))
	verifyReq.RemoteAddr = "203.0.113.10:1234"
	verifyRec := httptest.NewRecorder()
	h.handleAdminOTPVerify(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200, body=%s", verifyRec.Code, verifyRec.Body.String())
	}
	var verifyBody struct {
		OK    bool   `json:"ok"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(verifyRec.Body).Decode(&verifyBody); err != nil {
		t.Fatalf("decode verify body: %v", err)
	}
	if !verifyBody.OK || verifyBody.Token == "" {
		t.Fatalf("unexpected verify body: %+v", verifyBody)
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/admin/status", nil)
	adminReq.RemoteAddr = "203.0.113.10:1234"
	adminReq.Header.Set("X-Admin-Token", verifyBody.Token)
	if !h.adminTokenOK(adminReq) {
		t.Fatal("temporary token should authorize admin requests from the same IP")
	}
}

func TestAdminTempTokenRejectsIPMismatch(t *testing.T) {
	h, st := newTestHTTPServer()
	st.Config.AdminToken = ""
	st.WithLock(func() {
		st.TempAdminTokens["temp-token"] = &state.TempAdminToken{
			IP:        "203.0.113.10",
			ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/status", nil)
	req.RemoteAddr = "203.0.113.11:5678"
	req.Header.Set("X-Admin-Token", "temp-token")
	if h.adminTokenOK(req) {
		t.Fatal("temporary token should not authorize a different IP")
	}

	var banned bool
	st.WithRLock(func() {
		banned = st.TempAdminTokens["temp-token"].Banned
	})
	if !banned {
		t.Fatal("temporary token should be banned after IP mismatch")
	}
}

func TestAdminLogRate(t *testing.T) {
	h, _ := newTestHTTPServer()
	limiter := utils.NewRateLimiter(10, time.Second, time.Minute)
	limiter.ShouldLogConnection("203.0.113.10")
	h.logger.SetRateLimiter(limiter)

	req := httptest.NewRequest(http.MethodGet, "/admin/log-rate", nil)
	rec := httptest.NewRecorder()
	h.handleAdminLogRate(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK   bool    `json:"ok"`
		Rate float64 `json:"rate"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || body.Rate <= 0 {
		t.Fatalf("unexpected log rate body: %+v", body)
	}
}
