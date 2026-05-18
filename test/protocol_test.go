package test

import (
	"bytes"
	"math"
	"testing"

	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

func TestBinaryReaderWriter(t *testing.T) {
	w := protocol.NewBinaryWriter()
	w.WriteU8(0)
	w.WriteU8(255)
	w.WriteU8(128)
	if !bytes.Equal(w.Bytes(), []byte{0x00, 0xff, 0x80}) {
		t.Errorf("WriteU8 failed: %x", w.Bytes())
	}

	w = protocol.NewBinaryWriter()
	w.WriteU16(0x1234)
	if !bytes.Equal(w.Bytes(), []byte{0x34, 0x12}) {
		t.Errorf("WriteU16 LE failed: %x", w.Bytes())
	}

	w = protocol.NewBinaryWriter()
	w.WriteU32(0xdeadbeef)
	if !bytes.Equal(w.Bytes(), []byte{0xef, 0xbe, 0xad, 0xde}) {
		t.Errorf("WriteU32 LE failed: %x", w.Bytes())
	}

	w = protocol.NewBinaryWriter()
	w.WriteI32(-1)
	if !bytes.Equal(w.Bytes(), []byte{0xff, 0xff, 0xff, 0xff}) {
		t.Errorf("WriteI32 failed: %x", w.Bytes())
	}

	w = protocol.NewBinaryWriter()
	w.WriteU64(0x123456789abcdef0)
	if !bytes.Equal(w.Bytes(), []byte{0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12}) {
		t.Errorf("WriteU64 LE failed: %x", w.Bytes())
	}

	w = protocol.NewBinaryWriter()
	w.WriteI64(-1)
	if !bytes.Equal(w.Bytes(), []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}) {
		t.Errorf("WriteI64 failed: %x", w.Bytes())
	}

	w = protocol.NewBinaryWriter()
	w.WriteF32(3.14)
	if !bytes.Equal(w.Bytes(), []byte{0xc3, 0xf5, 0x48, 0x40}) {
		t.Errorf("WriteF32 failed: %x", w.Bytes())
	}

	w = protocol.NewBinaryWriter()
	w.WriteUleb(128)
	if !bytes.Equal(w.Bytes(), []byte{0x80, 0x01}) {
		t.Errorf("WriteUleb(128) failed: %x", w.Bytes())
	}

	w = protocol.NewBinaryWriter()
	w.WriteString("hello")
	if !bytes.Equal(w.Bytes(), []byte{0x05, 0x68, 0x65, 0x6c, 0x6c, 0x6f}) {
		t.Errorf("WriteString failed: %x", w.Bytes())
	}

	// Round-trip test
	w = protocol.NewBinaryWriter()
	w.WriteBool(true)
	w.WriteBool(false)
	w.WriteI8(-1)
	w.WriteU16(0x1234)
	w.WriteU32(0xdeadbeef)
	w.WriteI32(-1)
	w.WriteU64(0x123456789abcdef0)
	w.WriteI64(-1)
	w.WriteF32(3.14)
	w.WriteUleb(128)
	w.WriteString("hello")

	r := protocol.NewBinaryReader(w.Bytes())
	if !r.ReadBool() {
		t.Error("ReadBool true failed")
	}
	if r.ReadBool() {
		t.Error("ReadBool false failed")
	}
	if r.ReadI8() != -1 {
		t.Error("ReadI8 failed")
	}
	if r.ReadU16() != 0x1234 {
		t.Error("ReadU16 failed")
	}
	if r.ReadU32() != 0xdeadbeef {
		t.Error("ReadU32 failed")
	}
	if r.ReadI32() != -1 {
		t.Error("ReadI32 failed")
	}
	if r.ReadU64() != 0x123456789abcdef0 {
		t.Error("ReadU64 failed")
	}
	if r.ReadI64() != -1 {
		t.Error("ReadI64 failed")
	}
	if math.Abs(float64(r.ReadF32()-3.14)) > 0.001 {
		t.Error("ReadF32 failed")
	}
	if r.ReadUlebBigInt() != 128 {
		t.Error("ReadUleb failed")
	}
	if r.ReadString() != "hello" {
		t.Error("ReadString failed")
	}
}

func TestBinaryReaderEof(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic on unexpected EOF")
		}
	}()
	r := protocol.NewBinaryReader([]byte{0x01})
	r.ReadU16()
}

func TestFraming(t *testing.T) {
	// Encode 0
	if !bytes.Equal(protocol.EncodeLengthPrefixU32(0), []byte{0x00}) {
		t.Error("EncodeLengthPrefixU32(0) failed")
	}
	// Encode 127
	if !bytes.Equal(protocol.EncodeLengthPrefixU32(127), []byte{0x7f}) {
		t.Error("EncodeLengthPrefixU32(127) failed")
	}
	// Encode 128
	if !bytes.Equal(protocol.EncodeLengthPrefixU32(128), []byte{0x80, 0x01}) {
		t.Error("EncodeLengthPrefixU32(128) failed")
	}
	// Encode 16383
	if !bytes.Equal(protocol.EncodeLengthPrefixU32(16383), []byte{0xff, 0x7f}) {
		t.Error("EncodeLengthPrefixU32(16383) failed")
	}
	// Encode 16384
	if !bytes.Equal(protocol.EncodeLengthPrefixU32(16384), []byte{0x80, 0x80, 0x01}) {
		t.Error("EncodeLengthPrefixU32(16384) failed")
	}
	// Encode max u32
	if !bytes.Equal(protocol.EncodeLengthPrefixU32(0xffffffff), []byte{0xff, 0xff, 0xff, 0xff, 0x0f}) {
		t.Error("EncodeLengthPrefixU32(0xffffffff) failed")
	}

	// Decode empty
	res := protocol.TryDecodeFrame([]byte{}, protocol.MaxPayloadBytes)
	if !res.NeedMore {
		t.Error("expected need_more for empty buffer")
	}

	// Decode single frame
	frame := append(protocol.EncodeLengthPrefixU32(5), []byte("hello")...)
	res = protocol.TryDecodeFrame(frame, protocol.MaxPayloadBytes)
	if res.NeedMore || res.Error != nil || !bytes.Equal(res.Payload, []byte("hello")) || len(res.Remaining) != 0 {
		t.Errorf("single frame decode failed: %+v", res)
	}

	// Decode need more
	res = protocol.TryDecodeFrame([]byte{0x05, 0x68, 0x65}, protocol.MaxPayloadBytes)
	if !res.NeedMore {
		t.Error("expected need_more")
	}

	// Decode payload too large
	res = protocol.TryDecodeFrame(append(protocol.EncodeLengthPrefixU32(100), make([]byte, 100)...), 50)
	if res.Error != protocol.ErrPayloadTooLarge {
		t.Error("expected payload too large")
	}

	// Decode invalid length prefix (6th byte continues)
	res = protocol.TryDecodeFrame([]byte{0xff, 0xff, 0xff, 0xff, 0x80}, protocol.MaxPayloadBytes)
	if res.Error != protocol.ErrInvalidLengthPrefix {
		t.Error("expected invalid length prefix")
	}

	// Decode two frames
	frame1 := append(protocol.EncodeLengthPrefixU32(3), []byte("abc")...)
	frame2 := append(protocol.EncodeLengthPrefixU32(2), []byte("de")...)
	both := append(frame1, frame2...)
	res = protocol.TryDecodeFrame(both, protocol.MaxPayloadBytes)
	if !bytes.Equal(res.Payload, []byte("abc")) {
		t.Error("first frame decode failed")
	}
	res = protocol.TryDecodeFrame(res.Remaining, protocol.MaxPayloadBytes)
	if !bytes.Equal(res.Payload, []byte("de")) {
		t.Error("second frame decode failed")
	}
}

func TestCommandsRoundTrip(t *testing.T) {
	// Ping
	w := protocol.NewBinaryWriter()
	cmd := protocol.ClientCommand{Type: protocol.ClientCmdPing}
	protocol.EncodeClientCommand(w, cmd)
	r := protocol.NewBinaryReader(w.Bytes())
	decoded := protocol.DecodeClientCommand(r)
	if decoded.Type != protocol.ClientCmdPing {
		t.Error("Ping round-trip failed")
	}

	// Authenticate
	w = protocol.NewBinaryWriter()
	cmd = protocol.ClientCommand{Type: protocol.ClientCmdAuthenticate, Token: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	protocol.EncodeClientCommand(w, cmd)
	r = protocol.NewBinaryReader(w.Bytes())
	decoded = protocol.DecodeClientCommand(r)
	if decoded.Type != protocol.ClientCmdAuthenticate || decoded.Token != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Error("Authenticate round-trip failed")
	}

	// Chat
	w = protocol.NewBinaryWriter()
	cmd = protocol.ClientCommand{Type: protocol.ClientCmdChat, Message: "hello world"}
	protocol.EncodeClientCommand(w, cmd)
	r = protocol.NewBinaryReader(w.Bytes())
	decoded = protocol.DecodeClientCommand(r)
	if decoded.Type != protocol.ClientCmdChat || decoded.Message != "hello world" {
		t.Error("Chat round-trip failed")
	}

	// Pong
	w = protocol.NewBinaryWriter()
	scmd := protocol.ServerCommand{Type: protocol.ServerCmdPong}
	protocol.EncodeServerCommand(w, scmd)
	r = protocol.NewBinaryReader(w.Bytes())
	sdecoded := protocol.DecodeServerCommand(r)
	if sdecoded.Type != protocol.ServerCmdPong {
		t.Error("Pong round-trip failed")
	}

	// ChangeHost
	w = protocol.NewBinaryWriter()
	scmd = protocol.ServerCommand{Type: protocol.ServerCmdChangeHost, IsHost: true}
	protocol.EncodeServerCommand(w, scmd)
	r = protocol.NewBinaryReader(w.Bytes())
	sdecoded = protocol.DecodeServerCommand(r)
	if sdecoded.Type != protocol.ServerCmdChangeHost || !sdecoded.IsHost {
		t.Error("ChangeHost round-trip failed")
	}

	// Message (Chat)
	w = protocol.NewBinaryWriter()
	msg := protocol.Message{Type: protocol.MessageChat, User: 42, Content: "test"}
	protocol.EncodeMessage(w, msg)
	r = protocol.NewBinaryReader(w.Bytes())
	mdecoded := protocol.DecodeMessage(r)
	if mdecoded.Type != protocol.MessageChat || mdecoded.User != 42 || mdecoded.Content != "test" {
		t.Errorf("Message round-trip failed: %+v", mdecoded)
	}
}
