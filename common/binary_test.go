package common

import (
	"testing"
)

func TestBinaryWriter(t *testing.T) {
	w := NewBinaryWriter()

	// 测试基本类型
	WriteUint8(w, 0x12)
	WriteUint16(w, 0x1234)
	WriteUint32(w, 0x12345678)
	WriteInt32(w, -42)
	WriteFloat32(w, 3.14)
	WriteBool(w, true)
	WriteString(w, "hello")

	data := w.Data()
	if len(data) == 0 {
		t.Error("Expected non-empty data")
	}
}

func TestBinaryReader(t *testing.T) {
	w := NewBinaryWriter()
	WriteUint8(w, 0x12)
	WriteUint16(w, 0x1234)
	WriteUint32(w, 0x12345678)
	WriteInt32(w, -42)
	WriteFloat32(w, 3.14)
	WriteBool(w, true)
	WriteString(w, "hello")

	r := NewBinaryReader(w.Data())

	// 读取并验证
	if v, err := ReadUint8(r); err != nil || v != 0x12 {
		t.Errorf("ReadUint8 failed: got %v, err %v", v, err)
	}
	if v, err := ReadUint16(r); err != nil || v != 0x1234 {
		t.Errorf("ReadUint16 failed: got %v, err %v", v, err)
	}
	if v, err := ReadUint32(r); err != nil || v != 0x12345678 {
		t.Errorf("ReadUint32 failed: got %v, err %v", v, err)
	}
	if v, err := ReadInt32(r); err != nil || v != -42 {
		t.Errorf("ReadInt32 failed: got %v, err %v", v, err)
	}
	if v, err := ReadFloat32(r); err != nil || v < 3.13 || v > 3.15 {
		t.Errorf("ReadFloat32 failed: got %v, err %v", v, err)
	}
	if v, err := ReadBool(r); err != nil || !v {
		t.Errorf("ReadBool failed: got %v, err %v", v, err)
	}
	if v, err := ReadString(r); err != nil || v != "hello" {
		t.Errorf("ReadString failed: got %v, err %v", v, err)
	}
}

func TestUleb128(t *testing.T) {
	testCases := []uint64{
		0,
		1,
		127,
		128,
		16383,
		16384,
		65535,
		4294967295,
	}

	for _, tc := range testCases {
		w := NewBinaryWriter()
		w.Uleb(tc)

		r := NewBinaryReader(w.Data())
		v, err := r.Uleb()
		if err != nil {
			t.Errorf("ULEB128 decode error for %d: %v", tc, err)
		}
		if v != tc {
			t.Errorf("ULEB128 mismatch: expected %d, got %d", tc, v)
		}
	}
}

func TestCompactPos(t *testing.T) {
	pos := NewCompactPos(1.5, 2.5)

	w := NewBinaryWriter()
	pos.WriteBinary(w)

	r := NewBinaryReader(w.Data())
	var decoded CompactPos
	if err := decoded.ReadBinary(r); err != nil {
		t.Errorf("ReadBinary error: %v", err)
	}

	// 由于float16精度损失，允许一定误差
	if diff := pos.XFloat() - decoded.XFloat(); diff < -0.01 || diff > 0.01 {
		t.Errorf("X mismatch: expected %v, got %v", pos.XFloat(), decoded.XFloat())
	}
	if diff := pos.YFloat() - decoded.YFloat(); diff < -0.01 || diff > 0.01 {
		t.Errorf("Y mismatch: expected %v, got %v", pos.YFloat(), decoded.YFloat())
	}
}

func TestRoomId(t *testing.T) {
	// 测试有效ID
	validIDs := []string{"room1", "test-room", "test_room", "Room123"}
	for _, id := range validIDs {
		roomId, err := NewRoomId(id)
		if err != nil {
			t.Errorf("Expected valid ID for %s, got error: %v", id, err)
		}

		w := NewBinaryWriter()
		roomId.WriteBinary(w)

		r := NewBinaryReader(w.Data())
		var decoded RoomId
		if err := decoded.ReadBinary(r); err != nil {
			t.Errorf("ReadBinary error: %v", err)
		}
		if decoded.Value != id {
			t.Errorf("RoomId mismatch: expected %s, got %s", id, decoded.Value)
		}
	}

	// 测试无效ID
	_, err := NewRoomId("")
	if err == nil {
		t.Error("Expected error for empty ID")
	}
}

func TestTouchFrame(t *testing.T) {
	frame := TouchFrame{
		Time: 1.5,
		Points: []TouchPoint{
			{ID: 0, Pos: NewCompactPos(0.5, 0.5)},
			{ID: 1, Pos: NewCompactPos(0.6, 0.6)},
		},
	}

	w := NewBinaryWriter()
	frame.WriteBinary(w)

	r := NewBinaryReader(w.Data())
	var decoded TouchFrame
	if err := decoded.ReadBinary(r); err != nil {
		t.Errorf("ReadBinary error: %v", err)
	}

	if len(decoded.Points) != len(frame.Points) {
		t.Errorf("Points length mismatch: expected %d, got %d", len(frame.Points), len(decoded.Points))
	}
}

func TestClientCommand(t *testing.T) {
	cmd := ClientCommand{
		Type:    ClientCmdAuthenticate,
		Token:   "test-token-123",
		Message: "",
	}

	w := NewBinaryWriter()
	cmd.WriteBinary(w)

	r := NewBinaryReader(w.Data())
	var decoded ClientCommand
	if err := decoded.ReadBinary(r); err != nil {
		t.Errorf("ReadBinary error: %v", err)
	}

	if decoded.Type != cmd.Type {
		t.Errorf("Type mismatch: expected %v, got %v", cmd.Type, decoded.Type)
	}
	if decoded.Token != cmd.Token {
		t.Errorf("Token mismatch: expected %s, got %s", cmd.Token, decoded.Token)
	}
}

func TestServerCommand(t *testing.T) {
	cmd := ServerCommand{
		Type: ServerCmdPong,
	}

	w := NewBinaryWriter()
	cmd.WriteBinary(w)

	r := NewBinaryReader(w.Data())
	var decoded ServerCommand
	if err := decoded.ReadBinary(r); err != nil {
		t.Errorf("ReadBinary error: %v", err)
	}

	if decoded.Type != cmd.Type {
		t.Errorf("Type mismatch: expected %v, got %v", cmd.Type, decoded.Type)
	}
}

func BenchmarkUleb128(b *testing.B) {
	for i := 0; i < b.N; i++ {
		w := NewBinaryWriter()
		w.Uleb(123456789)
		r := NewBinaryReader(w.Data())
		r.Uleb()
	}
}

func BenchmarkBinaryData(b *testing.B) {
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := NewBinaryWriter()
		w.WriteBytes(data)
		r := NewBinaryReader(w.Data())
		r.Take(len(data))
	}
}
