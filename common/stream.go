package common

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const (
	HeartbeatInterval          = 3 * time.Second
	HeartbeatTimeout           = 2 * time.Second
	HeartbeatDisconnectTimeout = 10 * time.Second
)

// Stream 网络流
type Stream struct {
	conn    net.Conn
	version uint8

	sendChan chan []byte
	recvChan chan []byte

	stopChan chan struct{}
	wg       sync.WaitGroup

	mu       sync.RWMutex
	lastRecv time.Time
}

// NewStream 创建新的Stream（服务器端）- 读取客户端发送的版本号
func NewStream(conn net.Conn) (*Stream, error) {
	if err := conn.(*net.TCPConn).SetNoDelay(true); err != nil {
		return nil, err
	}

	// 读取客户端发送的版本号
	versionBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, versionBuf); err != nil {
		return nil, err
	}

	s := &Stream{
		conn:     conn,
		version:  versionBuf[0],
		sendChan: make(chan []byte, 1024),
		recvChan: make(chan []byte, 1024),
		stopChan: make(chan struct{}),
		lastRecv: time.Now(),
	}

	s.wg.Add(2)
	go s.sendLoop()
	go s.recvLoop()

	return s, nil
}

// NewStreamClient 客户端创建Stream - 发送版本号给服务器
func NewStreamClient(conn net.Conn, version uint8) (*Stream, error) {
	if err := conn.(*net.TCPConn).SetNoDelay(true); err != nil {
		return nil, err
	}

	// 发送版本号给服务器
	if _, err := conn.Write([]byte{version}); err != nil {
		return nil, err
	}

	s := &Stream{
		conn:     conn,
		version:  version,
		sendChan: make(chan []byte, 1024),
		recvChan: make(chan []byte, 1024),
		stopChan: make(chan struct{}),
		lastRecv: time.Now(),
	}

	s.wg.Add(2)
	go s.sendLoop()
	go s.recvLoop()

	return s, nil
}

// Version 获取版本号
func (s *Stream) Version() uint8 {
	return s.version
}

// SendRaw 发送原始数据
func (s *Stream) SendRaw(data []byte) error {
	select {
	case s.sendChan <- data:
		return nil
	case <-s.stopChan:
		return fmt.Errorf("stream closed")
	}
}

// RecvRaw 接收原始数据
func (s *Stream) RecvRaw() ([]byte, error) {
	select {
	case data := <-s.recvChan:
		return data, nil
	case <-s.stopChan:
		return nil, fmt.Errorf("stream closed")
	}
}

// Close 关闭Stream
func (s *Stream) Close() {
	close(s.stopChan)
	s.conn.Close()
	s.wg.Wait()
}

// LastRecvTime 获取最后接收时间
func (s *Stream) LastRecvTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastRecv
}

func (s *Stream) sendLoop() {
	defer s.wg.Done()

	for {
		select {
		case data := <-s.sendChan:
			if err := s.writeData(data); err != nil {
				return
			}
		case <-s.stopChan:
			return
		}
	}
}

func (s *Stream) recvLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopChan:
			return
		default:
		}

		data, err := s.readData()
		if err != nil {
			return
		}

		s.mu.Lock()
		s.lastRecv = time.Now()
		s.mu.Unlock()

		select {
		case s.recvChan <- data:
		case <-s.stopChan:
			return
		}
	}
}

func (s *Stream) writeData(data []byte) error {
	// 写入长度（ULEB128编码）
	lenBuf := make([]byte, 0, 5)
	x := uint32(len(data))
	for {
		b := byte(x & 0x7f)
		x >>= 7
		if x != 0 {
			b |= 0x80
		}
		lenBuf = append(lenBuf, b)
		if x == 0 {
			break
		}
	}

	if _, err := s.conn.Write(lenBuf); err != nil {
		return err
	}
	if _, err := s.conn.Write(data); err != nil {
		return err
	}
	return nil
}

func (s *Stream) readData() ([]byte, error) {
	// 读取长度（ULEB128编码）
	var length uint32
	var pos uint
	for {
		b := make([]byte, 1)
		if _, err := io.ReadFull(s.conn, b); err != nil {
			return nil, err
		}
		length |= uint32(b[0]&0x7f) << pos
		pos += 7
		if b[0]&0x80 == 0 {
			break
		}
		if pos > 32 {
			return nil, fmt.Errorf("invalid length")
		}
	}

	if length > 2*1024*1024 {
		return nil, fmt.Errorf("data packet too large: %d", length)
	}

	// 读取数据
	buffer := make([]byte, length)
	if _, err := io.ReadFull(s.conn, buffer); err != nil {
		return nil, err
	}

	return buffer, nil
}

// ServerStream 服务器端Stream包装
type ServerStream struct {
	*Stream
}

// NewServerStream 创建服务器端Stream
func NewServerStream(conn net.Conn) (*ServerStream, error) {
	stream, err := NewStream(conn)
	if err != nil {
		return nil, err
	}
	return &ServerStream{Stream: stream}, nil
}

// Send 发送服务器命令
func (s *ServerStream) Send(cmd ServerCommand) error {
	w := NewBinaryWriter()
	if err := cmd.WriteBinary(w); err != nil {
		return err
	}
	return s.SendRaw(w.Data())
}

// Recv 接收客户端命令
func (s *ServerStream) Recv() (ClientCommand, error) {
	data, err := s.RecvRaw()
	if err != nil {
		return ClientCommand{}, err
	}
	// 记录原始数据用于调试
	if len(data) > 0 {
		fmt.Printf("[DEBUG] Received raw data (len=%d): %v\n", len(data), data[:min(len(data), 20)])
	}
	r := NewBinaryReader(data)
	var cmd ClientCommand
	if err := cmd.ReadBinary(r); err != nil {
		return ClientCommand{}, fmt.Errorf("failed to decode command: %w, raw data: %v", err, data[:min(len(data), 20)])
	}
	return cmd, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ClientStream 客户端Stream包装
type ClientStream struct {
	*Stream
}

// NewClientStream 创建客户端Stream
func NewClientStream(conn net.Conn, version uint8) (*ClientStream, error) {
	stream, err := NewStreamClient(conn, version)
	if err != nil {
		return nil, err
	}
	return &ClientStream{Stream: stream}, nil
}

// Send 发送客户端命令
func (c *ClientStream) Send(cmd ClientCommand) error {
	w := NewBinaryWriter()
	if err := cmd.WriteBinary(w); err != nil {
		return err
	}
	return c.SendRaw(w.Data())
}

// Recv 接收服务器命令
func (c *ClientStream) Recv() (ServerCommand, error) {
	data, err := c.RecvRaw()
	if err != nil {
		return ServerCommand{}, err
	}
	r := NewBinaryReader(data)
	var cmd ServerCommand
	if err := cmd.ReadBinary(r); err != nil {
		return ServerCommand{}, err
	}
	return cmd, nil
}
