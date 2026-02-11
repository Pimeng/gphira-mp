package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"phira-mp/common"
)

const (
	Host = "https://phira.5wyxi.com"
)

// User 用户
type User struct {
	ID   int32
	Name string
	Lang string

	server  *Server
	session atomic.Value // *Session
	room    atomic.Value // *Room

	monitor  atomic.Bool
	gameTime atomic.Uint32

	mu           sync.RWMutex
	disconnected bool
	dangleMark   *time.Timer
}

// NewUser 创建新用户
func NewUser(id int32, name, lang string, server *Server) *User {
	return &User{
		ID:     id,
		Name:   name,
		Lang:   lang,
		server: server,
	}
}

// ToInfo 转换为用户信息
func (u *User) ToInfo() common.UserInfo {
	return common.UserInfo{
		ID:      u.ID,
		Name:    u.Name,
		Monitor: u.monitor.Load(),
	}
}

// CanMonitor 是否能观察
func (u *User) CanMonitor() bool {
	// 直播模式未启用时，不允许观察
	if !u.server.config.LiveMode {
		return false
	}
	for _, id := range u.server.config.Monitors {
		if id == u.ID {
			return true
		}
	}
	return false
}

// SetSession 设置会话
func (u *User) SetSession(session *Session) {
	u.session.Store(session)
	u.mu.Lock()
	if u.dangleMark != nil {
		u.dangleMark.Stop()
		u.dangleMark = nil
	}
	u.mu.Unlock()
}

// GetSession 获取会话
func (u *User) GetSession() *Session {
	s := u.session.Load()
	if s == nil {
		return nil
	}
	return s.(*Session)
}

// SetRoom 设置房间
func (u *User) SetRoom(room *Room) {
	u.room.Store(room)
}

// GetRoom 获取房间
func (u *User) GetRoom() *Room {
	r := u.room.Load()
	if r == nil {
		return nil
	}
	return r.(*Room)
}

// IsMonitor 是否是观察者
func (u *User) IsMonitor() bool {
	return u.monitor.Load()
}

// SetMonitor 设置观察者状态
func (u *User) SetMonitor(monitor bool) {
	u.monitor.Store(monitor)
}

// IsDisconnected 是否已断开连接
func (u *User) IsDisconnected() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.disconnected
}

// SetDisconnected 设置断开连接状态
func (u *User) SetDisconnected(disconnected bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.disconnected = disconnected
}

// Send 发送命令给用户
func (u *User) Send(cmd common.ServerCommand) {
	session := u.GetSession()
	if session != nil {
		session.Send(cmd)
	}
}

// Dangle 处理用户连接断开
func (u *User) Dangle() {
	u.mu.Lock()
	if u.dangleMark != nil {
		u.dangleMark.Stop()
	}
	u.dangleMark = time.AfterFunc(10*time.Second, func() {
		u.HandleDangleTimeout()
	})
	u.mu.Unlock()
}

// HandleDangleTimeout 处理悬挂超时
func (u *User) HandleDangleTimeout() {
	room := u.GetRoom()
	if room != nil {
		if room.GetState() == InternalStatePlaying {
			// 游戏中断开，直接离开
			u.server.RemoveUser(u.ID)
			if room.OnUserLeave(u) {
				u.server.RemoveRoom(room.ID, "房间为空")
			}
			return
		}
	}

	// 检查是否还在悬挂状态
	u.mu.Lock()
	if u.dangleMark != nil {
		u.mu.Unlock()
		return
	}
	u.mu.Unlock()

	if room != nil {
		u.server.RemoveUser(u.ID)
		if room.OnUserLeave(u) {
			u.server.RemoveRoom(room.ID, "房间为空")
		}
	}
}

// UserInfoFromAPI 从API获取用户信息
func UserInfoFromAPI(token string) (*User, *common.ClientRoomState, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", Host+"/me", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("authentication failed")
	}

	var userInfo struct {
		ID       int32  `json:"id"`
		Name     string `json:"name"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, nil, err
	}

	return &User{
		ID:   userInfo.ID,
		Name: userInfo.Name,
		Lang: userInfo.Language,
	}, nil, nil
}

// FetchChart 从API获取谱面信息
func FetchChart(chartID int32) (*Chart, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/chart/%d", Host, chartID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chart not found")
	}

	var chart Chart
	if err := json.NewDecoder(resp.Body).Decode(&chart); err != nil {
		return nil, err
	}
	return &chart, nil
}

// FetchRecord 从API获取记录信息
func FetchRecord(recordID int32) (*Record, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/record/%d", Host, recordID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("record not found")
	}

	var record Record
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return nil, err
	}
	return &record, nil
}
