package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/config"
	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/l10n"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

type adminDataFile struct {
	Version         int                `json:"version"`
	BannedUsers     []int32            `json:"bannedUsers"`
	BannedRoomUsers map[string][]int32 `json:"bannedRoomUsers"`
}

type ServerState struct {
	Config              *config.ServerConfig
	Logger              *utils.Logger
	ServerName          string
	ServerLang          *l10n.Language
	Sessions            map[string]any
	Users               map[int32]*game.User
	Rooms               map[roomid.RoomID]*game.Room
	BannedUsers         map[int32]struct{}
	BannedRoomUsers     map[roomid.RoomID]map[int32]struct{}
	RoomCreationEnabled bool
	ReplayEnabled       bool
	ReplayRecorder      *replay.Recorder
	ChartCache          *utils.ChartCache
	UploadedReplayMeta  *utils.UploadedReplayMeta
	AutoUploadConfigs   *utils.AutoUploadConfigs
	AutoUploadCallback  func(userID int32, chartID int32, timestamp int64, recordID int32)
	WSServer            interface {
		BroadcastRoomUpdate(roomID roomid.RoomID, data any)
		BroadcastRoomLog(roomID roomid.RoomID, message string, timestamp int64)
	}
	mu            sync.RWMutex
	adminDataPath string
}

func NewServerState(cfg *config.ServerConfig, logger *utils.Logger, serverName, adminDataPath string) *ServerState {
	s := &ServerState{
		Config:              cfg,
		Logger:              logger,
		ServerName:          serverName,
		ServerLang:          l10n.New(cfg.Lang),
		Sessions:            make(map[string]any),
		Users:               make(map[int32]*game.User),
		Rooms:               make(map[roomid.RoomID]*game.Room),
		BannedUsers:         make(map[int32]struct{}),
		BannedRoomUsers:     make(map[roomid.RoomID]map[int32]struct{}),
		RoomCreationEnabled: cfg.RoomCreationEnabled,
		ReplayEnabled:       cfg.ReplayEnabled,
		ChartCache:          utils.NewChartCache(200, 60*time.Minute),
		UploadedReplayMeta:  utils.NewUploadedReplayMeta(),
		AutoUploadConfigs:   utils.NewAutoUploadConfigs(),
		adminDataPath:       adminDataPath,
	}
	if s.ServerName == "" {
		s.ServerName = "Phira MP"
	}
	return s
}

func (s *ServerState) ApplyConfig(cfg *config.ServerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Config = cfg
	s.ServerName = cfg.ServerName
	if s.ServerName == "" {
		s.ServerName = "Phira MP"
	}
	s.ServerLang = l10n.New(cfg.Lang)
	s.ReplayEnabled = cfg.ReplayEnabled
	s.RoomCreationEnabled = cfg.RoomCreationEnabled
	if s.Logger != nil {
		s.Logger.DebugL(s.ServerLang, "log-config-applied", map[string]string{"serverName": s.ServerName, "lang": cfg.Lang, "replay": fmt.Sprintf("%v", s.ReplayEnabled), "roomCreation": fmt.Sprintf("%v", s.RoomCreationEnabled)})
	}
}

func (s *ServerState) WithLock(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn()
}

func (s *ServerState) WithRLock(fn func()) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fn()
}

func (s *ServerState) LoadAdminData() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.adminDataPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if s.Logger != nil {
				s.Logger.DebugL(s.ServerLang, "log-admin-data-not-found", map[string]string{"path": s.adminDataPath})
			}
			return nil
		}
		return err
	}

	var file adminDataFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	if file.Version != 1 {
		return fmt.Errorf("unsupported admin data version: %d", file.Version)
	}

	s.BannedUsers = make(map[int32]struct{})
	for _, id := range file.BannedUsers {
		s.BannedUsers[id] = struct{}{}
	}

	if s.Logger != nil {
		s.Logger.DebugL(s.ServerLang, "log-admin-data-loaded", map[string]string{"bannedUsers": fmt.Sprintf("%d", len(s.BannedUsers)), "bannedRoomUsers": fmt.Sprintf("%d", len(file.BannedRoomUsers))})
	}

	s.BannedRoomUsers = make(map[roomid.RoomID]map[int32]struct{})
	for ridStr, ids := range file.BannedRoomUsers {
		rid, err := roomid.Parse(ridStr)
		if err != nil {
			continue
		}
		set := make(map[int32]struct{})
		for _, id := range ids {
			set[id] = struct{}{}
		}
		if len(set) > 0 {
			s.BannedRoomUsers[rid] = set
		}
	}

	return nil
}

func (s *ServerState) SaveAdminData() error {
	s.mu.RLock()

	file := adminDataFile{
		Version:         1,
		BannedUsers:     make([]int32, 0, len(s.BannedUsers)),
		BannedRoomUsers: make(map[string][]int32),
	}

	for id := range s.BannedUsers {
		file.BannedUsers = append(file.BannedUsers, id)
	}

	for rid, set := range s.BannedRoomUsers {
		ids := make([]int32, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			file.BannedRoomUsers[rid.String()] = ids
		}
	}

	s.mu.RUnlock()

	dir := filepath.Dir(s.adminDataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := s.adminDataPath + ".tmp"
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, s.adminDataPath); err != nil {
		_ = os.Remove(s.adminDataPath)
		if err2 := os.Rename(tmpPath, s.adminDataPath); err2 != nil {
			return err2
		}
	}

	return nil
}
