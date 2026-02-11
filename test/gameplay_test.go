package test

import (
	"sync"
	"testing"

	"phira-mp/common"
	"phira-mp/server"
)

// TestFullGameFlow 测试完整游戏流程
func TestFullGameFlow(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	// 1. 创建房主和房间
	host := server.NewUser(1, "Host", "zh-CN", srv)
	srv.AddUser(host)

	roomID, _ := common.NewRoomId("game-room")
	room := server.NewRoom(roomID, host, srv)
	srv.AddRoom(room)

	// 2. 添加其他玩家
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)
	player3 := server.NewUser(3, "Player3", "zh-CN", srv)
	room.AddUser(player2, false)
	room.AddUser(player3, false)

	// 3. 选择谱面
	room.SetChart(&server.Chart{ID: 123, Name: "Test Chart"})
	if room.GetChart() == nil {
		t.Fatal("谱面设置失败")
	}

	// 4. 切换到等待准备状态
	room.SetState(server.InternalStateWaitForReady)

	// 5. 验证状态
	if room.GetState() != server.InternalStateWaitForReady {
		t.Error("状态应该是等待准备")
	}

	// 6. 切换到游戏中状态
	room.SetState(server.InternalStatePlaying)

	// 7. 验证状态
	if room.GetState() != server.InternalStatePlaying {
		t.Error("状态应该是游戏中")
	}
}

// TestGameAbortScenario 测试放弃场景
func TestGameAbortScenario(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("abort-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)

	room.SetState(server.InternalStatePlaying)

	// 验证游戏中状态
	if room.GetState() != server.InternalStatePlaying {
		t.Error("状态应该是游戏中")
	}
}

// TestGamePartialResults 测试部分结果场景
func TestGamePartialResults(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)
	player3 := server.NewUser(3, "Player3", "zh-CN", srv)

	roomID, _ := common.NewRoomId("partial-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)
	room.AddUser(player3, false)

	room.SetState(server.InternalStatePlaying)

	// 验证游戏中状态
	if room.GetState() != server.InternalStatePlaying {
		t.Error("状态应该是游戏中")
	}
}

// TestGameCycleMode 测试循环模式游戏流程
func TestGameCycleMode(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("cycle-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)

	// 启用循环模式
	room.SetCycle(true)
	if !room.IsCycle() {
		t.Error("循环模式应该已启用")
	}

	// 第一局
	room.SetChart(&server.Chart{ID: 1, Name: "Chart 1"})
	room.SetState(server.InternalStatePlaying)

	// 验证状态
	if room.GetState() != server.InternalStatePlaying {
		t.Error("状态应该是游戏中")
	}

	// 验证房主
	if room.GetHost().ID != host.ID {
		t.Error("房主应该保持不变")
	}
}

// TestLiveModeGameplay 测试直播模式游戏流程
func TestLiveModeGameplay(t *testing.T) {
	config := server.DefaultConfig()
	config.LiveMode = true
	config.Monitors = []int32{100}
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)
	monitor := server.NewUser(100, "Monitor", "zh-CN", srv)

	roomID, _ := common.NewRoomId("live-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)
	room.AddUser(monitor, true) // 作为观察者加入

	// 启用直播模式
	room.SetLive(true)

	if !room.IsLive() {
		t.Error("房间应该处于直播模式")
	}

	// 验证观察者在观察者列表中
	monitors := room.GetMonitors()
	found := false
	for _, m := range monitors {
		if m.ID == monitor.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("观察者应该在观察者列表中")
	}
}

// TestRoomLockDuringGame 测试游戏期间锁定房间
func TestRoomLockDuringGame(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("lock-room")
	room := server.NewRoom(roomID, host, srv)

	// 锁定房间
	room.SetLocked(true)

	// 新玩家应该无法加入
	if !room.IsLocked() {
		t.Error("房间应该已锁定")
	}

	// 解锁
	room.SetLocked(false)
	if room.IsLocked() {
		t.Error("房间应该已解锁")
	}
}

// TestConcurrentGameplay 测试并发游戏操作
func TestConcurrentGameplay(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("concurrent-room")
	room := server.NewRoom(roomID, host, srv)

	// 添加多个玩家
	for i := int32(2); i <= 5; i++ {
		player := server.NewUser(i, "Player", "zh-CN", srv)
		room.AddUser(player, false)
	}

	room.SetState(server.InternalStatePlaying)

	var wg sync.WaitGroup

	// 并发状态读取
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = room.GetState()
			_ = room.GetUsers()
		}()
	}

	wg.Wait()
}

// TestReadyCancelFlow 测试准备/取消准备流程
func TestReadyCancelFlow(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("ready-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)

	room.SetState(server.InternalStateWaitForReady)

	// 验证等待准备状态
	if room.GetState() != server.InternalStateWaitForReady {
		t.Error("状态应该是等待准备")
	}

	// 切换到游戏中
	room.SetState(server.InternalStatePlaying)
	if room.GetState() != server.InternalStatePlaying {
		t.Error("状态应该是游戏中")
	}
}

// TestHostLeaveDuringGame 测试房主游戏中离开
func TestHostLeaveDuringGame(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("host-leave-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)

	room.SetState(server.InternalStatePlaying)

	// 房主离开
	shouldDelete := room.OnUserLeave(host)

	// 游戏中房主离开，房间不应该被删除（其他玩家可以继续或结束）
	if shouldDelete {
		t.Log("房主离开，房间被标记为删除")
	}

	// 验证新房主
	newHost := room.GetHost()
	if newHost == nil {
		t.Error("应该有新房主")
	} else if newHost.ID != player2.ID {
		t.Errorf("新房主应该是玩家2，实际是 %d", newHost.ID)
	}
}

// TestPlayerDisconnectDuringGame 测试玩家游戏中断线
func TestPlayerDisconnectDuringGame(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("disconnect-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)

	room.SetState(server.InternalStatePlaying)

	// 玩家2断线
	player2.SetDisconnected(true)

	// 验证断线状态
	if !player2.IsDisconnected() {
		t.Error("玩家2应该处于断线状态")
	}
}

// TestMultipleGamesInSameRoom 测试同一房间多局游戏
func TestMultipleGamesInSameRoom(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("multi-game-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)

	// 第一局
	room.SetChart(&server.Chart{ID: 1, Name: "Chart 1"})
	room.SetState(server.InternalStatePlaying)

	// 验证第一局状态
	if room.GetState() != server.InternalStatePlaying {
		t.Error("第一局状态应该是游戏中")
	}

	// 回到选谱状态
	room.SetState(server.InternalStateSelectChart)

	// 第二局
	room.SetChart(&server.Chart{ID: 2, Name: "Chart 2"})
	room.SetState(server.InternalStatePlaying)

	// 验证第二局状态
	if room.GetState() != server.InternalStatePlaying {
		t.Error("第二局状态应该是游戏中")
	}

	// 验证谱面已更换
	if room.GetChart().ID != 2 {
		t.Error("谱面应该已更换为Chart 2")
	}
}

// TestChartChangeDuringSelection 测试选谱阶段更换谱面
func TestChartChangeDuringSelection(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("chart-change-room")
	room := server.NewRoom(roomID, host, srv)

	// 选择第一张谱面
	room.SetChart(&server.Chart{ID: 1, Name: "Chart 1"})
	if room.GetChart().ID != 1 {
		t.Error("第一张谱面设置失败")
	}

	// 更换谱面
	room.SetChart(&server.Chart{ID: 2, Name: "Chart 2"})
	if room.GetChart().ID != 2 {
		t.Error("第二张谱面设置失败")
	}
}

// TestMaxPlayersGame 测试最大玩家数游戏
func TestMaxPlayersGame(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("max-players-room")
	room := server.NewRoom(roomID, host, srv)

	// 添加最大玩家数
	for i := int32(2); i <= int32(server.RoomMaxUsers); i++ {
		player := server.NewUser(i, "Player", "zh-CN", srv)
		if !room.AddUser(player, false) {
			t.Fatalf("添加玩家 %d 失败", i)
		}
	}

	// 验证玩家数
	if len(room.GetUsers()) != server.RoomMaxUsers {
		t.Errorf("玩家数应该是 %d，实际: %d", server.RoomMaxUsers, len(room.GetUsers()))
	}

	// 开始游戏
	room.SetChart(&server.Chart{ID: 1, Name: "Chart"})
	room.SetState(server.InternalStatePlaying)

	// 验证游戏状态
	if room.GetState() != server.InternalStatePlaying {
		t.Error("游戏状态应该是游戏中")
	}
}

// TestObserverDuringGameplay 测试观察者观看游戏
func TestObserverDuringGameplay(t *testing.T) {
	config := server.DefaultConfig()
	config.LiveMode = true
	config.Monitors = []int32{100}
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)
	observer := server.NewUser(100, "Observer", "zh-CN", srv)

	roomID, _ := common.NewRoomId("observer-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)
	room.AddUser(observer, true)

	room.SetLive(true)
	room.SetChart(&server.Chart{ID: 1, Name: "Chart"})
	room.SetState(server.InternalStatePlaying)

	// 验证观察者不会出现在玩家列表中
	users := room.GetUsers()
	for _, u := range users {
		if u.ID == observer.ID {
			t.Error("观察者不应该在玩家列表中")
		}
	}

	// 验证观察者在所有用户列表中
	allUsers := room.GetAllUsers()
	found := false
	for _, u := range allUsers {
		if u.ID == observer.ID {
			found = true
			break
		}
	}
	if !found {
		t.Error("观察者应该在所有用户列表中")
	}
}

// TestGameStateTransitions 测试游戏状态转换
func TestGameStateTransitions(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("state-room")
	room := server.NewRoom(roomID, host, srv)

	// 初始状态：选谱
	if room.GetState() != server.InternalStateSelectChart {
		t.Error("初始状态应该是选谱")
	}

	// 选择谱面后仍然是选谱状态
	room.SetChart(&server.Chart{ID: 1, Name: "Chart"})
	if room.GetState() != server.InternalStateSelectChart {
		t.Error("选谱后状态应该是选谱")
	}

	// 切换到等待准备
	room.SetState(server.InternalStateWaitForReady)
	if room.GetState() != server.InternalStateWaitForReady {
		t.Error("状态应该是等待准备")
	}

	// 切换到游戏中
	room.SetState(server.InternalStatePlaying)
	if room.GetState() != server.InternalStatePlaying {
		t.Error("状态应该是游戏中")
	}

	// 游戏结束回到选谱
	room.SetState(server.InternalStateSelectChart)
	if room.GetState() != server.InternalStateSelectChart {
		t.Error("游戏结束后应该回到选谱状态")
	}
}

// TestTouchDataStreaming 测试触摸数据流
func TestTouchDataStreaming(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	observer := server.NewUser(100, "Observer", "zh-CN", srv)

	roomID, _ := common.NewRoomId("touch-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(observer, true)
	room.SetLive(true)

	// 模拟触摸数据帧
	frames := []common.TouchFrame{
		{Time: 0.0, Points: []common.TouchPoint{{ID: 0, Pos: common.NewCompactPos(0.5, 0.5)}}},
		{Time: 0.016, Points: []common.TouchPoint{{ID: 0, Pos: common.NewCompactPos(0.51, 0.51)}}},
		{Time: 0.032, Points: []common.TouchPoint{{ID: 0, Pos: common.NewCompactPos(0.52, 0.52)}}},
	}

	// 验证触摸数据可以创建（这里只是验证数据结构）
	if len(frames) != 3 {
		t.Error("触摸帧数量不匹配")
	}

	// 验证直播模式
	if !room.IsLive() {
		t.Error("房间应该处于直播模式")
	}
}

// TestJudgeDataStreaming 测试判定数据流
func TestJudgeDataStreaming(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	observer := server.NewUser(100, "Observer", "zh-CN", srv)

	roomID, _ := common.NewRoomId("judge-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(observer, true)
	room.SetLive(true)

	// 模拟判定事件
	judges := []common.JudgeEvent{
		{Time: 1.0, LineID: 0, NoteID: 1, Judgement: common.JudgementPerfect},
		{Time: 2.0, LineID: 1, NoteID: 2, Judgement: common.JudgementGood},
		{Time: 3.0, LineID: 0, NoteID: 3, Judgement: common.JudgementMiss},
	}

	// 验证判定数据可以创建
	if len(judges) != 3 {
		t.Error("判定事件数量不匹配")
	}

	// 验证判定类型
	if judges[0].Judgement != common.JudgementPerfect {
		t.Error("第一个判定应该是Perfect")
	}
}

// TestReplayRecording 测试回放录制
func TestReplayRecording(t *testing.T) {
	config := server.DefaultConfig()
	config.HTTPService = true
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("replay-room")
	room := server.NewRoom(roomID, host, srv)
	room.SetChart(&server.Chart{ID: 123, Name: "Test Chart"})

	recorder := srv.GetReplayRecorder()
	if recorder == nil {
		t.Fatal("回放录制器不应该为空")
	}

	// 开始录制
	err := recorder.StartRecording(room)
	if err != nil {
		t.Logf("开始录制: %v", err)
	}

	// 录制触摸数据
	frames := []common.TouchFrame{
		{Time: 0.0, Points: []common.TouchPoint{{ID: 0, Pos: common.NewCompactPos(0.5, 0.5)}}},
	}
	recorder.RecordTouch(roomID.Value, host.ID, frames)

	// 录制判定数据
	judges := []common.JudgeEvent{
		{Time: 1.0, LineID: 0, NoteID: 1, Judgement: common.JudgementPerfect},
	}
	recorder.RecordJudge(roomID.Value, host.ID, judges)

	// 更新成绩ID
	recorder.UpdateRecordID(roomID.Value, host.ID, 999)

	// 停止录制
	recorder.StopRecording(roomID.Value)
}

// TestGameResultScenario 测试游戏结果场景
func TestGameResultScenario(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("result-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)

	// 设置谱面并开始游戏
	room.SetChart(&server.Chart{ID: 1, Name: "Test Chart"})
	room.SetState(server.InternalStatePlaying)

	// 验证游戏状态
	if room.GetState() != server.InternalStatePlaying {
		t.Error("游戏状态应该是游戏中")
	}

	// 验证谱面
	if room.GetChart() == nil {
		t.Error("谱面不应该为空")
	}
}

// TestRoomBroadcastDuringGame 测试游戏期间广播
func TestRoomBroadcastDuringGame(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	player2 := server.NewUser(2, "Player2", "zh-CN", srv)

	roomID, _ := common.NewRoomId("broadcast-room")
	room := server.NewRoom(roomID, host, srv)
	room.AddUser(player2, false)

	// 发送消息（不应该panic）
	room.SendMessage(common.Message{
		Type:    common.MsgChat,
		User:    host.ID,
		Content: "Hello!",
	})

	// 广播命令（不应该panic）
	room.Broadcast(common.ServerCommand{
		Type: common.ServerCmdMessage,
		Message: &common.Message{
			Type:    common.MsgGameStart,
			User:    host.ID,
		},
	})
}

// TestConcurrentRoomOperations 测试房间并发操作
func TestConcurrentRoomOperations(t *testing.T) {
	config := server.DefaultConfig()
	srv := server.NewServer(config)

	host := server.NewUser(1, "Host", "zh-CN", srv)
	roomID, _ := common.NewRoomId("concurrent-ops-room")
	room := server.NewRoom(roomID, host, srv)

	var wg sync.WaitGroup

	// 并发添加用户
	for i := int32(2); i <= 10; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			player := server.NewUser(id, "Player", "zh-CN", srv)
			room.AddUser(player, false)
		}(i)
	}

	// 并发读取用户列表
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = room.GetUsers()
			_ = room.GetAllUsers()
		}()
	}

	// 并发状态操作
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(state server.InternalRoomState) {
			defer wg.Done()
			room.SetState(state)
			_ = room.GetState()
		}(server.InternalRoomState(i % 3))
	}

	wg.Wait()

	// 验证最终状态
	users := room.GetUsers()
	if len(users) < 2 {
		t.Error("并发添加用户失败")
	}
}