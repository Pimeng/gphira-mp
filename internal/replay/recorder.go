package replay

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// Participant identifies a replay participant.
type Participant struct {
	ID   int32
	Name string
}

// InFlight holds recording state for a single user in a room.
type InFlight struct {
	RoomKey      string
	UserID       int32
	UserName     string
	ChartID      int32
	ChartName    string
	Timestamp    int64
	RecordID     int32
	Closed       bool
	TouchFrames  []protocol.TouchFrame
	JudgeEvents  []protocol.JudgeEvent
}

// Recorder manages replay recording for all rooms.
type Recorder struct {
	baseDir string
	logger  *utils.Logger

	mu                sync.Mutex
	inflightByKey     map[string]*InFlight
	keysByRoom        map[string]map[string]struct{}
	completedByRoom   map[string][]FileInfo
}

// FileInfo describes a completed replay file.
type FileInfo struct {
	UserID    int32
	ChartID   int32
	Timestamp int64
	Path      string
}

// NewRecorder creates a new replay recorder.
func NewRecorder(baseDir string, logger *utils.Logger) *Recorder {
	return &Recorder{
		baseDir:         baseDir,
		logger:          logger,
		inflightByKey:   make(map[string]*InFlight),
		keysByRoom:      make(map[string]map[string]struct{}),
		completedByRoom: make(map[string][]FileInfo),
	}
}

// SetBaseDir updates the base directory for replay storage.
func (r *Recorder) SetBaseDir(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.baseDir = dir
}

func (r *Recorder) logf(format string, args ...any) {
	if r.logger != nil {
		r.logger.Debug(format, args...)
	}
}

// StartRoom begins recording for all participants in a room.
func (r *Recorder) StartRoom(roomID roomid.RoomID, chartID int, chartName string, users []Participant) {
	r.mu.Lock()
	defer r.mu.Unlock()

	roomKey := string(roomID)
	if existing := r.keysByRoom[roomKey]; len(existing) > 0 {
		r.logf("StartRoom skipped: room %s already has %d recordings", roomKey, len(existing))
		return
	}

	delete(r.completedByRoom, roomKey)
	keys := make(map[string]struct{})
	for _, p := range users {
		if p.ID < 0 {
			continue
		}
		ts := time.Now().UnixMilli()
		key := roomKey + ":" + itoa(int(p.ID))
		r.inflightByKey[key] = &InFlight{
			RoomKey:   roomKey,
			UserID:    p.ID,
			UserName:  p.Name,
			ChartID:   int32(chartID),
			ChartName: chartName,
			Timestamp: ts,
			RecordID:  0,
			Closed:    false,
		}
		keys[key] = struct{}{}
		r.logf("Recording started for user %d in room %s", p.ID, roomKey)
	}
	if len(keys) > 0 {
		r.keysByRoom[roomKey] = keys
	}
}

// EndRoom ends recording for a room and writes all files.
func (r *Recorder) EndRoom(roomID roomid.RoomID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	roomKey := string(roomID)
	keys := r.keysByRoom[roomKey]
	if keys == nil {
		r.logf("EndRoom: no keys found for room %s", roomKey)
		return
	}
	delete(r.keysByRoom, roomKey)

	var completed []FileInfo
	for key := range keys {
		it := r.inflightByKey[key]
		if it == nil {
			continue
		}
		delete(r.inflightByKey, key)
		if it.Closed {
			continue
		}
		it.Closed = true
		if err := r.writeFile(it); err != nil {
			r.logf("Failed to write replay for user %d: %v", it.UserID, err)
			continue
		}
		completed = append(completed, FileInfo{
			UserID:    it.UserID,
			ChartID:   it.ChartID,
			Timestamp: it.Timestamp,
			Path:      ReplayFilePath(r.baseDir, it.UserID, it.ChartID, it.Timestamp),
		})
	}
	if len(completed) > 0 {
		r.completedByRoom[roomKey] = completed
	}
	r.logf("EndRoom completed for %s: %d recordings written", roomKey, len(completed))
}

// CloseAll flushes all in-flight recordings.
func (r *Recorder) CloseAll() {
	r.mu.Lock()
	rooms := make([]string, 0, len(r.keysByRoom))
	for roomKey := range r.keysByRoom {
		rooms = append(rooms, roomKey)
	}
	r.mu.Unlock()

	for _, roomKey := range rooms {
		rid, _ := roomid.Parse(roomKey)
		r.EndRoom(rid)
	}
}

// SetRecordID sets the Phira record ID for a participant.
func (r *Recorder) SetRecordID(roomID roomid.RoomID, userID int32, recordID int32) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := string(roomID) + ":" + itoa(int(userID))
	it := r.inflightByKey[key]
	if it == nil || it.Closed {
		return
	}
	it.RecordID = recordID
}

// AppendTouches appends touch frames for a participant.
func (r *Recorder) AppendTouches(roomID roomid.RoomID, userID int32, frames []protocol.TouchFrame) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := string(roomID) + ":" + itoa(int(userID))
	it := r.inflightByKey[key]
	if it == nil || it.Closed {
		return
	}
	it.TouchFrames = append(it.TouchFrames, frames...)
}

// AppendJudges appends judge events for a participant.
func (r *Recorder) AppendJudges(roomID roomid.RoomID, userID int32, judges []protocol.JudgeEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := string(roomID) + ":" + itoa(int(userID))
	it := r.inflightByKey[key]
	if it == nil || it.Closed {
		return
	}
	it.JudgeEvents = append(it.JudgeEvents, judges...)
}

// ListRoomFiles returns file info for a room's replays.
func (r *Recorder) ListRoomFiles(roomID roomid.RoomID) []FileInfo {
	r.mu.Lock()
	defer r.mu.Unlock()

	roomKey := string(roomID)
	keys := r.keysByRoom[roomKey]
	if keys == nil {
		out := make([]FileInfo, len(r.completedByRoom[roomKey]))
		copy(out, r.completedByRoom[roomKey])
		return out
	}

	var out []FileInfo
	for key := range keys {
		it := r.inflightByKey[key]
		if it == nil {
			continue
		}
		out = append(out, FileInfo{
			UserID:    it.UserID,
			ChartID:   it.ChartID,
			Timestamp: it.Timestamp,
			Path:      ReplayFilePath(r.baseDir, it.UserID, it.ChartID, it.Timestamp),
		})
	}
	return out
}

// ClearRoomFiles removes completed file metadata for a room.
func (r *Recorder) ClearRoomFiles(roomID roomid.RoomID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.completedByRoom, string(roomID))
}

func (r *Recorder) writeFile(it *InFlight) error {
	content := encodeRecordContent(
		it.RecordID,
		it.Timestamp,
		it.ChartID,
		it.ChartName,
		it.UserID,
		it.UserName,
		it.TouchFrames,
		it.JudgeEvents,
	)

	payload, err := compressPayload(content, compressionZstd)
	if err != nil {
		return err
	}

	header := buildRecordHeader(compressionZstd)
	path := ReplayFilePath(r.baseDir, it.UserID, it.ChartID, it.Timestamp)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(header); err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		return err
	}
	return nil
}
