package test

import (
	"os"
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func TestReplayRecorderRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	logger := utils.NewLogger("INFO")
	rec := replay.NewRecorder(tmpDir, logger)

	roomID := roomid.RoomID("test-room")
	users := []replay.Participant{
		{ID: 1, Name: "Alice"},
		{ID: 2, Name: "Bob"},
	}

	rec.StartRoom(roomID, 42, "Test Chart", users)

	// Append touches and judges
	rec.AppendTouches(roomID, 1, []protocol.TouchFrame{
		{Time: 1.0, Points: []protocol.TouchPoint{{ID: 0, Pos: protocol.CompactPos{X: 0.5, Y: 0.5}}}},
		{Time: 2.0, Points: []protocol.TouchPoint{{ID: 0, Pos: protocol.CompactPos{X: 0.6, Y: 0.6}}}},
	})
	rec.AppendJudges(roomID, 1, []protocol.JudgeEvent{
		{Time: 1.5, LineID: 0, NoteID: 10, Judgement: protocol.JudgementPerfect},
	})

	rec.SetRecordID(roomID, 1, 1001)
	rec.SetRecordID(roomID, 2, 1002)

	// End room
	rec.EndRoom(roomID)

	// Check files exist
	for _, u := range users {
		files := rec.ListRoomFiles(roomID)
		found := false
		for _, f := range files {
			if f.UserID == u.ID {
				found = true
				if _, err := os.Stat(f.Path); os.IsNotExist(err) {
					t.Errorf("replay file not found: %s", f.Path)
				}
				break
			}
		}
		if !found {
			t.Errorf("no replay file for user %d", u.ID)
		}
	}
}

func TestReplayRecorderClearRoomFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logger := utils.NewLogger("INFO")
	rec := replay.NewRecorder(tmpDir, logger)

	roomID := roomid.RoomID("clear-room")
	users := []replay.Participant{{ID: 1, Name: "Alice"}}

	rec.StartRoom(roomID, 1, "Chart", users)
	rec.EndRoom(roomID)

	if len(rec.ListRoomFiles(roomID)) == 0 {
		t.Fatal("expected files after end room")
	}

	rec.ClearRoomFiles(roomID)
	if len(rec.ListRoomFiles(roomID)) != 0 {
		t.Error("expected no files after clear")
	}
}

func TestReplayFilePath(t *testing.T) {
	path := replay.ReplayFilePath("/base", 1, 42, 1234567890)
	// ReplayFilePath uses forward slashes consistently
	expected := "/base/1/42/1234567890.phirarec"
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}
