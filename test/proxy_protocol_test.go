package test

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/network"
)

// mockConn is a simple net.Conn implementation for testing.
type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
	closed   bool
}

func newMockConn(data []byte) *mockConn {
	return &mockConn{readBuf: bytes.NewBuffer(data), writeBuf: &bytes.Buffer{}}
}

func (m *mockConn) Read(b []byte) (int, error)   { return m.readBuf.Read(b) }
func (m *mockConn) Write(b []byte) (int, error)  { return m.writeBuf.Write(b) }
func (m *mockConn) Close() error                 { m.closed = true; return nil }
func (m *mockConn) LocalAddr() net.Addr          { return &net.TCPAddr{} }
func (m *mockConn) RemoteAddr() net.Addr         { return &net.TCPAddr{} }
func (m *mockConn) SetDeadline(t time.Time) error       { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error   { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error  { return nil }

func TestParseProxyProtocolV1(t *testing.T) {
	data := []byte("PROXY TCP4 192.168.0.1 192.168.0.11 56324 443\r\n")
	conn := newMockConn(data)

	info, wrappedConn, err := network.ParseProxyProtocol(conn, time.Second)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected proxy info")
	}
	if info.SourceAddress != "192.168.0.1" {
		t.Errorf("source = %q, want 192.168.0.1", info.SourceAddress)
	}
	if info.SourcePort != 56324 {
		t.Errorf("source port = %d, want 56324", info.SourcePort)
	}
	if info.Family != "TCP4" {
		t.Errorf("family = %q, want TCP4", info.Family)
	}

	// Wrapped conn should be usable
	buf := make([]byte, 1)
	_, err = wrappedConn.Read(buf)
	// No remaining data, so EOF
	if err == nil {
		t.Error("expected EOF on empty remaining data")
	}
}

func TestParseProxyProtocolV1Unknown(t *testing.T) {
	data := []byte("PROXY UNKNOWN\r\n")
	conn := newMockConn(data)

	info, _, err := network.ParseProxyProtocol(conn, time.Second)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if info != nil {
		t.Error("expected nil for UNKNOWN")
	}
}

func TestParseProxyProtocolNotProxy(t *testing.T) {
	data := []byte("GET / HTTP/1.1\r\n")
	conn := newMockConn(data)

	info, wrappedConn, err := network.ParseProxyProtocol(conn, time.Second)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if info != nil {
		t.Error("expected nil for non-proxy data")
	}

	// Remaining data should be preserved
	buf := make([]byte, 14)
	n, err := wrappedConn.Read(buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(buf[:n]) != "GET / HTTP/1.1" {
		t.Errorf("remaining data = %q", string(buf[:n]))
	}
}

func TestParseProxyProtocolV1IPv6(t *testing.T) {
	data := []byte("PROXY TCP6 ::1 ::1 56324 443\r\n")
	conn := newMockConn(data)

	info, _, err := network.ParseProxyProtocol(conn, time.Second)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected proxy info")
	}
	if info.SourceAddress != "::1" {
		t.Errorf("source = %q, want ::1", info.SourceAddress)
	}
	if info.Family != "TCP6" {
		t.Errorf("family = %q, want TCP6", info.Family)
	}
}

func TestParseProxyProtocolV2TCP4(t *testing.T) {
	// V2 header: signature(12) + ver_cmd(1) + fam_proto(1) + len(2) + addr(12)
	header := []byte{
		0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a, // signature
		0x21, // version 2, command PROXY
		0x11, // family IPv4, protocol TCP
		0x00, 0x0c, // address length = 12
		192, 168, 0, 1, // source address
		192, 168, 0, 11, // dest address
		0xdc, 0x04, // source port = 56324
		0x01, 0xbb, // dest port = 443
	}
	conn := newMockConn(header)

	info, _, err := network.ParseProxyProtocol(conn, time.Second)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected proxy info")
	}
	if info.SourceAddress != "192.168.0.1" {
		t.Errorf("source = %q, want 192.168.0.1", info.SourceAddress)
	}
	if info.SourcePort != 56324 {
		t.Errorf("source port = %d, want 56324", info.SourcePort)
	}
	if info.Family != "TCP4" {
		t.Errorf("family = %q, want TCP4", info.Family)
	}
}
