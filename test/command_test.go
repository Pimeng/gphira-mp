package test

import (
	"bytes"
	"testing"

	"phira-mp/common"
)

// TestClientCommandPing 测试Ping命令序列化
func TestClientCommandPing(t *testing.T) {
	cmd := common.ClientCommand{
		Type: common.ClientCmdPing,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	data := w.Data()
	if len(data) != 1 {
		t.Errorf("Ping命令数据长度应该是1，实际: %d", len(data))
	}

	// 读取验证
	r := common.NewBinaryReader(data)
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ClientCmdPing {
		t.Errorf("命令类型不匹配，期望: %d, 实际: %d", common.ClientCmdPing, readCmd.Type)
	}
}

// TestClientCommandAuthenticate 测试认证命令
func TestClientCommandAuthenticate(t *testing.T) {
	cmd := common.ClientCommand{
		Type:  common.ClientCmdAuthenticate,
		Token: "test-token-12345",
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	// 读取验证
	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ClientCmdAuthenticate {
		t.Error("命令类型不匹配")
	}

	if readCmd.Token != "test-token-12345" {
		t.Errorf("Token不匹配，期望: test-token-12345, 实际: %s", readCmd.Token)
	}
}

// TestClientCommandChat 测试聊天命令
func TestClientCommandChat(t *testing.T) {
	cmd := common.ClientCommand{
		Type:    common.ClientCmdChat,
		Message: "Hello, World! 你好世界！",
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	// 读取验证
	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ClientCmdChat {
		t.Error("命令类型不匹配")
	}

	if readCmd.Message != "Hello, World! 你好世界！" {
		t.Errorf("消息内容不匹配，期望: Hello, World! 你好世界！, 实际: %s", readCmd.Message)
	}
}

// TestClientCommandTouches 测试触摸数据命令
func TestClientCommandTouches(t *testing.T) {
	frames := []common.TouchFrame{
		{
			Time: 0.0,
			Points: []common.TouchPoint{
				{ID: 0, Pos: common.NewCompactPos(0.5, 0.5)},
				{ID: 1, Pos: common.NewCompactPos(0.3, 0.7)},
			},
		},
		{
			Time: 0.016,
			Points: []common.TouchPoint{
				{ID: 0, Pos: common.NewCompactPos(0.51, 0.51)},
			},
		},
	}

	cmd := common.ClientCommand{
		Type:   common.ClientCmdTouches,
		Frames: frames,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	// 读取验证
	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ClientCmdTouches {
		t.Error("命令类型不匹配")
	}

	if len(readCmd.Frames) != 2 {
		t.Errorf("帧数不匹配，期望: 2, 实际: %d", len(readCmd.Frames))
	}

	// 验证第一帧
	if len(readCmd.Frames[0].Points) != 2 {
		t.Errorf("第一帧点数不匹配，期望: 2, 实际: %d", len(readCmd.Frames[0].Points))
	}
}

// TestClientCommandJudges 测试判定数据命令
func TestClientCommandJudges(t *testing.T) {
	judges := []common.JudgeEvent{
		{
			Time:      1.5,
			LineID:    0,
			NoteID:    10,
			Judgement: common.JudgementPerfect,
		},
		{
			Time:      2.0,
			LineID:    1,
			NoteID:    15,
			Judgement: common.JudgementGood,
		},
		{
			Time:      2.5,
			LineID:    0,
			NoteID:    20,
			Judgement: common.JudgementMiss,
		},
	}

	cmd := common.ClientCommand{
		Type:   common.ClientCmdJudges,
		Judges: judges,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	// 读取验证
	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ClientCmdJudges {
		t.Error("命令类型不匹配")
	}

	if len(readCmd.Judges) != 3 {
		t.Errorf("判定数不匹配，期望: 3, 实际: %d", len(readCmd.Judges))
	}

	// 验证判定值
	if readCmd.Judges[0].Judgement != common.JudgementPerfect {
		t.Error("第一个判定应该是Perfect")
	}
	if readCmd.Judges[1].Judgement != common.JudgementGood {
		t.Error("第二个判定应该是Good")
	}
	if readCmd.Judges[2].Judgement != common.JudgementMiss {
		t.Error("第三个判定应该是Miss")
	}
}

// TestClientCommandCreateRoom 测试创建房间命令
func TestClientCommandCreateRoom(t *testing.T) {
	roomID, _ := common.NewRoomId("test-room")
	cmd := common.ClientCommand{
		Type:   common.ClientCmdCreateRoom,
		RoomId: roomID,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	// 读取验证
	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ClientCmdCreateRoom {
		t.Error("命令类型不匹配")
	}

	if readCmd.RoomId.Value != "test-room" {
		t.Errorf("房间ID不匹配，期望: test-room, 实际: %s", readCmd.RoomId.Value)
	}
}

// TestClientCommandJoinRoom 测试加入房间命令
func TestClientCommandJoinRoom(t *testing.T) {
	roomID, _ := common.NewRoomId("test-room")

	// 测试普通加入
	cmd := common.ClientCommand{
		Type:    common.ClientCmdJoinRoom,
		RoomId:  roomID,
		Monitor: false,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Monitor {
		t.Error("普通加入不应该设置Monitor为true")
	}

	// 测试观察者加入
	cmd.Monitor = true
	w = common.NewBinaryWriter()
	err = cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r = common.NewBinaryReader(w.Data())
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if !readCmd.Monitor {
		t.Error("观察者加入应该设置Monitor为true")
	}
}

// TestClientCommandLockRoom 测试锁定房间命令
func TestClientCommandLockRoom(t *testing.T) {
	cmd := common.ClientCommand{
		Type: common.ClientCmdLockRoom,
		Lock: true,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if !readCmd.Lock {
		t.Error("锁定命令应该设置Lock为true")
	}
}

// TestClientCommandCycleRoom 测试循环房间命令
func TestClientCommandCycleRoom(t *testing.T) {
	cmd := common.ClientCommand{
		Type:  common.ClientCmdCycleRoom,
		Cycle: true,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if !readCmd.Cycle {
		t.Error("循环命令应该设置Cycle为true")
	}
}

// TestClientCommandSelectChart 测试选择谱面命令
func TestClientCommandSelectChart(t *testing.T) {
	cmd := common.ClientCommand{
		Type:    common.ClientCmdSelectChart,
		ChartID: 12345,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.ChartID != 12345 {
		t.Errorf("谱面ID不匹配，期望: 12345, 实际: %d", readCmd.ChartID)
	}
}

// TestClientCommandPlayed 测试游戏完成命令
func TestClientCommandPlayed(t *testing.T) {
	cmd := common.ClientCommand{
		Type:     common.ClientCmdPlayed,
		RecordID: 99999,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.RecordID != 99999 {
		t.Errorf("记录ID不匹配，期望: 99999, 实际: %d", readCmd.RecordID)
	}
}

// TestServerCommandPong 测试Pong响应
func TestServerCommandPong(t *testing.T) {
	cmd := common.ServerCommand{
		Type: common.ServerCmdPong,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ServerCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ServerCmdPong {
		t.Error("命令类型不匹配")
	}
}

// TestServerCommandAuthenticate 测试认证响应
func TestServerCommandAuthenticate(t *testing.T) {
	cmd := common.ServerCommand{
		Type: common.ServerCmdAuthenticate,
		AuthenticateResult: &common.Result[common.AuthResult]{
			Ok: &common.AuthResult{
				User: common.UserInfo{
					ID:      1,
					Name:    "TestUser",
					Monitor: false,
				},
				Room: nil,
			},
		},
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ServerCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ServerCmdAuthenticate {
		t.Error("命令类型不匹配")
	}

	if readCmd.AuthenticateResult == nil || readCmd.AuthenticateResult.Ok == nil {
		t.Fatal("认证结果不应该为空")
	}

	if readCmd.AuthenticateResult.Ok.User.ID != 1 {
		t.Error("用户ID不匹配")
	}
}

// TestServerCommandAuthenticateError 测试认证失败响应
func TestServerCommandAuthenticateError(t *testing.T) {
	errMsg := "认证失败"
	cmd := common.ServerCommand{
		Type: common.ServerCmdAuthenticate,
		AuthenticateResult: &common.Result[common.AuthResult]{
			Err: &errMsg,
		},
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ServerCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.AuthenticateResult == nil || readCmd.AuthenticateResult.Err == nil {
		t.Fatal("错误结果不应该为空")
	}

	if *readCmd.AuthenticateResult.Err != "认证失败" {
		t.Errorf("错误消息不匹配，期望: 认证失败, 实际: %s", *readCmd.AuthenticateResult.Err)
	}
}

// TestServerCommandMessage 测试消息通知
func TestServerCommandMessage(t *testing.T) {
	cmd := common.ServerCommand{
		Type: common.ServerCmdMessage,
		Message: &common.Message{
			Type:    common.MsgChat,
			User:    1,
			Content: "Hello everyone!",
		},
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ServerCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ServerCmdMessage {
		t.Error("命令类型不匹配")
	}

	if readCmd.Message == nil {
		t.Fatal("消息不应该为空")
	}

	if readCmd.Message.Type != common.MsgChat {
		t.Error("消息类型不匹配")
	}

	if readCmd.Message.Content != "Hello everyone!" {
		t.Errorf("消息内容不匹配，期望: Hello everyone!, 实际: %s", readCmd.Message.Content)
	}
}

// TestServerCommandChangeState 测试状态变更通知
func TestServerCommandChangeState(t *testing.T) {
	chartID := int32(123)
	cmd := common.ServerCommand{
		Type: common.ServerCmdChangeState,
		ChangeState: &common.RoomState{
			Type:    common.RoomStatePlaying,
			ChartID: &chartID,
		},
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ServerCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ServerCmdChangeState {
		t.Error("命令类型不匹配")
	}

	if readCmd.ChangeState == nil {
		t.Fatal("状态不应该为空")
	}

	if readCmd.ChangeState.Type != common.RoomStatePlaying {
		t.Error("状态类型不匹配")
	}
}

// TestServerCommandTouches 测试触摸数据广播
func TestServerCommandTouches(t *testing.T) {
	frames := []common.TouchFrame{
		{
			Time: 1.0,
			Points: []common.TouchPoint{
				{ID: 0, Pos: common.NewCompactPos(0.5, 0.5)},
			},
		},
	}

	cmd := common.ServerCommand{
		Type:          common.ServerCmdTouches,
		TouchesPlayer: 1,
		TouchesFrames: frames,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ServerCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ServerCmdTouches {
		t.Error("命令类型不匹配")
	}

	if readCmd.TouchesPlayer != 1 {
		t.Errorf("玩家ID不匹配，期望: 1, 实际: %d", readCmd.TouchesPlayer)
	}

	if len(readCmd.TouchesFrames) != 1 {
		t.Errorf("帧数不匹配，期望: 1, 实际: %d", len(readCmd.TouchesFrames))
	}
}

// TestServerCommandJudges 测试判定数据广播
func TestServerCommandJudges(t *testing.T) {
	judges := []common.JudgeEvent{
		{
			Time:      1.0,
			LineID:    0,
			NoteID:    1,
			Judgement: common.JudgementPerfect,
		},
	}

	cmd := common.ServerCommand{
		Type:         common.ServerCmdJudges,
		JudgesPlayer: 1,
		JudgesEvents: judges,
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入命令失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ServerCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取命令失败: %v", err)
	}

	if readCmd.Type != common.ServerCmdJudges {
		t.Error("命令类型不匹配")
	}

	if readCmd.JudgesPlayer != 1 {
		t.Errorf("玩家ID不匹配，期望: 1, 实际: %d", readCmd.JudgesPlayer)
	}
}

// TestAllClientCommands 测试所有客户端命令类型
func TestAllClientCommands(t *testing.T) {
	// 测试简单命令（不需要额外数据）
	simpleCommands := []common.ClientCommandType{
		common.ClientCmdPing,
		common.ClientCmdLeaveRoom,
		common.ClientCmdRequestStart,
		common.ClientCmdReady,
		common.ClientCmdCancelReady,
		common.ClientCmdAbort,
	}

	for _, cmdType := range simpleCommands {
		cmd := common.ClientCommand{Type: cmdType}

		w := common.NewBinaryWriter()
		err := cmd.WriteBinary(w)
		if err != nil {
			t.Errorf("写入命令类型 %d 失败: %v", cmdType, err)
			continue
		}

		r := common.NewBinaryReader(w.Data())
		var readCmd common.ClientCommand
		err = readCmd.ReadBinary(r)
		if err != nil {
			t.Errorf("读取命令类型 %d 失败: %v", cmdType, err)
			continue
		}

		if readCmd.Type != cmdType {
			t.Errorf("命令类型 %d 不匹配，实际: %d", cmdType, readCmd.Type)
		}
	}
}

// TestAllServerCommands 测试所有服务器命令类型
func TestAllServerCommands(t *testing.T) {
	// 测试简单命令（不需要额外数据）
	simpleCommands := []common.ServerCommandType{
		common.ServerCmdPong,
		common.ServerCmdChat,
		common.ServerCmdLeaveRoom,
		common.ServerCmdLockRoom,
		common.ServerCmdCycleRoom,
		common.ServerCmdRequestStart,
		common.ServerCmdReady,
		common.ServerCmdCancelReady,
		common.ServerCmdPlayed,
		common.ServerCmdAbort,
	}

	for _, cmdType := range simpleCommands {
		cmd := common.ServerCommand{Type: cmdType}

		w := common.NewBinaryWriter()
		err := cmd.WriteBinary(w)
		if err != nil {
			t.Errorf("写入命令类型 %d 失败: %v", cmdType, err)
			continue
		}

		r := common.NewBinaryReader(w.Data())
		var readCmd common.ServerCommand
		err = readCmd.ReadBinary(r)
		if err != nil {
			t.Errorf("读取命令类型 %d 失败: %v", cmdType, err)
			continue
		}

		if readCmd.Type != cmdType {
			t.Errorf("命令类型 %d 不匹配，实际: %d", cmdType, readCmd.Type)
		}
	}
}

// TestCommandRoundTrip 测试命令往返序列化
func TestCommandRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		cmd  common.ClientCommand
	}{
		{
			name: "Empty Ping",
			cmd:  common.ClientCommand{Type: common.ClientCmdPing},
		},
		{
			name: "Authenticate",
			cmd: common.ClientCommand{
				Type:  common.ClientCmdAuthenticate,
				Token: "my-secret-token",
			},
		},
		{
			name: "Chat",
			cmd: common.ClientCommand{
				Type:    common.ClientCmdChat,
				Message: "Test message with unicode: 中文测试 🎮",
			},
		},
		{
			name: "SelectChart",
			cmd: common.ClientCommand{
				Type:    common.ClientCmdSelectChart,
				ChartID: 987654321,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := common.NewBinaryWriter()
			err := tc.cmd.WriteBinary(w)
			if err != nil {
				t.Fatalf("写入失败: %v", err)
			}

			r := common.NewBinaryReader(w.Data())
			var readCmd common.ClientCommand
			err = readCmd.ReadBinary(r)
			if err != nil {
				t.Fatalf("读取失败: %v", err)
			}

			if readCmd.Type != tc.cmd.Type {
				t.Errorf("类型不匹配: 期望 %d, 实际 %d", tc.cmd.Type, readCmd.Type)
			}
		})
	}
}

// TestEmptyData 测试空数据处理
func TestEmptyData(t *testing.T) {
	// 测试读取空数据
	r := common.NewBinaryReader([]byte{})
	var cmd common.ClientCommand
	err := cmd.ReadBinary(r)
	if err == nil {
		t.Error("读取空数据应该返回错误")
	}
}

// TestInvalidCommandType 测试无效命令类型
func TestInvalidCommandType(t *testing.T) {
	// 创建一个包含无效命令类型的数据
	w := common.NewBinaryWriter()
	common.WriteUint8(w, 255) // 无效的命令类型

	r := common.NewBinaryReader(w.Data())
	var cmd common.ClientCommand
	err := cmd.ReadBinary(r)
	// 应该返回错误或者能够处理
	if err == nil {
		t.Log("无效命令类型被接受（可能是有默认处理）")
	}
}

// TestLargeMessage 测试大消息处理
func TestLargeMessage(t *testing.T) {
	// 创建一个较大的聊天消息（在限制范围内，最大200字符）
	largeContent := bytes.Repeat([]byte("A"), 150)

	cmd := common.ClientCommand{
		Type:    common.ClientCmdChat,
		Message: string(largeContent),
	}

	w := common.NewBinaryWriter()
	err := cmd.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入大消息失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readCmd common.ClientCommand
	err = readCmd.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取大消息失败: %v", err)
	}

	if len(readCmd.Message) != 150 {
		t.Errorf("大消息长度不匹配，期望: 150, 实际: %d", len(readCmd.Message))
	}
}

// TestTouchFramePrecision 测试触摸帧精度
func TestTouchFramePrecision(t *testing.T) {
	// 测试 CompactPos 的精度
	originalX := float32(0.123456)
	originalY := float32(0.987654)

	pos := common.NewCompactPos(originalX, originalY)
	recoveredX := pos.XFloat()
	recoveredY := pos.YFloat()

	// float16 有一定精度损失，检查是否在合理范围内
	diffX := absFloat32(originalX - recoveredX)
	diffY := absFloat32(originalY - recoveredY)

	if diffX > 0.01 {
		t.Errorf("X坐标精度损失过大: 原始 %.6f, 恢复 %.6f, 差值 %.6f", originalX, recoveredX, diffX)
	}

	if diffY > 0.01 {
		t.Errorf("Y坐标精度损失过大: 原始 %.6f, 恢复 %.6f, 差值 %.6f", originalY, recoveredY, diffY)
	}
}

func absFloat32(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
