package server

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"

	"phira-mp/common"
)

const (
	RoomMaxUsers = 8
)

// InternalRoomState 房间内部状态
type InternalRoomState int

const (
	InternalStateSelectChart InternalRoomState = iota
	InternalStateWaitForReady
	InternalStatePlaying
)

// ToClientState 转换为客户端状态
func (s InternalRoomState) ToClientState(chartID *int32) common.RoomState {
	switch s {
	case InternalStateSelectChart:
		return common.RoomState{
			Type:    common.RoomStateSelectChart,
			ChartID: chartID,
		}
	case InternalStateWaitForReady:
		return common.RoomState{Type: common.RoomStateWaitingForReady}
	case InternalStatePlaying:
		return common.RoomState{Type: common.RoomStatePlaying}
	}
	return common.RoomState{Type: common.RoomStateSelectChart}
}

// Room 房间
type Room struct {
	ID    common.RoomId
	host  atomic.Value // *User
	state atomic.Int32 // InternalRoomState

	live   atomic.Bool
	locked atomic.Bool
	cycle  atomic.Bool

	users       sync.RWMutex
	userList    []*User
	monitors    sync.RWMutex
	monitorList []*User

	chart atomic.Value // *Chart

	// 游戏状态
	started sync.Map // map[int32]bool - 已准备的玩家
	results sync.Map // map[int32]*Record - 游戏结果
	aborted sync.Map // map[int32]bool - 放弃的玩家

	server *Server
}

// NewRoom 创建新房间
func NewRoom(id common.RoomId, host *User, server *Server) *Room {
	r := &Room{
		ID:     id,
		server: server,
	}
	r.host.Store(host)
	r.state.Store(int32(InternalStateSelectChart))
	r.userList = []*User{host}
	return r
}

// GetHost 获取房主
func (r *Room) GetHost() *User {
	return r.host.Load().(*User)
}

// SetHost 设置房主
func (r *Room) SetHost(user *User) {
	r.host.Store(user)
}

// GetState 获取房间状态
func (r *Room) GetState() InternalRoomState {
	return InternalRoomState(r.state.Load())
}

// SetState 设置房间状态
func (r *Room) SetState(state InternalRoomState) {
	r.state.Store(int32(state))
}

// IsLive 是否直播中
func (r *Room) IsLive() bool {
	return r.live.Load()
}

// SetLive 设置直播状态
func (r *Room) SetLive(live bool) {
	r.live.Store(live)
}

// IsLocked 是否锁定
func (r *Room) IsLocked() bool {
	return r.locked.Load()
}

// SetLocked 设置锁定状态
func (r *Room) SetLocked(locked bool) {
	r.locked.Store(locked)
}

// IsCycle 是否循环
func (r *Room) IsCycle() bool {
	return r.cycle.Load()
}

// SetCycle 设置循环状态
func (r *Room) SetCycle(cycle bool) {
	r.cycle.Store(cycle)
}

// GetChart 获取当前谱面
func (r *Room) GetChart() *Chart {
	chart := r.chart.Load()
	if chart == nil {
		return nil
	}
	return chart.(*Chart)
}

// SetChart 设置谱面
func (r *Room) SetChart(chart *Chart) {
	r.chart.Store(chart)
}

// GetClientRoomState 获取客户端房间状态
func (r *Room) GetClientRoomState(user *User) common.ClientRoomState {
	chart := r.GetChart()
	var chartID *int32
	if chart != nil {
		chartID = &chart.ID
	}

	users := r.GetAllUsers()
	userMap := make(map[int32]common.UserInfo)
	for _, u := range users {
		userMap[u.ID] = u.ToInfo()
	}

	// 检查是否已准备
	isReady := false
	if r.GetState() == InternalStateWaitForReady {
		_, isReady = r.started.Load(user.ID)
	}

	return common.ClientRoomState{
		ID:      r.ID,
		State:   r.GetState().ToClientState(chartID),
		Live:    r.IsLive(),
		Locked:  r.IsLocked(),
		Cycle:   r.IsCycle(),
		IsHost:  r.GetHost().ID == user.ID,
		IsReady: isReady,
		Users:   userMap,
	}
}

// AddUser 添加用户
func (r *Room) AddUser(user *User, monitor bool) bool {
	if monitor {
		r.monitors.Lock()
		defer r.monitors.Unlock()
		r.monitorList = append(r.monitorList, user)
		return true
	}

	r.users.Lock()
	defer r.users.Unlock()
	if len(r.userList) >= RoomMaxUsers {
		return false
	}
	r.userList = append(r.userList, user)
	return true
}

// GetUsers 获取所有普通用户
func (r *Room) GetUsers() []*User {
	r.users.RLock()
	defer r.users.RUnlock()
	result := make([]*User, 0, len(r.userList))
	for _, u := range r.userList {
		if u != nil && !u.IsDisconnected() {
			result = append(result, u)
		}
	}
	return result
}

// GetMonitors 获取所有观察者
func (r *Room) GetMonitors() []*User {
	r.monitors.RLock()
	defer r.monitors.RUnlock()
	result := make([]*User, 0, len(r.monitorList))
	for _, u := range r.monitorList {
		if u != nil && !u.IsDisconnected() {
			result = append(result, u)
		}
	}
	return result
}

// GetAllUsers 获取所有用户（包括观察者）
func (r *Room) GetAllUsers() []*User {
	return append(r.GetUsers(), r.GetMonitors()...)
}

// RemoveUser 移除用户
func (r *Room) RemoveUser(userID int32) {
	r.users.Lock()
	defer r.users.Unlock()
	for i, u := range r.userList {
		if u.ID == userID {
			r.userList = append(r.userList[:i], r.userList[i+1:]...)
			break
		}
	}

	r.monitors.Lock()
	defer r.monitors.Unlock()
	for i, u := range r.monitorList {
		if u.ID == userID {
			r.monitorList = append(r.monitorList[:i], r.monitorList[i+1:]...)
			break
		}
	}
}

// CheckHost 检查是否是房主
func (r *Room) CheckHost(user *User) error {
	if r.GetHost().ID != user.ID {
		return fmt.Errorf("only host can do this")
	}
	return nil
}

// Broadcast 广播消息给所有用户
func (r *Room) Broadcast(cmd common.ServerCommand) {
	for _, user := range r.GetAllUsers() {
		user.Send(cmd)
	}
}

// BroadcastMonitors 广播给观察者
func (r *Room) BroadcastMonitors(cmd common.ServerCommand) {
	for _, user := range r.GetMonitors() {
		user.Send(cmd)
	}
}

// SendMessage 发送房间消息
func (r *Room) SendMessage(msg common.Message) {
	r.Broadcast(common.ServerCommand{
		Type:    common.ServerCmdMessage,
		Message: &msg,
	})
}

// OnUserLeave 用户离开房间
// 返回值：是否删除房间
func (r *Room) OnUserLeave(user *User) bool {
	r.SendMessage(common.Message{
		Type: common.MsgLeaveRoom,
		User: user.ID,
		Name: user.Name,
	})

	r.RemoveUser(user.ID)
	user.SetRoom(nil)

	// 如果是房主离开
	if r.GetHost().ID == user.ID {
		users := r.GetUsers()
		if len(users) == 0 {
			return true // 房间空了，删除房间
		}
		// 随机选择新房主
		newHost := users[rand.Intn(len(users))]
		r.SetHost(newHost)
		r.SendMessage(common.Message{
			Type: common.MsgNewHost,
			User: newHost.ID,
		})
		newHost.Send(common.ServerCommand{
			Type:       common.ServerCmdChangeHost,
			ChangeHost: true,
		})
	}

	r.CheckAllReady()
	return false
}

// ResetGameTime 重置游戏时间
func (r *Room) ResetGameTime() {
	for _, user := range r.GetUsers() {
		user.gameTime.Store(math.Float32bits(float32(math.Inf(-1))))
	}
}

// CheckAllReady 检查是否全部准备就绪
func (r *Room) CheckAllReady() {
	state := r.GetState()
	switch state {
	case InternalStateWaitForReady:
		// 只检查普通玩家，不包括观察者
		users := r.GetUsers()
		allReady := true
		for _, u := range users {
			if _, ok := r.started.Load(u.ID); !ok {
				allReady = false
				break
			}
		}
		if allReady {
			// 清空之前的游戏状态
			r.results = sync.Map{}
			r.aborted = sync.Map{}
			
			r.SendMessage(common.Message{Type: common.MsgStartPlaying})
			r.ResetGameTime()
			r.SetState(InternalStatePlaying)
			r.Broadcast(common.ServerCommand{
				Type:        common.ServerCmdChangeState,
				ChangeState: &common.RoomState{Type: common.RoomStatePlaying},
			})

			// 开始回放录制
			if recorder := r.server.GetReplayRecorder(); recorder != nil {
				recorder.StartRecording(r)
			}
		}

	case InternalStatePlaying:
		users := r.GetUsers()
		allDone := true
		for _, u := range users {
			_, hasResult := r.results.Load(u.ID)
			_, hasAborted := r.aborted.Load(u.ID)
			if !hasResult && !hasAborted {
				allDone = false
				break
			}
		}
		if allDone {
			// 输出游玩结束信息
			r.logGameEnd()

			// 停止回放录制
			if recorder := r.server.GetReplayRecorder(); recorder != nil {
				recorder.StopRecording(r.ID.Value)
			}

			r.SendMessage(common.Message{Type: common.MsgGameEnd})
			
			// 清空游戏状态
			r.started = sync.Map{}
			r.results = sync.Map{}
			r.aborted = sync.Map{}
			
			r.SetState(InternalStateSelectChart)

			// 循环模式：切换房主
			if r.IsCycle() {
				r.CycleHost()
			}

			chart := r.GetChart()
			var chartID *int32
			if chart != nil {
				chartID = &chart.ID
			}
			r.Broadcast(common.ServerCommand{
				Type:        common.ServerCmdChangeState,
				ChangeState: &common.RoomState{Type: common.RoomStateSelectChart, ChartID: chartID},
			})
		}
	}
}

// CycleHost 循环切换房主
func (r *Room) CycleHost() {
	users := r.GetUsers()
	if len(users) == 0 {
		return
	}

	oldHost := r.GetHost()
	var newHostIndex int
	for i, u := range users {
		if u.ID == oldHost.ID {
			newHostIndex = (i + 1) % len(users)
			break
		}
	}
	newHost := users[newHostIndex]
	r.SetHost(newHost)

	r.SendMessage(common.Message{
		Type: common.MsgNewHost,
		User: newHost.ID,
	})

	oldHost.Send(common.ServerCommand{
		Type:       common.ServerCmdChangeHost,
		ChangeHost: false,
	})
	newHost.Send(common.ServerCommand{
		Type:       common.ServerCmdChangeHost,
		ChangeHost: true,
	})
}

// OnStateChange 状态变化时广播
func (r *Room) OnStateChange() {
	chart := r.GetChart()
	var chartID *int32
	if chart != nil {
		chartID = &chart.ID
	}
	r.Broadcast(common.ServerCommand{
		Type:        common.ServerCmdChangeState,
		ChangeState: &common.RoomState{Type: r.GetState().ToClientState(chartID).Type, ChartID: chartID},
	})
}

// logGameEnd 输出游戏结束信息
func (r *Room) logGameEnd() {
	host := r.GetHost()
	chart := r.GetChart()
	chartName := "未知谱面"
	if chart != nil {
		chartName = chart.Name
	}

	// 收集所有玩家结果
	var results []string
	r.results.Range(func(key, value interface{}) bool {
		userID := key.(int32)
		record := value.(*Record)
		// 查找用户名
		userName := fmt.Sprintf("玩家%d", userID)
		for _, u := range r.GetAllUsers() {
			if u.ID == userID {
				userName = u.Name
				break
			}
		}
		results = append(results, fmt.Sprintf("%s(%d): 分数=%d, 准度=%.2f%%", userName, userID, record.Score, record.Accuracy))
		return true
	})

	// 收集放弃的玩家
	var aborted []string
	r.aborted.Range(func(key, value interface{}) bool {
		userID := key.(int32)
		// 查找用户名
		userName := fmt.Sprintf("玩家%d", userID)
		for _, u := range r.GetAllUsers() {
			if u.ID == userID {
				userName = u.Name
				break
			}
		}
		aborted = append(aborted, fmt.Sprintf("%s(%d)", userName, userID))
		return true
	})

	logMsg := fmt.Sprintf("房间 `%s` 游玩结束 - 房主: %s(%d), 谱面: %s", r.ID.Value, host.Name, host.ID, chartName)
	if len(results) > 0 {
		logMsg += fmt.Sprintf(" | 成绩: %v", results)
	}
	if len(aborted) > 0 {
		logMsg += fmt.Sprintf(" | 放弃: %v", aborted)
	}
	log.Printf(logMsg)
}
