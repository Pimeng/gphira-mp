package network

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func writeTestReplay(t *testing.T, baseDir string, userID, chartID, recordID int32) int64 {
	t.Helper()
	rec := replay.NewRecorder(baseDir, nil)
	rid := roomid.RoomID("replay-room")
	rec.StartRoom(rid, int(chartID), "Test Chart", []replay.Participant{{ID: userID, Name: "Alice"}})
	rec.SetRecordID(rid, userID, recordID)
	rec.EndRoom(rid)
	files := rec.ListRoomFiles(rid)
	if len(files) != 1 {
		t.Fatalf("replay files = %d, want 1", len(files))
	}
	return files[0].Timestamp
}

func TestReplayAuthListsLocalReplays(t *testing.T) {
	h, st := newTestHTTPServer()
	st.Config.ReplayBaseDir = t.TempDir()
	ts := writeTestReplay(t, st.Config.ReplayBaseDir, 100, 7, 123)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me" || r.Header.Get("Authorization") != "Bearer user-token" {
			t.Fatalf("unexpected API request: path=%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": 100, "name": "Alice", "language": "zh-CN"})
	}))
	defer api.Close()
	st.Config.PhiraAPIEndpoint = api.URL

	req := httptest.NewRequest(http.MethodPost, "/replay/auth", strings.NewReader(`{"token":"user-token"}`))
	rec := httptest.NewRecorder()

	h.handleReplayAuth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK           bool   `json:"ok"`
		UserID       int32  `json:"userId"`
		SessionToken string `json:"sessionToken"`
		Charts       []struct {
			ChartID int32 `json:"chartId"`
			Replays []struct {
				Timestamp int64 `json:"timestamp"`
				RecordID  int32 `json:"recordId"`
			} `json:"replays"`
		} `json:"charts"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || body.UserID != 100 || body.SessionToken == "" {
		t.Fatalf("unexpected auth response: %+v", body)
	}
	if len(body.Charts) != 1 || body.Charts[0].ChartID != 7 || len(body.Charts[0].Replays) != 1 {
		t.Fatalf("unexpected charts: %+v", body.Charts)
	}
	if body.Charts[0].Replays[0].Timestamp != ts || body.Charts[0].Replays[0].RecordID != 123 {
		t.Fatalf("unexpected replay entry: %+v", body.Charts[0].Replays[0])
	}
}

func TestReplayDownloadUsesSessionToken(t *testing.T) {
	h, st := newTestHTTPServer()
	st.Config.ReplayBaseDir = t.TempDir()
	ts := writeTestReplay(t, st.Config.ReplayBaseDir, 100, 7, 123)
	sessionToken, _ := h.putReplaySession(100)

	req := httptest.NewRequest(http.MethodGet, "/replay/download?sessionToken="+sessionToken+"&chartId=7&timestamp="+int64String(ts), nil)
	rec := httptest.NewRecorder()

	h.handleReplayDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(got, int64String(ts)+".phirarec") {
		t.Fatalf("content-disposition = %q", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected replay bytes")
	}
}

func TestReplayDeleteRemovesLocalFile(t *testing.T) {
	h, st := newTestHTTPServer()
	st.Config.ReplayBaseDir = t.TempDir()
	ts := writeTestReplay(t, st.Config.ReplayBaseDir, 100, 7, 123)
	sessionToken, _ := h.putReplaySession(100)

	req := httptest.NewRequest(http.MethodPost, "/replay/delete", strings.NewReader(`{"sessionToken":"`+sessionToken+`","chartId":7,"timestamp":`+int64String(ts)+`}`))
	rec := httptest.NewRecorder()

	h.handleReplayDelete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if h.serveReplayFile(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), 100, 7, ts) {
		t.Fatal("replay file should have been deleted")
	}
}

func int64String(v int64) string {
	return strconv.FormatInt(v, 10)
}
