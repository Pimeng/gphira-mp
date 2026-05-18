package test

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/client"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/stream"
)

// pipeMockConn is a net.Conn backed by two io.Pipe pairs for bidirectional communication.
type pipeMockConn struct {
	reader     io.ReadCloser
	writer     io.WriteCloser
	localAddr  string
	localPort  int
	closeOnce  sync.Once
	closed     bool
}

func newPipeMockConns() (*pipeMockConn, *pipeMockConn) {
	// client -> server
	c2sR, c2sW := io.Pipe()
	// server -> client
	s2cR, s2cW := io.Pipe()

	clientConn := &pipeMockConn{
		reader:    s2cR,
		writer:    c2sW,
		localAddr: "127.0.0.1",
		localPort: 12346,
	}
	serverConn := &pipeMockConn{
		reader:    c2sR,
		writer:    s2cW,
		localAddr: "127.0.0.1",
		localPort: 12346,
	}
	return clientConn, serverConn
}

func (p *pipeMockConn) Read(b []byte) (int, error)   { return p.reader.Read(b) }
func (p *pipeMockConn) Write(b []byte) (int, error)  { return p.writer.Write(b) }
func (p *pipeMockConn) Close() error {
	p.closeOnce.Do(func() {
		p.closed = true
		p.reader.Close()
		p.writer.Close()
	})
	return nil
}
func (p *pipeMockConn) LocalAddr() net.Addr          { return &net.TCPAddr{IP: net.ParseIP(p.localAddr), Port: p.localPort} }
func (p *pipeMockConn) RemoteAddr() net.Addr         { return &net.TCPAddr{IP: net.ParseIP(p.localAddr), Port: p.localPort} }
func (p *pipeMockConn) SetDeadline(t time.Time) error       { return nil }
func (p *pipeMockConn) SetReadDeadline(t time.Time) error   { return nil }
func (p *pipeMockConn) SetWriteDeadline(t time.Time) error  { return nil }

func setupClientTest(t *testing.T, handler func(protocol.ClientCommand) error) (*client.Client, *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]) {
	t.Helper()
	clientConn, serverConn := newPipeMockConns()

	serverReady := make(chan *stream.Stream[protocol.ServerCommand, protocol.ClientCommand], 1)
	go func() {
		var serverStream *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
		serverStream, _ = mockServerStream(serverConn, func(cmd protocol.ClientCommand) error {
			if handler != nil {
				return handler(cmd)
			}
			return nil
		})
		serverReady <- serverStream
	}()

	opts := &client.Options{Dialer: func(string, int) (net.Conn, error) { return clientConn, nil }}
	c, err := client.Connect(clientConn.localAddr, clientConn.localPort, opts)
	if err != nil {
		t.Fatalf("client connect failed: %v", err)
	}

	var serverStream *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	select {
	case serverStream = <-serverReady:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for server stream")
	}

	t.Cleanup(func() {
		c.Close()
		serverStream.Close()
	})

	return c, serverStream
}

// mockServerStream creates a server-side stream on the other end of a pipe.
func mockServerStream(conn net.Conn, handler func(protocol.ClientCommand) error) (*stream.Stream[protocol.ServerCommand, protocol.ClientCommand], error) {
	codec := stream.Codec[protocol.ServerCommand, protocol.ClientCommand]{
		EncodeSend: func(cmd protocol.ServerCommand) []byte {
			w := protocol.NewBinaryWriter()
			protocol.EncodeServerCommand(w, cmd)
			return w.Bytes()
		},
		DecodeRecv: func(data []byte) (protocol.ClientCommand, error) {
			r := protocol.NewBinaryReader(data)
			return protocol.DecodeClientCommand(r), nil
		},
		IsHighPriority: func(cmd protocol.ServerCommand) bool { return false },
	}
	return stream.New(conn, codec, handler, nil, nil)
}

func TestClientPing(t *testing.T) {
	serverReceived := make(chan protocol.ClientCommand, 1)
	var serverStreamRef *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	c, serverStream := setupClientTest(t, func(cmd protocol.ClientCommand) error {
		serverReceived <- cmd
		_ = serverStreamRef.Send(protocol.ServerCommand{Type: protocol.ServerCmdPong})
		return nil
	})
	serverStreamRef = serverStream
	_ = c

	delay, err := c.Ping()
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	if delay < 0 {
		t.Error("delay should be non-negative")
	}

	select {
	case cmd := <-serverReceived:
		if cmd.Type != protocol.ClientCmdPing {
			t.Errorf("expected Ping, got %d", cmd.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for server to receive ping")
	}
}

func TestClientAuthenticate(t *testing.T) {
	var serverStreamRef *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	c, serverStream := setupClientTest(t, func(cmd protocol.ClientCommand) error {
		if cmd.Type == protocol.ClientCmdAuthenticate {
			res := protocol.Ok(protocol.AuthenticateResult{
				Me: protocol.UserInfo{ID: 42, Name: "tester"},
			})
			_ = serverStreamRef.Send(protocol.ServerCommand{Type: protocol.ServerCmdAuthenticate, AuthResult: res})
		}
		return nil
	})
	serverStreamRef = serverStream

	if err := c.Authenticate("test-token"); err != nil {
		t.Fatalf("authenticate failed: %v", err)
	}

	me, ok := c.Me()
	if !ok || me.ID != 42 || me.Name != "tester" {
		t.Errorf("unexpected me: %+v, ok=%v", me, ok)
	}
}

func TestClientChat(t *testing.T) {
	var serverStreamRef *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	c, serverStream := setupClientTest(t, func(cmd protocol.ClientCommand) error {
		if cmd.Type == protocol.ClientCmdChat {
			_ = serverStreamRef.Send(protocol.ServerCommand{Type: protocol.ServerCmdChat, Result: protocol.Ok(struct{}{})})
		}
		return nil
	})
	serverStreamRef = serverStream

	if err := c.Chat("hello"); err != nil {
		t.Fatalf("chat failed: %v", err)
	}
}

func TestClientCreateRoom(t *testing.T) {
	var serverStreamRef *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	c, serverStream := setupClientTest(t, func(cmd protocol.ClientCommand) error {
		if cmd.Type == protocol.ClientCmdAuthenticate {
			res := protocol.Ok(protocol.AuthenticateResult{
				Me: protocol.UserInfo{ID: 1, Name: "tester"},
			})
			_ = serverStreamRef.Send(protocol.ServerCommand{Type: protocol.ServerCmdAuthenticate, AuthResult: res})
		}
		if cmd.Type == protocol.ClientCmdCreateRoom {
			_ = serverStreamRef.Send(protocol.ServerCommand{Type: protocol.ServerCmdCreateRoom, Result: protocol.Ok(struct{}{})})
		}
		return nil
	})
	serverStreamRef = serverStream

	c.Authenticate("token") // Set meValue for createRoom local state

	if err := c.CreateRoom("testroom"); err != nil {
		t.Fatalf("create room failed: %v", err)
	}

	room := c.Room()
	if room == nil {
		t.Fatal("expected room to be set")
	}
	if room.ID.String() != "testroom" {
		t.Errorf("room id = %s, want testroom", room.ID.String())
	}
	if !room.IsHost {
		t.Error("expected is_host = true")
	}
}

func TestClientJoinRoom(t *testing.T) {
	var serverStreamRef *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	c, serverStream := setupClientTest(t, func(cmd protocol.ClientCommand) error {
		if cmd.Type == protocol.ClientCmdJoinRoom {
			res := protocol.Ok(protocol.JoinRoomResponse{
				State: protocol.RoomState{Type: protocol.RoomStateSelectChart},
				Users: []protocol.UserInfo{{ID: 1, Name: "host"}},
				Live:  false,
			})
			_ = serverStreamRef.Send(protocol.ServerCommand{Type: protocol.ServerCmdJoinRoom, JoinResult: res})
		}
		return nil
	})
	serverStreamRef = serverStream

	resp, err := c.JoinRoom("testroom", false)
	if err != nil {
		t.Fatalf("join room failed: %v", err)
	}
	if resp.State.Type != protocol.RoomStateSelectChart {
		t.Errorf("unexpected state: %v", resp.State)
	}

	room := c.Room()
	if room == nil {
		t.Fatal("expected room")
	}
	if _, ok := room.Users[1]; !ok {
		t.Error("expected user 1 in room")
	}
}

func TestClientStateUpdates(t *testing.T) {
	var serverStreamRef *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	c, serverStream := setupClientTest(t, func(cmd protocol.ClientCommand) error {
		if cmd.Type == protocol.ClientCmdJoinRoom {
			res := protocol.Ok(protocol.JoinRoomResponse{
				State: protocol.RoomState{Type: protocol.RoomStateSelectChart},
				Users: []protocol.UserInfo{},
				Live:  false,
			})
			_ = serverStreamRef.Send(protocol.ServerCommand{Type: protocol.ServerCmdJoinRoom, JoinResult: res})
		}
		return nil
	})
	serverStreamRef = serverStream

	c.JoinRoom("testroom", false)

	// Simulate ChangeState
	_ = serverStream.Send(protocol.ServerCommand{
		Type:  protocol.ServerCmdChangeState,
		State: protocol.RoomState{Type: protocol.RoomStateWaitingForReady},
	})

	time.Sleep(50 * time.Millisecond)

	room := c.Room()
	if room == nil || room.State.Type != protocol.RoomStateWaitingForReady {
		t.Errorf("expected WaitingForReady state, got %+v", room)
	}

	// Simulate OnJoinRoom
	_ = serverStream.Send(protocol.ServerCommand{
		Type:     protocol.ServerCmdOnJoinRoom,
		UserInfo: protocol.UserInfo{ID: 99, Name: "newbie"},
	})

	time.Sleep(50 * time.Millisecond)

	room = c.Room()
	if room == nil {
		t.Fatal("expected room")
	}
	if _, ok := room.Users[99]; !ok {
		t.Error("expected user 99 in room")
	}
}

func TestClientMessages(t *testing.T) {
	c, serverStream := setupClientTest(t, nil)

	_ = serverStream.Send(protocol.ServerCommand{
		Type: protocol.ServerCmdMessage,
		Message: protocol.Message{
			Type:    protocol.MessageChat,
			User:    1,
			Content: "hi",
		},
	})

	time.Sleep(50 * time.Millisecond)

	msgs := c.TakeMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hi" {
		t.Errorf("message content = %s, want hi", msgs[0].Content)
	}

	msgs = c.TakeMessages()
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after take, got %d", len(msgs))
	}
}

func TestClientLivePlayer(t *testing.T) {
	c, serverStream := setupClientTest(t, nil)

	_ = serverStream.Send(protocol.ServerCommand{
		Type:   protocol.ServerCmdTouches,
		Player: 7,
		Frames: []protocol.TouchFrame{{Time: 1.0}},
	})

	time.Sleep(50 * time.Millisecond)

	p := c.LivePlayer(7)
	if len(p.TouchFrames) != 1 {
		t.Fatalf("expected 1 touch frame, got %d", len(p.TouchFrames))
	}

	_ = serverStream.Send(protocol.ServerCommand{
		Type:        protocol.ServerCmdJudges,
		Player:      7,
		JudgeEvents: []protocol.JudgeEvent{{Time: 2.0, Judgement: protocol.JudgementPerfect}},
	})

	time.Sleep(50 * time.Millisecond)

	if len(p.JudgeEvents) != 1 {
		t.Fatalf("expected 1 judge event, got %d", len(p.JudgeEvents))
	}
}

func TestClientClose(t *testing.T) {
	clientConn, serverConn := newPipeMockConns()

	serverReady := make(chan *stream.Stream[protocol.ServerCommand, protocol.ClientCommand], 1)
	go func() {
		serverStream, _ := mockServerStream(serverConn, nil)
		serverReady <- serverStream
	}()

	opts := &client.Options{Dialer: func(string, int) (net.Conn, error) { return clientConn, nil }}
	c, err := client.Connect(clientConn.localAddr, clientConn.localPort, opts)
	if err != nil {
		t.Fatalf("client connect failed: %v", err)
	}

	select {
	case serverStream := <-serverReady:
		defer serverStream.Close()
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for server stream")
	}

	if err := c.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !c.IsClosed() {
		t.Error("expected client to be closed")
	}
}

func TestClientRoomID(t *testing.T) {
	var serverStreamRef *stream.Stream[protocol.ServerCommand, protocol.ClientCommand]
	c, serverStream := setupClientTest(t, func(cmd protocol.ClientCommand) error {
		if cmd.Type == protocol.ClientCmdJoinRoom {
			res := protocol.Ok(protocol.JoinRoomResponse{
				State: protocol.RoomState{Type: protocol.RoomStateSelectChart},
				Users: []protocol.UserInfo{},
				Live:  false,
			})
			_ = serverStreamRef.Send(protocol.ServerCommand{Type: protocol.ServerCmdJoinRoom, JoinResult: res})
		}
		return nil
	})
	serverStreamRef = serverStream

	_, ok := c.RoomID()
	if ok {
		t.Error("expected no room id before join")
	}

	c.JoinRoom("testroom", false)

	id, ok := c.RoomID()
	if !ok || id.String() != "testroom" {
		t.Errorf("room id = %s, ok=%v", id.String(), ok)
	}
}
