package test

import (
	"os"
	"path/filepath"
	"testing"

	"phira-mp/server"
)

// TestAdminDataCreation 测试管理员数据创建
func TestAdminDataCreation(t *testing.T) {
	adminData := server.NewAdminData()

	if adminData == nil {
		t.Fatal("管理员数据创建失败")
	}

	if adminData.BannedUsers == nil {
		t.Error("BannedUsers map不应该为空")
	}

	if adminData.RoomBans == nil {
		t.Error("RoomBans map不应该为空")
	}
}

// TestBanUser 测试封禁用户
func TestBanUser(t *testing.T) {
	adminData := server.NewAdminData()

	// 封禁用户
	adminData.BanUser(1, true)
	if !adminData.IsUserBanned(1) {
		t.Error("用户1应该被封禁")
	}

	// 解封用户
	adminData.BanUser(1, false)
	if adminData.IsUserBanned(1) {
		t.Error("用户1应该被解封")
	}

	// 封禁多个用户
	adminData.BanUser(2, true)
	adminData.BanUser(3, true)

	if !adminData.IsUserBanned(2) {
		t.Error("用户2应该被封禁")
	}
	if !adminData.IsUserBanned(3) {
		t.Error("用户3应该被封禁")
	}
}

// TestBanUserFromRoom 测试房间级封禁
func TestBanUserFromRoom(t *testing.T) {
	adminData := server.NewAdminData()

	// 封禁用户进入特定房间
	adminData.BanUserFromRoom(1, "room1", true)
	if !adminData.IsUserBannedFromRoom(1, "room1") {
		t.Error("用户1应该被禁止进入room1")
	}

	// 用户应该能进入其他房间
	if adminData.IsUserBannedFromRoom(1, "room2") {
		t.Error("用户1应该能进入room2")
	}

	// 其他用户应该能进入room1
	if adminData.IsUserBannedFromRoom(2, "room1") {
		t.Error("用户2应该能进入room1")
	}

	// 解封
	adminData.BanUserFromRoom(1, "room1", false)
	if adminData.IsUserBannedFromRoom(1, "room1") {
		t.Error("用户1应该被解封")
	}
}

// TestMultipleRoomBans 测试多房间封禁
func TestMultipleRoomBans(t *testing.T) {
	adminData := server.NewAdminData()

	// 封禁用户进入多个房间
	adminData.BanUserFromRoom(1, "room1", true)
	adminData.BanUserFromRoom(1, "room2", true)
	adminData.BanUserFromRoom(1, "room3", true)

	// 验证所有封禁
	if !adminData.IsUserBannedFromRoom(1, "room1") {
		t.Error("用户1应该被禁止进入room1")
	}
	if !adminData.IsUserBannedFromRoom(1, "room2") {
		t.Error("用户1应该被禁止进入room2")
	}
	if !adminData.IsUserBannedFromRoom(1, "room3") {
		t.Error("用户1应该被禁止进入room3")
	}

	// 封禁多个用户进入同一房间
	adminData.BanUserFromRoom(2, "room1", true)
	adminData.BanUserFromRoom(3, "room1", true)

	if !adminData.IsUserBannedFromRoom(2, "room1") {
		t.Error("用户2应该被禁止进入room1")
	}
	if !adminData.IsUserBannedFromRoom(3, "room1") {
		t.Error("用户3应该被禁止进入room1")
	}
}

// TestGetBannedUsers 测试获取被封禁用户列表
func TestGetBannedUsers(t *testing.T) {
	adminData := server.NewAdminData()

	// 初始应该为空
	bannedUsers := adminData.GetBannedUsers()
	if len(bannedUsers) != 0 {
		t.Errorf("初始被封禁用户列表应该为空，实际: %d", len(bannedUsers))
	}

	// 封禁用户
	adminData.BanUser(1, true)
	adminData.BanUser(5, true)
	adminData.BanUser(10, true)

	bannedUsers = adminData.GetBannedUsers()
	if len(bannedUsers) != 3 {
		t.Errorf("被封禁用户数量应该是3，实际: %d", len(bannedUsers))
	}

	// 验证用户ID存在
	userMap := make(map[int32]bool)
	for _, userID := range bannedUsers {
		userMap[userID] = true
	}

	if !userMap[1] {
		t.Error("被封禁用户列表应该包含用户1")
	}
	if !userMap[5] {
		t.Error("被封禁用户列表应该包含用户5")
	}
	if !userMap[10] {
		t.Error("被封禁用户列表应该包含用户10")
	}
}

// TestGetRoomBans 测试获取房间封禁列表
func TestGetRoomBans(t *testing.T) {
	adminData := server.NewAdminData()

	// 封禁多个用户进入room1
	adminData.BanUserFromRoom(1, "room1", true)
	adminData.BanUserFromRoom(2, "room1", true)
	adminData.BanUserFromRoom(3, "room1", true)

	roomBans := adminData.GetRoomBans("room1")
	if len(roomBans) != 3 {
		t.Errorf("room1的封禁用户数量应该是3，实际: %d", len(roomBans))
	}

	// 未封禁的房间应该返回空
	emptyBans := adminData.GetRoomBans("room2")
	if len(emptyBans) != 0 {
		t.Errorf("room2的封禁用户列表应该为空，实际: %d", len(emptyBans))
	}
}

// TestAdminDataSaveAndLoad 测试管理员数据保存和加载
func TestAdminDataSaveAndLoad(t *testing.T) {
	// 创建临时目录
	tempDir := t.TempDir()
	dataPath := filepath.Join(tempDir, "admin_data.json")

	// 创建并填充数据
	adminData := server.NewAdminData()
	adminData.BanUser(1, true)
	adminData.BanUser(2, true)
	adminData.BanUserFromRoom(3, "room1", true)
	adminData.BanUserFromRoom(4, "room1", true)
	adminData.BanUserFromRoom(3, "room2", true)

	// 保存数据
	err := adminData.Save(dataPath)
	if err != nil {
		t.Fatalf("保存数据失败: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		t.Fatal("数据文件应该存在")
	}

	// 创建新实例并加载数据
	loadedData := server.NewAdminData()
	err = loadedData.Load(dataPath)
	if err != nil {
		t.Fatalf("加载数据失败: %v", err)
	}

	// 验证数据
	if !loadedData.IsUserBanned(1) {
		t.Error("用户1应该被封禁")
	}
	if !loadedData.IsUserBanned(2) {
		t.Error("用户2应该被封禁")
	}
	if loadedData.IsUserBanned(3) {
		t.Error("用户3不应该被全局封禁")
	}

	// 验证房间封禁
	if !loadedData.IsUserBannedFromRoom(3, "room1") {
		t.Error("用户3应该被禁止进入room1")
	}
	if !loadedData.IsUserBannedFromRoom(4, "room1") {
		t.Error("用户4应该被禁止进入room1")
	}
	if !loadedData.IsUserBannedFromRoom(3, "room2") {
		t.Error("用户3应该被禁止进入room2")
	}
}

// TestAdminDataLoadNonExistent 测试加载不存在的文件
func TestAdminDataLoadNonExistent(t *testing.T) {
	adminData := server.NewAdminData()

	// 加载不存在的文件应该不返回错误（使用空数据）
	err := adminData.Load("/non/existent/path/admin_data.json")
	if err != nil {
		t.Errorf("加载不存在的文件不应该返回错误，实际: %v", err)
	}

	// 数据应该保持为空
	if len(adminData.GetBannedUsers()) != 0 {
		t.Error("加载不存在的文件后，被封禁用户列表应该为空")
	}
}

// TestAdminDataConcurrentAccess 测试并发访问
func TestAdminDataConcurrentAccess(t *testing.T) {
	adminData := server.NewAdminData()

	// 并发封禁用户
	done := make(chan bool, 10)
	for i := int32(1); i <= 10; i++ {
		go func(id int32) {
			adminData.BanUser(id, true)
			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证所有用户都被封禁
	for i := int32(1); i <= 10; i++ {
		if !adminData.IsUserBanned(i) {
			t.Errorf("用户 %d 应该被封禁", i)
		}
	}

	// 并发检查封禁状态
	for i := 0; i < 10; i++ {
		go func() {
			_ = adminData.GetBannedUsers()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestAdminDataPersistence 测试数据持久化
func TestAdminDataPersistence(t *testing.T) {
	tempDir := t.TempDir()
	dataPath := filepath.Join(tempDir, "admin_data.json")

	// 第一轮：创建并保存数据
	adminData1 := server.NewAdminData()
	adminData1.BanUser(100, true)
	adminData1.BanUserFromRoom(200, "test-room", true)

	err := adminData1.Save(dataPath)
	if err != nil {
		t.Fatalf("第一次保存失败: %v", err)
	}

	// 第二轮：加载、修改并保存
	adminData2 := server.NewAdminData()
	err = adminData2.Load(dataPath)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}

	// 添加更多封禁
	adminData2.BanUser(101, true)
	adminData2.BanUserFromRoom(201, "test-room", true)

	err = adminData2.Save(dataPath)
	if err != nil {
		t.Fatalf("第二次保存失败: %v", err)
	}

	// 第三轮：验证所有数据
	adminData3 := server.NewAdminData()
	err = adminData3.Load(dataPath)
	if err != nil {
		t.Fatalf("最终加载失败: %v", err)
	}

	// 验证所有封禁都存在
	if !adminData3.IsUserBanned(100) {
		t.Error("用户100应该被封禁")
	}
	if !adminData3.IsUserBanned(101) {
		t.Error("用户101应该被封禁")
	}
	if !adminData3.IsUserBannedFromRoom(200, "test-room") {
		t.Error("用户200应该被禁止进入test-room")
	}
	if !adminData3.IsUserBannedFromRoom(201, "test-room") {
		t.Error("用户201应该被禁止进入test-room")
	}
}

// TestBanUserTwice 测试重复封禁
func TestBanUserTwice(t *testing.T) {
	adminData := server.NewAdminData()

	// 重复封禁同一用户
	adminData.BanUser(1, true)
	adminData.BanUser(1, true)
	adminData.BanUser(1, true)

	// 应该只记录一次
	bannedUsers := adminData.GetBannedUsers()
	count := 0
	for _, userID := range bannedUsers {
		if userID == 1 {
			count++
		}
	}

	if count != 1 {
		t.Errorf("用户1应该只出现一次，实际出现 %d 次", count)
	}
}

// TestUnbanNonExistentUser 测试解封不存在的用户
func TestUnbanNonExistentUser(t *testing.T) {
	adminData := server.NewAdminData()

	// 解封从未被封禁的用户不应该出错
	adminData.BanUser(999, false)

	if adminData.IsUserBanned(999) {
		t.Error("用户999不应该被封禁")
	}
}

// TestRoomBanCleanup 测试房间封禁清理
func TestRoomBanCleanup(t *testing.T) {
	adminData := server.NewAdminData()

	// 封禁用户
	adminData.BanUserFromRoom(1, "room1", true)
	adminData.BanUserFromRoom(2, "room1", true)

	// 解封所有用户
	adminData.BanUserFromRoom(1, "room1", false)
	adminData.BanUserFromRoom(2, "room1", false)

	// 房间应该被清理
	roomBans := adminData.GetRoomBans("room1")
	if len(roomBans) != 0 {
		t.Errorf("room1的封禁列表应该为空，实际: %d", len(roomBans))
	}
}

// TestLargeUserID 测试大用户ID
func TestLargeUserID(t *testing.T) {
	adminData := server.NewAdminData()

	// 使用大ID
	largeID := int32(2147483647) // max int32
	adminData.BanUser(largeID, true)

	if !adminData.IsUserBanned(largeID) {
		t.Error("大ID用户应该被封禁")
	}

	// 房间封禁
	adminData.BanUserFromRoom(largeID, "room1", true)
	if !adminData.IsUserBannedFromRoom(largeID, "room1") {
		t.Error("大ID用户应该被禁止进入房间")
	}
}

// TestEmptyRoomID 测试空房间ID
func TestEmptyRoomID(t *testing.T) {
	adminData := server.NewAdminData()

	// 使用空字符串作为房间ID
	adminData.BanUserFromRoom(1, "", true)

	if !adminData.IsUserBannedFromRoom(1, "") {
		t.Error("用户应该被禁止进入空ID房间")
	}
}

// TestNegativeUserID 测试负用户ID
func TestNegativeUserID(t *testing.T) {
	adminData := server.NewAdminData()

	// 使用负ID
	negativeID := int32(-1)
	adminData.BanUser(negativeID, true)

	if !adminData.IsUserBanned(negativeID) {
		t.Error("负ID用户应该被封禁")
	}
}
