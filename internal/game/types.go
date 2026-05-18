package game

// Chart represents a chart/beatmap.
type Chart struct {
	ID   int
	Name string
}

// RecordData represents a game score record.
type RecordData struct {
	ID        int32
	Player    int32
	Score     int
	Perfect   int
	Good      int
	Bad       int
	Miss      int
	MaxCombo  int
	Accuracy  float32
	FullCombo bool
	Std       float32
	StdScore  float32
}

// ContestConfig holds contest mode settings for a room.
type ContestConfig struct {
	Whitelist   map[int32]struct{}
	ManualStart bool
	AutoDisband bool
}

// RoomLog is a single room log entry.
type RoomLog struct {
	Message   string
	Timestamp int64
}
