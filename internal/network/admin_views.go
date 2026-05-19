package network

import (
	"sort"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

type adminRoomState struct {
	Type          string   `json:"type"`
	ReadyUsers    *[]int32 `json:"ready_users,omitempty"`
	ReadyCount    *int     `json:"ready_count,omitempty"`
	ResultsCount  *int     `json:"results_count,omitempty"`
	AbortedCount  *int     `json:"aborted_count,omitempty"`
	FinishedUsers *[]int32 `json:"finished_users,omitempty"`
	AbortedUsers  *[]int32 `json:"aborted_users,omitempty"`
}

type adminRoomUser struct {
	ID        int32   `json:"id"`
	Name      string  `json:"name"`
	Connected bool    `json:"connected"`
	IsHost    bool    `json:"is_host"`
	GameTime  float32 `json:"game_time"`
	Language  string  `json:"language"`
	Finished  *bool   `json:"finished,omitempty"`
	Aborted   *bool   `json:"aborted,omitempty"`
	RecordID  *int32  `json:"record_id,omitempty"`
}

type adminRoomMonitor struct {
	ID        int32  `json:"id"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
	Language  string `json:"language"`
}

type adminRoomData struct {
	RoomID          string             `json:"roomid"`
	MaxUsers        int                `json:"max_users"`
	CurrentUsers    int                `json:"current_users"`
	CurrentMonitors int                `json:"current_monitors"`
	ReplayEligible  bool               `json:"replay_eligible"`
	Live            bool               `json:"live"`
	Locked          bool               `json:"locked"`
	Cycle           bool               `json:"cycle"`
	Host            adminRoomHost      `json:"host"`
	State           adminRoomState     `json:"state"`
	Chart           *adminChart        `json:"chart"`
	Contest         *adminContest      `json:"contest"`
	Users           []adminRoomUser    `json:"users"`
	Monitors        []adminRoomMonitor `json:"monitors"`
	RecentLogs      []game.RoomLog     `json:"recent_logs"`
}

type adminRoomHost struct {
	ID        int32  `json:"id"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

type adminChart struct {
	Name string `json:"name"`
	ID   int    `json:"id"`
}

type adminContest struct {
	WhitelistCount int     `json:"whitelist_count"`
	Whitelist      []int32 `json:"whitelist"`
	ManualStart    bool    `json:"manual_start"`
	AutoDisband    bool    `json:"auto_disband"`
}

type roomUpdateData struct {
	RoomID     string              `json:"roomid"`
	State      string              `json:"state"`
	Locked     bool                `json:"locked"`
	Cycle      bool                `json:"cycle"`
	Live       bool                `json:"live"`
	Chart      *adminChart         `json:"chart"`
	Host       roomUpdateHost      `json:"host"`
	Users      []roomUpdateUser    `json:"users"`
	Monitors   []roomUpdateMonitor `json:"monitors"`
	RecentLogs []game.RoomLog      `json:"recent_logs"`
}

type roomUpdateHost struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

type roomUpdateUser struct {
	ID      int32  `json:"id"`
	Name    string `json:"name"`
	IsReady bool   `json:"is_ready"`
}

type roomUpdateMonitor struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

func buildAdminRoomsData(st *state.ServerState) []adminRoomData {
	type roomEntry struct {
		rid  roomid.RoomID
		room *game.Room
	}
	var entries []roomEntry
	users := map[int32]*game.User{}
	st.WithRLock(func() {
		for rid, room := range st.Rooms {
			entries = append(entries, roomEntry{rid: rid, room: room})
		}
		for id, user := range st.Users {
			users[id] = user
		}
	})
	rooms := make([]adminRoomData, 0, len(entries))
	for _, entry := range entries {
		rooms = append(rooms, buildAdminRoomData(entry.rid, entry.room.Snapshot(), users))
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].RoomID < rooms[j].RoomID })
	return rooms
}

func buildRoomUpdateData(st *state.ServerState, rid roomid.RoomID) *roomUpdateData {
	var room *game.Room
	usersByID := map[int32]*game.User{}
	st.WithRLock(func() {
		room = st.Rooms[rid]
		for id, user := range st.Users {
			usersByID[id] = user
		}
	})
	if room == nil {
		return nil
	}

	snap := room.Snapshot()
	hostName, _ := userNameAndConnected(usersByID[snap.HostID], snap.HostID)
	ready := make(map[int32]struct{}, len(snap.State.ReadyUsers))
	for _, id := range snap.State.ReadyUsers {
		ready[id] = struct{}{}
	}

	users := make([]roomUpdateUser, 0, len(snap.Users))
	for _, id := range snap.Users {
		name, _ := userNameAndConnected(usersByID[id], id)
		_, isReady := ready[id]
		users = append(users, roomUpdateUser{ID: id, Name: name, IsReady: isReady})
	}
	monitors := make([]roomUpdateMonitor, 0, len(snap.Monitors))
	for _, id := range snap.Monitors {
		name, _ := userNameAndConnected(usersByID[id], id)
		monitors = append(monitors, roomUpdateMonitor{ID: id, Name: name})
	}

	return &roomUpdateData{
		RoomID:     string(rid),
		State:      snap.State.Type,
		Locked:     snap.Locked,
		Cycle:      snap.Cycle,
		Live:       snap.Live,
		Chart:      chartView(snap.Chart),
		Host:       roomUpdateHost{ID: snap.HostID, Name: hostName},
		Users:      users,
		Monitors:   monitors,
		RecentLogs: snap.RecentLogs,
	}
}

func buildAdminRoomData(rid roomid.RoomID, snap game.RoomSnapshot, usersByID map[int32]*game.User) adminRoomData {
	hostName, hostConnected := userNameAndConnected(usersByID[snap.HostID], snap.HostID)
	stateView := adminRoomState{Type: snap.State.Type}
	switch snap.State.Type {
	case "waiting_for_ready":
		readyUsers := append([]int32(nil), snap.State.ReadyUsers...)
		readyCount := len(readyUsers)
		stateView.ReadyUsers = &readyUsers
		stateView.ReadyCount = &readyCount
	case "playing":
		finishedUsers := make([]int32, 0, len(snap.State.Results))
		for id := range snap.State.Results {
			finishedUsers = append(finishedUsers, id)
		}
		sort.Slice(finishedUsers, func(i, j int) bool { return finishedUsers[i] < finishedUsers[j] })
		abortedUsers := append([]int32(nil), snap.State.Aborted...)
		resultsCount := len(finishedUsers)
		abortedCount := len(abortedUsers)
		stateView.ResultsCount = &resultsCount
		stateView.AbortedCount = &abortedCount
		stateView.FinishedUsers = &finishedUsers
		stateView.AbortedUsers = &abortedUsers
	}

	users := make([]adminRoomUser, 0, len(snap.Users))
	aborted := make(map[int32]struct{}, len(snap.State.Aborted))
	for _, id := range snap.State.Aborted {
		aborted[id] = struct{}{}
	}
	for _, id := range snap.Users {
		u := usersByID[id]
		name, connected := userNameAndConnected(u, id)
		info := adminRoomUser{
			ID:        id,
			Name:      name,
			Connected: connected,
			IsHost:    id == snap.HostID,
			GameTime:  userGameTime(u),
			Language:  userLanguage(u),
		}
		if snap.State.Type == "playing" {
			_, didAbort := aborted[id]
			rec, didFinish := snap.State.Results[id]
			finished := didFinish || didAbort
			info.Finished = &finished
			info.Aborted = &didAbort
			if didFinish {
				recordID := rec.ID
				info.RecordID = &recordID
			}
		}
		users = append(users, info)
	}

	monitors := make([]adminRoomMonitor, 0, len(snap.Monitors))
	for _, id := range snap.Monitors {
		u := usersByID[id]
		name, connected := userNameAndConnected(u, id)
		monitors = append(monitors, adminRoomMonitor{
			ID:        id,
			Name:      name,
			Connected: connected,
			Language:  userLanguage(u),
		})
	}

	var contest *adminContest
	if snap.Contest != nil {
		whitelist := append([]int32(nil), snap.Contest.Whitelist...)
		contest = &adminContest{
			WhitelistCount: len(whitelist),
			Whitelist:      whitelist,
			ManualStart:    snap.Contest.ManualStart,
			AutoDisband:    snap.Contest.AutoDisband,
		}
	}

	return adminRoomData{
		RoomID:          string(rid),
		MaxUsers:        snap.MaxUsers,
		CurrentUsers:    len(snap.Users),
		CurrentMonitors: len(snap.Monitors),
		ReplayEligible:  snap.ReplayEligible,
		Live:            snap.Live,
		Locked:          snap.Locked,
		Cycle:           snap.Cycle,
		Host:            adminRoomHost{ID: snap.HostID, Name: hostName, Connected: hostConnected},
		State:           stateView,
		Chart:           chartView(snap.Chart),
		Contest:         contest,
		Users:           users,
		Monitors:        monitors,
		RecentLogs:      snap.RecentLogs,
	}
}

func chartView(chart *game.Chart) *adminChart {
	if chart == nil {
		return nil
	}
	return &adminChart{Name: chart.Name, ID: chart.ID}
}

func userNameAndConnected(u *game.User, fallback int32) (string, bool) {
	if u == nil {
		return int32String(fallback), false
	}
	return u.GetName(), u.HasSession()
}

func userGameTime(u *game.User) float32 {
	if u == nil {
		return -1e9
	}
	return u.GetGameTime()
}

func userLanguage(u *game.User) string {
	if u == nil || u.GetLang() == nil {
		return "unknown"
	}
	return u.GetLang().Lang()
}

func int32String(v int32) string {
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	if v < 0 {
		buf = append(buf, '-')
		v = -v
	}
	var tmp [10]byte
	i := len(tmp)
	for v > 0 {
		i--
		tmp[i] = byte('0' + v%10)
		v /= 10
	}
	return string(append(buf, tmp[i:]...))
}
