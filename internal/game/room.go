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

type RoomStateSnapshot struct {
	Type       string
	ReadyUsers []int32
	Results    map[int32]RecordData
	Aborted    []int32
}

type ContestSnapshot struct {
	Whitelist   []int32
	ManualStart bool
	AutoDisband bool
}

type RoomSnapshot struct {
	ID             roomid.RoomID
	MaxUsers       int
	ReplayEligible bool
	HostID         int32
	State          RoomStateSnapshot
	Live           bool
	Locked         bool
	Cycle          bool
	Contest        *ContestSnapshot
	Chart          *Chart
	Users          []int32
	Monitors       []int32
	RecentLogs     []RoomLog
}

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

func (r *Room) UserCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.users)
}

func (r *Room) MonitorCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.monitors)
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

func (r *Room) Snapshot() RoomSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snap := RoomSnapshot{
		ID:             r.ID,
		MaxUsers:       r.MaxUsers,
		ReplayEligible: r.ReplayEligible,
		HostID:         r.HostID,
		Live:           r.Live,
		Locked:         r.Locked,
		Cycle:          r.Cycle,
		Users:          r.userIDsLocked(),
		Monitors:       r.monitorIDsLocked(),
		RecentLogs:     make([]RoomLog, len(r.recentLogs)),
	}
	sort.Slice(snap.Users, func(i, j int) bool { return snap.Users[i] < snap.Users[j] })
	sort.Slice(snap.Monitors, func(i, j int) bool { return snap.Monitors[i] < snap.Monitors[j] })
	copy(snap.RecentLogs, r.recentLogs)

	if r.Chart != nil {
		chart := *r.Chart
		snap.Chart = &chart
	}
	if r.Contest != nil {
		whitelist := make([]int32, 0, len(r.Contest.Whitelist))
		for id := range r.Contest.Whitelist {
			whitelist = append(whitelist, id)
		}
		sort.Slice(whitelist, func(i, j int) bool { return whitelist[i] < whitelist[j] })
		snap.Contest = &ContestSnapshot{
			Whitelist:   whitelist,
			ManualStart: r.Contest.ManualStart,
			AutoDisband: r.Contest.AutoDisband,
		}
	}

	switch st := r.State.(type) {
	case *StateWaitForReady:
		snap.State.Type = "waiting_for_ready"
		snap.State.ReadyUsers = make([]int32, 0, len(st.Started))
		for id := range st.Started {
			snap.State.ReadyUsers = append(snap.State.ReadyUsers, id)
		}
		sort.Slice(snap.State.ReadyUsers, func(i, j int) bool { return snap.State.ReadyUsers[i] < snap.State.ReadyUsers[j] })
	case *StatePlaying:
		snap.State.Type = "playing"
		snap.State.Results = make(map[int32]RecordData, len(st.Results))
		for id, rec := range st.Results {
			if rec != nil {
				snap.State.Results[id] = *rec
			}
		}
		snap.State.Aborted = make([]int32, 0, len(st.Aborted))
		for id := range st.Aborted {
			snap.State.Aborted = append(snap.State.Aborted, id)
		}
		sort.Slice(snap.State.Aborted, func(i, j int) bool { return snap.State.Aborted[i] < snap.State.Aborted[j] })
	default:
		snap.State.Type = "select_chart"
	}

	return snap
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

func (r *Room) OnUserJoin(userID int32, monitor bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.State.(*StatePlaying); ok {
		if !monitor {
			s.Aborted[userID] = struct{}{}
		}
	}
}

func (r *Room) SetLocked(locked bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Locked = locked
}

func (r *Room) SetCycle(cycle bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Cycle = cycle
}

func (r *Room) SetMaxUsers(maxUsers int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.MaxUsers = maxUsers
}

func (r *Room) SetChart(chart *Chart) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Chart = chart
}

func (r *Room) StartWaitForReady(hostID int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.State.(*StateSelectChart); !ok {
		return errors.New("room-invalid-state")
	}
	if r.Chart == nil {
		return errors.New("start-no-chart-selected")
	}
	r.State = &StateWaitForReady{Started: map[int32]struct{}{hostID: {}}}
	return nil
}

func (r *Room) ResetToSelectChart() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.State = &StateSelectChart{}
}

func (r *Room) SetReady(userID int32) (already bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.State.(*StatePlaying); ok {
		return false, errors.New("room-invalid-state")
	}
	s, ok := r.State.(*StateWaitForReady)
	if !ok {
		// Not in WaitForReady state (e.g. SelectChart) - treat as no-op
		return true, nil
	}
	if _, exists := s.Started[userID]; exists {
		return false, errors.New("room-already-ready")
	}
	s.Started[userID] = struct{}{}
	return false, nil
}

func (r *Room) CancelReady(userID int32) (wasHost bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.State.(*StatePlaying); ok {
		return false, errors.New("room-invalid-state")
	}
	s, ok := r.State.(*StateWaitForReady)
	if !ok {
		return false, nil
	}
	if _, exists := s.Started[userID]; !exists {
		return false, errors.New("room-not-ready")
	}
	delete(s.Started, userID)
	return r.HostID == userID, nil
}

func (r *Room) AddResult(userID int32, record *RecordData) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.State.(*StatePlaying)
	if !ok {
		return errors.New("room-invalid-state")
	}
	if _, aborted := s.Aborted[userID]; aborted {
		return errors.New("room-game-aborted")
	}
	if _, results := s.Results[userID]; results {
		return errors.New("record-already-uploaded")
	}
	s.Results[userID] = record
	return nil
}

func (r *Room) SetContest(contest *ContestConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Contest = contest
}

func (r *Room) ClearContest() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Contest = nil
}

func (r *Room) SetContestWhitelist(whitelist map[int32]struct{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Contest == nil {
		return errors.New("contest-not-enabled")
	}
	r.Contest.Whitelist = whitelist
	return nil
}

func (r *Room) ForceStartPlaying() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.State.(*StateWaitForReady); !ok {
		return errors.New("room-not-waiting")
	}
	r.State = &StatePlaying{
		Results: make(map[int32]*RecordData),
		Aborted: make(map[int32]struct{}),
	}
	return nil
}

func (r *Room) RefreshLive(replayEnabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Live = len(r.monitors) > 0 || (replayEnabled && r.ReplayEligible)
}

func (r *Room) CanAcceptTouches(userID int32) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.State.(*StatePlaying)
	if !ok {
		return false
	}
	if _, aborted := s.Aborted[userID]; aborted {
		return false
	}
	if _, results := s.Results[userID]; results {
		return false
	}
	return true
}

func (r *Room) SetAborted(userID int32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.State.(*StatePlaying)
	if !ok {
		return errors.New("room-invalid-state")
	}
	if _, results := s.Results[userID]; results {
		return errors.New("record-already-uploaded")
	}
	if _, aborted := s.Aborted[userID]; aborted {
		return errors.New("room-game-aborted")
	}
	s.Aborted[userID] = struct{}{}
	return nil
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
			u.ResetGameTime()
		}
	}
}

// ResetGameTime resets the game time for all players in the room.
func (r *Room) ResetGameTime(usersById func(int32) *User) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.resetGameTimeLocked(usersById)
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

		if callbacks.Logger != nil {
			callbacks.Logger.DebugL(callbacks.Lang, "log-room-all-ready", map[string]string{"room": string(r.ID), "users": fmt.Sprintf("%d", len(r.users))})
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

		if callbacks.UsersById != nil {
			r.resetGameTimeLocked(callbacks.UsersById)
		}
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
			callbacks.Logger.LogfL(callbacks.Lang, "log-game-ended", map[string]string{"room": string(r.ID)})
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
				bestScoreName = u.GetName()
			}
			bestAccName := fmt.Sprintf("%d", bestAccID)
			if u := callbacks.UsersById(bestAccID); u != nil {
				bestAccName = u.GetName()
			}
			bestStdName := fmt.Sprintf("%d", bestStdID)
			if u := callbacks.UsersById(bestStdID); u != nil {
				bestStdName = u.GetName()
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

		if callbacks.Logger != nil {
			callbacks.Logger.DebugL(callbacks.Lang, "log-room-game-ended", map[string]string{"room": string(r.ID), "results": fmt.Sprintf("%d", len(s.Results)), "aborted": fmt.Sprintf("%d", len(s.Aborted))})
		}

		r.State = &StateSelectChart{}

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
					name = u.GetName()
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
				callbacks.Logger.LogfL(callbacks.Lang, "log-contest-game-ended", map[string]string{"room": string(r.ID), "chart": chartText, "results": string(resultsJSON), "aborted": string(abortedJSON)})
			}
			callbacks.DisbandRoom(r)
			return nil
		}

		if r.Cycle {
			users := r.userIDsLocked()
			sort.Slice(users, func(i, j int) bool { return users[i] < users[j] })
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

				if callbacks.Logger != nil {
					callbacks.Logger.DebugL(callbacks.Lang, "log-room-host-cycled", map[string]string{"room": string(r.ID), "oldHost": fmt.Sprintf("%d", oldHost), "newHost": fmt.Sprintf("%d", newHost)})
				}

				if callbacks.Logger != nil && callbacks.Lang != nil {
					callbacks.Logger.LogfL(callbacks.Lang, "log-host-changed", map[string]string{"oldHost": fmt.Sprintf("%d", oldHost), "newHost": fmt.Sprintf("%d", newHost), "room": string(r.ID)})
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
			Message: protocol.Message{Type: protocol.MessageLeaveRoom, User: user.ID, Name: user.GetName()},
		})
	}

	wasMonitor := user.IsMonitor()
	user.ClearRoom()

	if wasMonitor {
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
