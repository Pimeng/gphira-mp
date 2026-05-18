package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

type InternalRoomState interface {
	isInternalRoomState()
}

type StateSelectChart struct{}

func (s *StateSelectChart) isInternalRoomState() {}

type StateWaitForReady struct {
	Started map[int32]struct{}
}

func (s *StateWaitForReady) isInternalRoomState() {}

type StatePlaying struct {
	Results map[int32]*RecordData
	Aborted map[int32]struct{}
}

func (s *StatePlaying) isInternalRoomState() {}

type Room struct {
	ID             roomid.RoomID
	MaxUsers       int
	ReplayEligible bool
	HostID         int32
	State          InternalRoomState
	Live           bool
	Locked         bool
	Cycle          bool
	Contest        *ContestConfig
	Chart          *Chart
	users          map[int32]struct{}
	monitors       map[int32]struct{}
	recentLogs     []RoomLog
	mu             sync.RWMutex
}

const maxRecentLogs = 50

func NewRoom(id roomid.RoomID, hostID int32, maxUsers int, replayEligible bool) *Room {
	r := &Room{
		ID:             id,
		MaxUsers:       maxUsers,
		ReplayEligible: replayEligible,
		HostID:         hostID,
		State:          &StateSelectChart{},
		users:          make(map[int32]struct{}),
		monitors:       make(map[int32]struct{}),
	}
	r.users[hostID] = struct{}{}
	return r
}

func (r *Room) AddUser(userID int32, monitor bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if monitor {
		r.monitors[userID] = struct{}{}
		return true
	}
	if len(r.users) >= r.MaxUsers {
		return false
	}
	r.users[userID] = struct{}{}
	return true
}

func (r *Room) RemoveUser(userID int32, monitor bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if monitor {
		delete(r.monitors, userID)
	} else {
		delete(r.users, userID)
	}
}

func (r *Room) userIDsLocked() []int32 {
	ids := make([]int32, 0, len(r.users))
	for id := range r.users {
		ids = append(ids, id)
	}
	return ids
}

func (r *Room) monitorIDsLocked() []int32 {
	ids := make([]int32, 0, len(r.monitors))
	for id := range r.monitors {
		ids = append(ids, id)
	}
	return ids
}

func (r *Room) allParticipantIDsLocked() []int32 {
	ids := make([]int32, 0, len(r.users)+len(r.monitors))
	for id := range r.users {
		ids = append(ids, id)
	}
	for id := range r.monitors {
		ids = append(ids, id)
	}
	return ids
}

func (r *Room) UserIDs() []int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.userIDsLocked()
}

func (r *Room) MonitorIDs() []int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.monitorIDsLocked()
}

func (r *Room) AllParticipantIDs() []int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.allParticipantIDsLocked()
}

func (r *Room) IsHost(userID int32) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.HostID == userID
}

func (r *Room) CheckHost(userID int32) error {
	if !r.IsHost(userID) {
		return errors.New("room-only-host")
	}
	return nil
}

func (r *Room) clientRoomStateLocked() protocol.RoomState {
	switch s := r.State.(type) {
	case *StateSelectChart:
		var chartID *int32
		if r.Chart != nil {
			id := int32(r.Chart.ID)
			chartID = &id
		}
		return protocol.RoomState{Type: protocol.RoomStateSelectChart, ChartID: chartID}
	case *StateWaitForReady:
		return protocol.RoomState{Type: protocol.RoomStateWaitingForReady}
	case *StatePlaying:
		return protocol.RoomState{Type: protocol.RoomStatePlaying}
	default:
		_ = s
		return protocol.RoomState{Type: protocol.RoomStateSelectChart}
	}
}

func (r *Room) ClientRoomState() protocol.RoomState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clientRoomStateLocked()
}

func (r *Room) ClientState(userID int32, users map[int32]*User) protocol.ClientRoomState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	userInfos := make(map[int32]protocol.UserInfo)
	for id := range r.users {
		if u, ok := users[id]; ok && u != nil {
			userInfos[id] = u.ToInfo()
		}
	}
	for id := range r.monitors {
		if u, ok := users[id]; ok && u != nil {
			userInfos[id] = u.ToInfo()
		}
	}

	isReady := false
	if s, ok := r.State.(*StateWaitForReady); ok {
		_, isReady = s.Started[userID]
	}

	return protocol.ClientRoomState{
		ID:      r.ID,
		State:   r.clientRoomStateLocked(),
		Live:    r.Live,
		Locked:  r.Locked,
		Cycle:   r.Cycle,
		IsHost:  r.HostID == userID,
		IsReady: isReady,
		Users:   userInfos,
	}
}

func (r *Room) ValidateJoin(userID int32, monitor bool, monitors []int, state InternalRoomState) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.Contest != nil {
		if _, ok := r.Contest.Whitelist[userID]; !ok {
			return errors.New("room-not-whitelisted")
		}
	}

	if r.Locked {
		return errors.New("join-room-locked")
	}

	if !monitor {
		if _, ok := r.State.(*StateWaitForReady); ok {
			return errors.New("join-game-ongoing")
		}
	} else {
		found := false
		for _, m := range monitors {
			if int(userID) == m {
				found = true
				break
			}
		}
		if !found {
			return errors.New("join-cant-monitor")
		}
	}

	_ = state
	return nil
}

func (r *Room) HandleJoin(userID int32, state InternalRoomState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.State.(*StatePlaying); ok {
		s.Aborted[userID] = struct{}{}
	}
	_ = state
}

func (r *Room) ValidateStart(userID int32) error {
	if err := r.CheckHost(userID); err != nil {
		return err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Chart == nil {
		return errors.New("start-no-chart-selected")
	}
	if _, ok := r.State.(*StateSelectChart); !ok {
		return errors.New("room-invalid-state")
	}
	return nil
}

func (r *Room) ValidateSelectChart(userID int32) error {
	if err := r.CheckHost(userID); err != nil {
		return err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.State.(*StateSelectChart); !ok {
		return errors.New("room-invalid-state")
	}
	return nil
}

func (r *Room) resetGameTimeLocked(usersById func(int32) *User) {
	for id := range r.users {
		if u := usersById(id); u != nil {
			u.GameTime = -1e9
		}
	}
}

func (r *Room) CheckAllReady(callbacks *RoomCallbacks) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch s := r.State.(type) {
	case *StateWaitForReady:
		allIds := r.allParticipantIDsLocked()
		for _, id := range allIds {
			if _, ok := s.Started[id]; !ok {
				return nil
			}
		}

		if r.Contest != nil && r.Contest.ManualStart {
			return nil
		}

		if callbacks.OnEnterPlaying != nil {
			callbacks.OnEnterPlaying(r)
		}

		if callbacks.Broadcast != nil {
			if err := callbacks.Broadcast(protocol.ServerCommand{
				Type:    protocol.ServerCmdMessage,
				Message: protocol.Message{Type: protocol.MessageStartPlaying},
			}); err != nil {
				return err
			}
		}

		r.resetGameTimeLocked(callbacks.UsersById)
		r.State = &StatePlaying{
			Results: make(map[int32]*RecordData),
			Aborted: make(map[int32]struct{}),
		}

		if callbacks.Broadcast != nil {
			if err := callbacks.Broadcast(protocol.ServerCommand{
				Type:  protocol.ServerCmdChangeState,
				State: r.clientRoomStateLocked(),
			}); err != nil {
				return err
			}
		}

		if callbacks.NotifyWebSocket != nil {
			callbacks.NotifyWebSocket(r.ID)
		}

	case *StatePlaying:
		playerIds := r.userIDsLocked()
		allFinished := true
		for _, id := range playerIds {
			if _, ok := s.Results[id]; !ok {
				if _, ok2 := s.Aborted[id]; !ok2 {
					allFinished = false
					break
				}
			}
		}
		if !allFinished {
			return nil
		}

		if callbacks.Logger != nil && callbacks.Lang != nil {
			callbacks.Logger.Logf("Game ended in room %s", r.ID)
		}

		if len(s.Results) > 0 && callbacks.Lang != nil && callbacks.UsersById != nil {
			bestScore := -1
			bestAcc := float32(-1.0)
			bestStd := float32(1e9)
			var bestScoreID, bestAccID, bestStdID int32

			for id, rec := range s.Results {
				if rec.Score > bestScore {
					bestScore = rec.Score
					bestScoreID = id
				}
				if rec.Accuracy > bestAcc {
					bestAcc = rec.Accuracy
					bestAccID = id
				}
				if rec.Std < bestStd {
					bestStd = rec.Std
					bestStdID = id
				}
			}

			bestScoreName := fmt.Sprintf("%d", bestScoreID)
			if u := callbacks.UsersById(bestScoreID); u != nil {
				bestScoreName = u.Name
			}
			bestAccName := fmt.Sprintf("%d", bestAccID)
			if u := callbacks.UsersById(bestAccID); u != nil {
				bestAccName = u.Name
			}
			bestStdName := fmt.Sprintf("%d", bestStdID)
			if u := callbacks.UsersById(bestStdID); u != nil {
				bestStdName = u.Name
			}

			scoreText := callbacks.Lang.Format("chat-game-summary-score", map[string]string{
				"name":  bestScoreName,
				"id":    fmt.Sprintf("%d", bestScoreID),
				"score": fmt.Sprintf("%d", bestScore),
			})
			accText := callbacks.Lang.Format("chat-game-summary-acc", map[string]string{
				"name": bestAccName,
				"id":   fmt.Sprintf("%d", bestAccID),
				"acc":  fmt.Sprintf("%.2f%%", float64(bestAcc)*100),
			})
			stdText := callbacks.Lang.Format("chat-game-summary-std", map[string]string{
				"name": bestStdName,
				"id":   fmt.Sprintf("%d", bestStdID),
				"std":  fmt.Sprintf("%d", int(math.Round(float64(bestStd)*1000))),
			})
			summary := callbacks.Lang.Format("chat-game-summary", map[string]string{
				"scoreText": scoreText,
				"accText":   accText,
				"stdText":   stdText,
			})

			if callbacks.Broadcast != nil {
				_ = callbacks.Broadcast(protocol.ServerCommand{
					Type: protocol.ServerCmdMessage,
					Message: protocol.Message{
						Type:    protocol.MessageChat,
						User:    0,
						Content: summary,
					},
				})
			}
		}

		if callbacks.Broadcast != nil {
			if err := callbacks.Broadcast(protocol.ServerCommand{
				Type:    protocol.ServerCmdMessage,
				Message: protocol.Message{Type: protocol.MessageGameEnd},
			}); err != nil {
				return err
			}
		}

		if callbacks.OnGameEnd != nil {
			callbacks.OnGameEnd(r)
		}

		r.State = &StateSelectChart{}

		if callbacks.Broadcast != nil {
			if err := callbacks.Broadcast(protocol.ServerCommand{
				Type:  protocol.ServerCmdChangeState,
				State: r.clientRoomStateLocked(),
			}); err != nil {
				return err
			}
		}

		if callbacks.NotifyWebSocket != nil {
			callbacks.NotifyWebSocket(r.ID)
		}

		if r.Contest != nil && r.Contest.AutoDisband && callbacks.DisbandRoom != nil {
			chartText := "null"
			if r.Chart != nil {
				chartText = fmt.Sprintf("%d:%s", r.Chart.ID, r.Chart.Name)
			}

			type contestRow struct {
				ID       int32   `json:"id"`
				Name     string  `json:"name"`
				Score    int     `json:"score"`
				Acc      float32 `json:"acc"`
				FC       bool    `json:"fc"`
				Std      float32 `json:"std"`
				StdScore float32 `json:"std_score"`
			}
			var rows []contestRow
			for id, rec := range s.Results {
				name := fmt.Sprintf("%d", id)
				if u := callbacks.UsersById(id); u != nil {
					name = u.Name
				}
				rows = append(rows, contestRow{
					ID:       id,
					Name:     name,
					Score:    rec.Score,
					Acc:      rec.Accuracy,
					FC:       rec.FullCombo,
					Std:      rec.Std,
					StdScore: rec.StdScore,
				})
			}
			sort.Slice(rows, func(i, j int) bool {
				return rows[i].Score > rows[j].Score
			})
			resultsJSON, _ := json.Marshal(rows)
			var abortedIDs []int32
			for id := range s.Aborted {
				abortedIDs = append(abortedIDs, id)
			}
			sort.Slice(abortedIDs, func(i, j int) bool {
				return abortedIDs[i] < abortedIDs[j]
			})
			abortedJSON, _ := json.Marshal(abortedIDs)

			if callbacks.Logger != nil && callbacks.Lang != nil {
				callbacks.Logger.Logf("Contest game ended in room %s: chart=%s results=%s aborted=%s",
					r.ID, chartText, string(resultsJSON), string(abortedJSON))
			}
			callbacks.DisbandRoom(r)
			return nil
		}

		if r.Cycle {
			users := r.userIDsLocked()
			if len(users) > 1 {
				idx := -1
				for i, id := range users {
					if id == r.HostID {
						idx = i
						break
					}
				}
				if idx < 0 {
					idx = 0
				}
				newHost := users[(idx+1)%len(users)]
				oldHost := r.HostID
				r.HostID = newHost

				if callbacks.Logger != nil && callbacks.Lang != nil {
					callbacks.Logger.Logf("Host changed from %d to %d in room %s", oldHost, newHost, r.ID)
				}

				if callbacks.Broadcast != nil {
					if err := callbacks.Broadcast(protocol.ServerCommand{
						Type:    protocol.ServerCmdMessage,
						Message: protocol.Message{Type: protocol.MessageNewHost, User: newHost},
					}); err != nil {
						return err
					}
				}

				if oldUser := callbacks.UsersById(oldHost); oldUser != nil {
					oldUser.TrySend(protocol.ServerCommand{Type: protocol.ServerCmdChangeHost, IsHost: false})
				}
				if newUser := callbacks.UsersById(newHost); newUser != nil {
					newUser.TrySend(protocol.ServerCommand{Type: protocol.ServerCmdChangeHost, IsHost: true})
				}
			}
		}
	}

	return nil
}

func (r *Room) AddLog(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recentLogs = append(r.recentLogs, RoomLog{
		Message:   message,
		Timestamp: time.Now().UnixMilli(),
	})
	if len(r.recentLogs) > maxRecentLogs {
		r.recentLogs = r.recentLogs[1:]
	}
}

func (r *Room) GetRecentLogs() []RoomLog {
	r.mu.RLock()
	defer r.mu.RUnlock()
	logs := make([]RoomLog, len(r.recentLogs))
	copy(logs, r.recentLogs)
	return logs
}

func (r *Room) OnUserLeave(user *User, callbacks *RoomCallbacks) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if callbacks.Broadcast != nil {
		_ = callbacks.Broadcast(protocol.ServerCommand{
			Type:    protocol.ServerCmdMessage,
			Message: protocol.Message{Type: protocol.MessageLeaveRoom, User: user.ID, Name: user.Name},
		})
	}

	user.Room = nil

	if user.Monitor {
		delete(r.monitors, user.ID)
	} else {
		delete(r.users, user.ID)
	}

	if r.HostID == user.ID {
		users := r.userIDsLocked()
		if len(users) == 0 {
			if callbacks.NotifyWebSocket != nil {
				callbacks.NotifyWebSocket(r.ID)
			}
			return true
		}
		var newHost int32
		if callbacks.PickRandomUserId != nil {
			newHost = callbacks.PickRandomUserId(users)
		}
		if newHost == 0 {
			if callbacks.NotifyWebSocket != nil {
				callbacks.NotifyWebSocket(r.ID)
			}
			return true
		}
		r.HostID = newHost
		if callbacks.Broadcast != nil {
			_ = callbacks.Broadcast(protocol.ServerCommand{
				Type:    protocol.ServerCmdMessage,
				Message: protocol.Message{Type: protocol.MessageNewHost, User: newHost},
			})
		}
		if newUser := callbacks.UsersById(newHost); newUser != nil {
			newUser.TrySend(protocol.ServerCommand{Type: protocol.ServerCmdChangeHost, IsHost: true})
		}
	}

	if callbacks.NotifyWebSocket != nil {
		callbacks.NotifyWebSocket(r.ID)
	}
	return len(r.users) == 0 && len(r.monitors) == 0
}
