package server

import (
	"encoding/json"
	"os"
	"sync"
)

// AdminData 管理员数据
type AdminData struct {
	mu sync.RWMutex

	// 服务器级封禁（禁止进入服务器）
	BannedUsers map[int32]bool `json:"banned_users"`

	// 房间级封禁（禁止进入特定房间）
	RoomBans map[string]map[int32]bool `json:"room_bans"` // roomId -> userId -> banned
}

// NewAdminData 创建新的管理员数据
func NewAdminData() *AdminData {
	return &AdminData{
		BannedUsers: make(map[int32]bool),
		RoomBans:    make(map[string]map[int32]bool),
	}
}

// Load 从文件加载数据
func (a *AdminData) Load(path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，使用空数据
			return nil
		}
		return err
	}

	return json.Unmarshal(data, a)
}

// Save 保存数据到文件
func (a *AdminData) Save(path string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// IsUserBanned 检查用户是否被服务器封禁
func (a *AdminData) IsUserBanned(userID int32) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.BannedUsers[userID]
}

// BanUser 封禁/解封用户
func (a *AdminData) BanUser(userID int32, banned bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if banned {
		a.BannedUsers[userID] = true
	} else {
		delete(a.BannedUsers, userID)
	}
}

// IsUserBannedFromRoom 检查用户是否被禁止进入特定房间
func (a *AdminData) IsUserBannedFromRoom(userID int32, roomID string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if roomBans, ok := a.RoomBans[roomID]; ok {
		return roomBans[userID]
	}
	return false
}

// BanUserFromRoom 封禁/解封用户进入房间
func (a *AdminData) BanUserFromRoom(userID int32, roomID string, banned bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.RoomBans[roomID] == nil {
		a.RoomBans[roomID] = make(map[int32]bool)
	}

	if banned {
		a.RoomBans[roomID][userID] = true
	} else {
		delete(a.RoomBans[roomID], userID)
		// 如果房间没有封禁用户了，删除该房间的map
		if len(a.RoomBans[roomID]) == 0 {
			delete(a.RoomBans, roomID)
		}
	}
}

// GetBannedUsers 获取所有被封禁的用户
func (a *AdminData) GetBannedUsers() []int32 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]int32, 0, len(a.BannedUsers))
	for userID := range a.BannedUsers {
		result = append(result, userID)
	}
	return result
}

// GetRoomBans 获取房间的所有封禁用户
func (a *AdminData) GetRoomBans(roomID string) []int32 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if roomBans, ok := a.RoomBans[roomID]; ok {
		result := make([]int32, 0, len(roomBans))
		for userID := range roomBans {
			result = append(result, userID)
		}
		return result
	}
	return nil
}
