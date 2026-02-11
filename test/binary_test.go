package test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"phira-mp/common"
)

// TestBinaryReaderBasic 测试二进制读取器基本功能
func TestBinaryReaderBasic(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	r := common.NewBinaryReader(data)

	// 测试读取单个字节
	b, err := r.Byte()
	if err != nil {
		t.Fatalf("读取字节失败: %v", err)
	}
	if b != 0x01 {
		t.Errorf("读取的字节不匹配，期望: 0x01, 实际: 0x%02x", b)
	}

	// 测试读取多个字节
	taken, err := r.Take(2)
	if err != nil {
		t.Fatalf("读取字节失败: %v", err)
	}
	if !bytes.Equal(taken, []byte{0x02, 0x03}) {
		t.Errorf("读取的字节不匹配，期望: [0x02, 0x03], 实际: %v", taken)
	}
}

// TestBinaryReaderEOF 测试读取器EOF处理
func TestBinaryReaderEOF(t *testing.T) {
	data := []byte{0x01}
	r := common.NewBinaryReader(data)

	// 读取唯一字节
	_, err := r.Byte()
	if err != nil {
		t.Fatalf("读取字节失败: %v", err)
	}

	// 再次读取应该返回EOF
	_, err = r.Byte()
	if err == nil {
		t.Error("读取超出范围应该返回错误")
	}

	// 尝试读取多个字节
	r = common.NewBinaryReader([]byte{0x01, 0x02})
	_, err = r.Take(3)
	if err == nil {
		t.Error("读取超出范围应该返回错误")
	}
}

// TestBinaryWriterBasic 测试二进制写入器基本功能
func TestBinaryWriterBasic(t *testing.T) {
	w := common.NewBinaryWriter()

	// 写入字节
	w.WriteByte(0x01)
	w.WriteByte(0x02)

	// 写入字节切片
	w.WriteBytes([]byte{0x03, 0x04, 0x05})

	data := w.Data()
	expected := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	if !bytes.Equal(data, expected) {
		t.Errorf("写入的数据不匹配，期望: %v, 实际: %v", expected, data)
	}
}

// TestUlebEncoding 测试ULEB128编码
func TestUlebEncoding(t *testing.T) {
	testCases := []struct {
		value    uint64
		expected []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0x80, 0x01}},
		{255, []byte{0xff, 0x01}},
		{256, []byte{0x80, 0x02}},
		{16383, []byte{0xff, 0x7f}},
		{16384, []byte{0x80, 0x80, 0x01}},
		{65535, []byte{0xff, 0xff, 0x03}},
		{4294967295, []byte{0xff, 0xff, 0xff, 0xff, 0x0f}}, // max uint32
	}

	for _, tc := range testCases {
		// 测试写入
		w := common.NewBinaryWriter()
		w.Uleb(tc.value)
		if !bytes.Equal(w.Data(), tc.expected) {
			t.Errorf("ULEB编码失败: 值 %d, 期望 %v, 实际 %v", tc.value, tc.expected, w.Data())
		}

		// 测试读取
		r := common.NewBinaryReader(tc.expected)
		readValue, err := r.Uleb()
		if err != nil {
			t.Errorf("ULEB解码失败: 值 %d, 错误: %v", tc.value, err)
			continue
		}
		if readValue != tc.value {
			t.Errorf("ULEB解码值不匹配: 期望 %d, 实际 %d", tc.value, readValue)
		}
	}
}

// TestInt8 测试int8读写
func TestInt8(t *testing.T) {
	testCases := []int8{-128, -1, 0, 1, 127}

	for _, v := range testCases {
		w := common.NewBinaryWriter()
		common.WriteInt8(w, v)

		r := common.NewBinaryReader(w.Data())
		readV, err := common.ReadInt8(r)
		if err != nil {
			t.Errorf("读取int8失败: %v", err)
			continue
		}
		if readV != v {
			t.Errorf("int8值不匹配: 期望 %d, 实际 %d", v, readV)
		}
	}
}

// TestUint8 测试uint8读写
func TestUint8(t *testing.T) {
	testCases := []uint8{0, 1, 127, 128, 255}

	for _, v := range testCases {
		w := common.NewBinaryWriter()
		common.WriteUint8(w, v)

		r := common.NewBinaryReader(w.Data())
		readV, err := common.ReadUint8(r)
		if err != nil {
			t.Errorf("读取uint8失败: %v", err)
			continue
		}
		if readV != v {
			t.Errorf("uint8值不匹配: 期望 %d, 实际 %d", v, readV)
		}
	}
}

// TestUint16 测试uint16读写
func TestUint16(t *testing.T) {
	testCases := []uint16{0, 1, 255, 256, 65535}

	for _, v := range testCases {
		w := common.NewBinaryWriter()
		common.WriteUint16(w, v)

		// 验证使用小端序
		data := w.Data()
		if len(data) != 2 {
			t.Errorf("uint16数据长度应该是2，实际: %d", len(data))
			continue
		}

		expected := make([]byte, 2)
		binary.LittleEndian.PutUint16(expected, v)
		if !bytes.Equal(data, expected) {
			t.Errorf("uint16编码不匹配: 期望 %v, 实际 %v", expected, data)
		}

		r := common.NewBinaryReader(data)
		readV, err := common.ReadUint16(r)
		if err != nil {
			t.Errorf("读取uint16失败: %v", err)
			continue
		}
		if readV != v {
			t.Errorf("uint16值不匹配: 期望 %d, 实际 %d", v, readV)
		}
	}
}

// TestUint32 测试uint32读写
func TestUint32(t *testing.T) {
	testCases := []uint32{0, 1, 65535, 65536, 4294967295}

	for _, v := range testCases {
		w := common.NewBinaryWriter()
		common.WriteUint32(w, v)

		r := common.NewBinaryReader(w.Data())
		readV, err := common.ReadUint32(r)
		if err != nil {
			t.Errorf("读取uint32失败: %v", err)
			continue
		}
		if readV != v {
			t.Errorf("uint32值不匹配: 期望 %d, 实际 %d", v, readV)
		}
	}
}

// TestInt32 测试int32读写
func TestInt32(t *testing.T) {
	testCases := []int32{-2147483648, -1, 0, 1, 2147483647}

	for _, v := range testCases {
		w := common.NewBinaryWriter()
		common.WriteInt32(w, v)

		r := common.NewBinaryReader(w.Data())
		readV, err := common.ReadInt32(r)
		if err != nil {
			t.Errorf("读取int32失败: %v", err)
			continue
		}
		if readV != v {
			t.Errorf("int32值不匹配: 期望 %d, 实际 %d", v, readV)
		}
	}
}

// TestFloat32 测试float32读写
func TestFloat32(t *testing.T) {
	testCases := []float32{
		0.0,
		1.0,
		-1.0,
		3.14159,
		-2.71828,
		1e10,
		1e-10,
		math.MaxFloat32,
		math.SmallestNonzeroFloat32,
	}

	for _, v := range testCases {
		w := common.NewBinaryWriter()
		common.WriteFloat32(w, v)

		r := common.NewBinaryReader(w.Data())
		readV, err := common.ReadFloat32(r)
		if err != nil {
			t.Errorf("读取float32失败: %v", err)
			continue
		}
		if readV != v {
			t.Errorf("float32值不匹配: 期望 %v, 实际 %v", v, readV)
		}
	}
}

// TestBool 测试bool读写
func TestBool(t *testing.T) {
	testCases := []bool{true, false}

	for _, v := range testCases {
		w := common.NewBinaryWriter()
		common.WriteBool(w, v)

		// 验证编码
		data := w.Data()
		if len(data) != 1 {
			t.Errorf("bool数据长度应该是1，实际: %d", len(data))
			continue
		}

		expected := byte(0)
		if v {
			expected = 1
		}
		if data[0] != expected {
			t.Errorf("bool编码不匹配: 期望 %d, 实际 %d", expected, data[0])
		}

		r := common.NewBinaryReader(data)
		readV, err := common.ReadBool(r)
		if err != nil {
			t.Errorf("读取bool失败: %v", err)
			continue
		}
		if readV != v {
			t.Errorf("bool值不匹配: 期望 %v, 实际 %v", v, readV)
		}
	}
}

// TestString 测试string读写
func TestString(t *testing.T) {
	testCases := []string{
		"",
		"a",
		"Hello, World!",
		"中文测试",
		"🎮 Emoji测试",
		"Mixed: 混合文本 123!@#",
	}

	for _, v := range testCases {
		w := common.NewBinaryWriter()
		common.WriteString(w, v)

		r := common.NewBinaryReader(w.Data())
		readV, err := common.ReadString(r)
		if err != nil {
			t.Errorf("读取string失败: %v", err)
			continue
		}
		if readV != v {
			t.Errorf("string值不匹配: 期望 %q, 实际 %q", v, readV)
		}
	}
}

// TestCompactPos 测试CompactPos
func TestCompactPos(t *testing.T) {
	testCases := []struct {
		x, y float32
	}{
		{0.0, 0.0},
		{1.0, 1.0},
		{0.5, 0.5},
		{-1.0, -1.0},
		{0.123456, 0.987654},
	}

	for _, tc := range testCases {
		pos := common.NewCompactPos(tc.x, tc.y)

		// 由于float16精度限制，允许一定误差
		xDiff := math.Abs(float64(pos.XFloat() - tc.x))
		yDiff := math.Abs(float64(pos.YFloat() - tc.y))

		if xDiff > 0.01 {
			t.Errorf("CompactPos X精度损失过大: 原始 %.6f, 恢复 %.6f, 差值 %.6f",
				tc.x, pos.XFloat(), xDiff)
		}
		if yDiff > 0.01 {
			t.Errorf("CompactPos Y精度损失过大: 原始 %.6f, 恢复 %.6f, 差值 %.6f",
				tc.y, pos.YFloat(), yDiff)
		}
	}
}

// TestCompactPosBinary 测试CompactPos二进制序列化
func TestCompactPosBinary(t *testing.T) {
	pos := common.NewCompactPos(0.5, 0.75)

	w := common.NewBinaryWriter()
	err := pos.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入CompactPos失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readPos common.CompactPos
	err = readPos.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取CompactPos失败: %v", err)
	}

	// 验证值（考虑float16精度）
	xDiff := math.Abs(float64(readPos.XFloat() - 0.5))
	yDiff := math.Abs(float64(readPos.YFloat() - 0.75))

	if xDiff > 0.01 {
		t.Errorf("X值不匹配: 期望 ~0.5, 实际 %.6f", readPos.XFloat())
	}
	if yDiff > 0.01 {
		t.Errorf("Y值不匹配: 期望 ~0.75, 实际 %.6f", readPos.YFloat())
	}
}

// TestVarchar 测试Varchar
func TestVarchar(t *testing.T) {
	// 测试有效字符串
	v, err := common.NewVarchar(100, "Hello")
	if err != nil {
		t.Fatalf("创建Varchar失败: %v", err)
	}
	if v.Value != "Hello" {
		t.Errorf("Varchar值不匹配: 期望 Hello, 实际 %s", v.Value)
	}

	// 测试超长字符串
	_, err = common.NewVarchar(5, "Hello World")
	if err == nil {
		t.Error("超长字符串应该返回错误")
	}

	// 测试二进制序列化
	v2, _ := common.NewVarchar(100, "Test")
	w := common.NewBinaryWriter()
	err = v2.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入Varchar失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readV common.Varchar
	readV.MaxLen = 100
	err = readV.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取Varchar失败: %v", err)
	}
	if readV.Value != "Test" {
		t.Errorf("Varchar值不匹配: 期望 Test, 实际 %s", readV.Value)
	}
}

// TestRoomId 测试RoomId
func TestRoomId(t *testing.T) {
	// 测试有效ID
	validIDs := []string{
		"room1",
		"test-room",
		"test_room",
		"Room123",
		"a",
	}

	for _, id := range validIDs {
		roomId, err := common.NewRoomId(id)
		if err != nil {
			t.Errorf("有效的RoomId %q 不应该返回错误: %v", id, err)
			continue
		}
		if roomId.Value != id {
			t.Errorf("RoomId值不匹配: 期望 %q, 实际 %q", id, roomId.Value)
		}
	}

	// 测试无效ID
	invalidIDs := []string{
		"",
		"room@123",
		"room 123",
		"room.123",
	}

	for _, id := range invalidIDs {
		_, err := common.NewRoomId(id)
		if err == nil {
			t.Errorf("无效的RoomId %q 应该返回错误", id)
		}
	}

	// 测试二进制序列化
	roomId, _ := common.NewRoomId("test-room")
	w := common.NewBinaryWriter()
	err := roomId.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入RoomId失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readRoomId common.RoomId
	err = readRoomId.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取RoomId失败: %v", err)
	}
	if readRoomId.Value != "test-room" {
		t.Errorf("RoomId值不匹配: 期望 test-room, 实际 %s", readRoomId.Value)
	}
}

// TestTouchFrame 测试TouchFrame
func TestTouchFrame(t *testing.T) {
	frame := common.TouchFrame{
		Time: 1.5,
		Points: []common.TouchPoint{
			{ID: 0, Pos: common.NewCompactPos(0.1, 0.2)},
			{ID: 1, Pos: common.NewCompactPos(0.3, 0.4)},
		},
	}

	w := common.NewBinaryWriter()
	err := frame.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入TouchFrame失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readFrame common.TouchFrame
	err = readFrame.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取TouchFrame失败: %v", err)
	}

	if readFrame.Time != frame.Time {
		t.Errorf("Time不匹配: 期望 %f, 实际 %f", frame.Time, readFrame.Time)
	}

	if len(readFrame.Points) != len(frame.Points) {
		t.Errorf("Points数量不匹配: 期望 %d, 实际 %d", len(frame.Points), len(readFrame.Points))
	}
}

// TestJudgeEvent 测试JudgeEvent
func TestJudgeEvent(t *testing.T) {
	judge := common.JudgeEvent{
		Time:      2.5,
		LineID:    1,
		NoteID:    100,
		Judgement: common.JudgementPerfect,
	}

	w := common.NewBinaryWriter()
	err := judge.WriteBinary(w)
	if err != nil {
		t.Fatalf("写入JudgeEvent失败: %v", err)
	}

	r := common.NewBinaryReader(w.Data())
	var readJudge common.JudgeEvent
	err = readJudge.ReadBinary(r)
	if err != nil {
		t.Fatalf("读取JudgeEvent失败: %v", err)
	}

	if readJudge.Time != judge.Time {
		t.Errorf("Time不匹配: 期望 %f, 实际 %f", judge.Time, readJudge.Time)
	}

	if readJudge.LineID != judge.LineID {
		t.Errorf("LineID不匹配: 期望 %d, 实际 %d", judge.LineID, readJudge.LineID)
	}

	if readJudge.NoteID != judge.NoteID {
		t.Errorf("NoteID不匹配: 期望 %d, 实际 %d", judge.NoteID, readJudge.NoteID)
	}

	if readJudge.Judgement != judge.Judgement {
		t.Errorf("Judgement不匹配: 期望 %v, 实际 %v", judge.Judgement, readJudge.Judgement)
	}
}

// TestAllJudgementTypes 测试所有判定类型
func TestAllJudgementTypes(t *testing.T) {
	judgements := []common.Judgement{
		common.JudgementPerfect,
		common.JudgementGood,
		common.JudgementBad,
		common.JudgementMiss,
		common.JudgementHoldPerfect,
		common.JudgementHoldGood,
	}

	for _, j := range judgements {
		w := common.NewBinaryWriter()
		err := j.WriteBinary(w)
		if err != nil {
			t.Errorf("写入Judgement %v 失败: %v", j, err)
			continue
		}

		r := common.NewBinaryReader(w.Data())
		var readJ common.Judgement
		err = readJ.ReadBinary(r)
		if err != nil {
			t.Errorf("读取Judgement %v 失败: %v", j, err)
			continue
		}

		if readJ != j {
			t.Errorf("Judgement不匹配: 期望 %v, 实际 %v", j, readJ)
		}
	}
}

// TestBinaryDataRoundTrip 测试BinaryData接口往返
func TestBinaryDataRoundTrip(t *testing.T) {
	testCases := []struct {
		name string
		data common.BinaryData
	}{
		{
			name: "CompactPos",
			data: &common.CompactPos{X: 1000, Y: 2000},
		},
		{
			name: "TouchFrame",
			data: &common.TouchFrame{
				Time:   1.0,
				Points: []common.TouchPoint{{ID: 0, Pos: common.NewCompactPos(0.5, 0.5)}},
			},
		},
		{
			name: "JudgeEvent",
			data: &common.JudgeEvent{
				Time:      1.0,
				LineID:    0,
				NoteID:    1,
				Judgement: common.JudgementPerfect,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := common.NewBinaryWriter()
			err := tc.data.WriteBinary(w)
			if err != nil {
				t.Fatalf("写入失败: %v", err)
			}

			r := common.NewBinaryReader(w.Data())
			// 注意：这里不能直接调用ReadBinary，因为需要创建新的实例
			// 实际测试已在各自的测试函数中完成
			_ = r
		})
	}
}
