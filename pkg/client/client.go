// Package client provides a Phira MP TCP client with auto-reconnect and heartbeat.
package client

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
	"github.com/Pimeng/gphira-mp-next/pkg/stream"
)

const (
	heartbeatIntervalMs = 3000
	heartbeatTimeoutMs  = 2000
)

// LivePlayer holds touch frames and judge events for a player.
type LivePlayer struct {
	TouchFrames []protocol.TouchFrame
	JudgeEvents []protocol.JudgeEvent
}

// Options configures the client.
type Options struct {
	TimeoutMs              int
	AutoReconnect          bool
	MaxReconnectAttempts   int
	ReconnectBaseDelayMs   int
	OnReconnect            func()
	OnReconnectFailed      func()
	// Dialer overrides the default TCP dialer. Used for testing.
	Dialer                 func(host string, port int) (net.Conn, error)
}

func defaultOptions() *Options {
	return &Options{
		TimeoutMs:            7000,
		AutoReconnect:        false,
		MaxReconnectAttempts: 5,
		ReconnectBaseDelayMs: 1000,
	}
}

// rpcPending holds a pending RPC call.
type rpcPending struct {
	resolve func(any)
	reject  func(error)
	timer   *time.Timer
}

// Client is a Phira MP TCP client.
type Client struct {
	host   string
	port   int
	opts   *Options

	mu      sync.RWMutex
	strm    *stream.Stream[protocol.ClientCommand, protocol.ServerCommand]
	closed  bool

	// heartbeat
	pingTimer    *time.Timer
	pingFailCount int
	DelayMs      int

	// pong waiter
	pongMu       sync.Mutex
	pongWaiter   *rpcPending

	// rpc callbacks keyed by ServerCommandType (the reply type)
	rpcMu       sync.Mutex
	rpcCallbacks map[protocol.ServerCommandType]*rpcPending

	// state
	meValue     protocol.UserInfo
	hasMe       bool
	roomValue   *protocol.ClientRoomState
	messages    []protocol.Message
	livePlayers map[int32]*LivePlayer

	// reconnect state
	reconnectAttempts   int
	isReconnecting      bool
	reconnectTimer      *time.Timer
	lastToken           string
	lastRoomID          roomid.RoomID
	lastMonitor         bool
}

// Connect creates a new client and connects to the server.
func Connect(host string, port int, opts *Options) (*Client, error) {
	if opts == nil {
		opts = defaultOptions()
	} else {
		defaults := defaultOptions()
		if opts.TimeoutMs == 0 {
			opts.TimeoutMs = defaults.TimeoutMs
		}
		if opts.MaxReconnectAttempts == 0 {
			opts.MaxReconnectAttempts = defaults.MaxReconnectAttempts
		}
		if opts.ReconnectBaseDelayMs == 0 {
			opts.ReconnectBaseDelayMs = defaults.ReconnectBaseDelayMs
		}
	}
	c := &Client{
		host:         host,
		port:         port,
		opts:         opts,
		rpcCallbacks: make(map[protocol.ServerCommandType]*rpcPending),
		livePlayers:  make(map[int32]*LivePlayer),
	}
	if err := c.doConnect(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) doConnect() error {
	var conn net.Conn
	var err error
	if c.opts.Dialer != nil {
		conn, err = c.opts.Dialer(c.host, c.port)
	} else {
		addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))
		conn, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return err
	}

	codec := stream.Codec[protocol.ClientCommand, protocol.ServerCommand]{
		EncodeSend: func(cmd protocol.ClientCommand) []byte {
			w := protocol.NewBinaryWriter()
			protocol.EncodeClientCommand(w, cmd)
			return w.Bytes()
		},
		DecodeRecv: func(data []byte) (protocol.ServerCommand, error) {
			r := protocol.NewBinaryReader(data)
			return protocol.DecodeServerCommand(r), nil
		},
		IsHighPriority: func(cmd protocol.ClientCommand) bool {
			switch cmd.Type {
			case protocol.ClientCmdPing, protocol.ClientCmdAuthenticate,
				protocol.ClientCmdReady, protocol.ClientCmdCancelReady, protocol.ClientCmdAbort:
				return true
			}
			return false
		},
	}

	strm, err := stream.NewClient(conn, codec, c.onServerCommand, func(cmd protocol.ServerCommand) bool {
		return cmd.Type == protocol.ServerCmdPong
	}, func(phase string, err error) {
		// stream error, connection will be closed
	})
	if err != nil {
		conn.Close()
		return err
	}

	c.mu.Lock()
	c.strm = strm
	c.mu.Unlock()

	c.startHeartbeat()
	return nil
}

// Close cleanly closes the client.
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	strm := c.strm
	c.strm = nil
	c.mu.Unlock()

	c.stopHeartbeat()
	c.rejectAllPending(errors.New("client closed"))

	if c.reconnectTimer != nil {
		c.reconnectTimer.Stop()
		c.reconnectTimer = nil
	}

	if strm != nil {
		strm.Close()
	}
	return nil
}

// IsClosed returns true if the client has been closed.
func (c *Client) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// Me returns the authenticated user info.
func (c *Client) Me() (protocol.UserInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.meValue, c.hasMe
}

// Room returns the current room state.
func (c *Client) Room() *protocol.ClientRoomState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.roomValue == nil {
		return nil
	}
	cp := *c.roomValue
	cp.Users = make(map[int32]protocol.UserInfo, len(c.roomValue.Users))
	for k, v := range c.roomValue.Users {
		cp.Users[k] = v
	}
	return &cp
}

// RoomID returns the current room ID if in a room.
func (c *Client) RoomID() (roomid.RoomID, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.roomValue == nil {
		return "", false
	}
	return c.roomValue.ID, true
}

// IsHost returns whether the client is the room host.
func (c *Client) IsHost() (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.roomValue == nil {
		return false, false
	}
	return c.roomValue.IsHost, true
}

// IsReady returns whether the client is ready.
func (c *Client) IsReady() (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.roomValue == nil {
		return false, false
	}
	return c.roomValue.IsReady, true
}

// TakeMessages returns and clears accumulated messages.
func (c *Client) TakeMessages() []protocol.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.messages
	c.messages = nil
	return out
}

// LivePlayer returns the live player data for a given player ID.
func (c *Client) LivePlayer(playerID int32) *LivePlayer {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.livePlayers[playerID]; ok {
		return p
	}
	p := &LivePlayer{}
	c.livePlayers[playerID] = p
	return p
}

// ---------------------------------------------------------------------------
// RPC methods
// ---------------------------------------------------------------------------

// Ping sends a ping and returns the round-trip delay in milliseconds.
func (c *Client) Ping() (int, error) {
	start := time.Now()
	if err := c.send(protocol.ClientCommand{Type: protocol.ClientCmdPing}); err != nil {
		return 0, err
	}
	if err := c.waitPong(heartbeatTimeoutMs); err != nil {
		return 0, err
	}
	delay := int(time.Since(start).Milliseconds())
	c.DelayMs = delay
	return delay, nil
}

// Authenticate sends an authentication request.
func (c *Client) Authenticate(token string) error {
	c.lastToken = token
	res, err := c.rcall(protocol.ServerCmdAuthenticate, protocol.ClientCommand{Type: protocol.ClientCmdAuthenticate, Token: token})
	if err != nil {
		return err
	}
	if !res.AuthResult.Ok {
		return errors.New(res.AuthResult.Error)
	}
	c.mu.Lock()
	c.meValue = res.AuthResult.Value.Me
	c.hasMe = true
	if res.AuthResult.Value.Room != nil {
		c.roomValue = res.AuthResult.Value.Room
	}
	c.mu.Unlock()
	return nil
}

// Chat sends a chat message.
func (c *Client) Chat(message string) error {
	return c.rcallUnit(protocol.ServerCmdChat, protocol.ClientCommand{Type: protocol.ClientCmdChat, Message: message})
}

// CreateRoom creates a room with the given ID.
func (c *Client) CreateRoom(id string) error {
	roomID, err := roomid.Parse(id)
	if err != nil {
		return err
	}
	if err := c.rcallUnit(protocol.ServerCmdCreateRoom, protocol.ClientCommand{Type: protocol.ClientCmdCreateRoom, RoomID: roomID}); err != nil {
		return err
	}
	c.mu.Lock()
	me := c.meValue
	users := make(map[int32]protocol.UserInfo)
	users[me.ID] = me
	c.roomValue = &protocol.ClientRoomState{
		ID:      roomID,
		State:   protocol.RoomState{Type: protocol.RoomStateSelectChart},
		Users:   users,
		IsHost:  true,
		IsReady: false,
	}
	c.mu.Unlock()
	return nil
}

// JoinRoom joins a room.
func (c *Client) JoinRoom(id string, monitor bool) (protocol.JoinRoomResponse, error) {
	roomID, err := roomid.Parse(id)
	if err != nil {
		return protocol.JoinRoomResponse{}, err
	}
	c.lastRoomID = roomID
	c.lastMonitor = monitor
	res, err := c.rcall(protocol.ServerCmdJoinRoom, protocol.ClientCommand{Type: protocol.ClientCmdJoinRoom, RoomID: roomID, Monitor: monitor})
	if err != nil {
		return protocol.JoinRoomResponse{}, err
	}
	if !res.JoinResult.Ok {
		return protocol.JoinRoomResponse{}, errors.New(res.JoinResult.Error)
	}
	users := make(map[int32]protocol.UserInfo)
	for _, u := range res.JoinResult.Value.Users {
		users[u.ID] = u
	}
	c.mu.Lock()
	c.roomValue = &protocol.ClientRoomState{
		ID:      roomID,
		State:   res.JoinResult.Value.State,
		Live:    res.JoinResult.Value.Live,
		Users:   users,
		IsHost:  false,
		IsReady: false,
	}
	c.mu.Unlock()
	return res.JoinResult.Value, nil
}

// LeaveRoom leaves the current room.
func (c *Client) LeaveRoom() error {
	err := c.rcallUnit(protocol.ServerCmdLeaveRoom, protocol.ClientCommand{Type: protocol.ClientCmdLeaveRoom})
	c.mu.Lock()
	c.roomValue = nil
	c.lastRoomID = ""
	c.lastMonitor = false
	c.mu.Unlock()
	return err
}

// LockRoom locks or unlocks the room.
func (c *Client) LockRoom(lock bool) error {
	err := c.rcallUnit(protocol.ServerCmdLockRoom, protocol.ClientCommand{Type: protocol.ClientCmdLockRoom, Lock: lock})
	c.mu.Lock()
	if c.roomValue != nil {
		c.roomValue.Locked = lock
	}
	c.mu.Unlock()
	return err
}

// CycleRoom sets the room cycle mode.
func (c *Client) CycleRoom(cycle bool) error {
	err := c.rcallUnit(protocol.ServerCmdCycleRoom, protocol.ClientCommand{Type: protocol.ClientCmdCycleRoom, Cycle: cycle})
	c.mu.Lock()
	if c.roomValue != nil {
		c.roomValue.Cycle = cycle
	}
	c.mu.Unlock()
	return err
}

// SelectChart selects a chart.
func (c *Client) SelectChart(chartID int32) error {
	return c.rcallUnit(protocol.ServerCmdSelectChart, protocol.ClientCommand{Type: protocol.ClientCmdSelectChart, ChartID: chartID})
}

// RequestStart requests to start the game.
func (c *Client) RequestStart() error {
	return c.rcallUnit(protocol.ServerCmdRequestStart, protocol.ClientCommand{Type: protocol.ClientCmdRequestStart})
}

// Ready signals readiness.
func (c *Client) Ready() error {
	err := c.rcallUnit(protocol.ServerCmdReady, protocol.ClientCommand{Type: protocol.ClientCmdReady})
	c.mu.Lock()
	if c.roomValue != nil {
		c.roomValue.IsReady = true
	}
	c.mu.Unlock()
	return err
}

// CancelReady cancels readiness.
func (c *Client) CancelReady() error {
	err := c.rcallUnit(protocol.ServerCmdCancelReady, protocol.ClientCommand{Type: protocol.ClientCmdCancelReady})
	c.mu.Lock()
	if c.roomValue != nil {
		c.roomValue.IsReady = false
	}
	c.mu.Unlock()
	return err
}

// Played sends the played result.
func (c *Client) Played(recordID int32) error {
	return c.rcallUnit(protocol.ServerCmdPlayed, protocol.ClientCommand{Type: protocol.ClientCmdPlayed, RecordID: recordID})
}

// Abort sends an abort request.
func (c *Client) Abort() error {
	return c.rcallUnit(protocol.ServerCmdAbort, protocol.ClientCommand{Type: protocol.ClientCmdAbort})
}

// SendTouches sends touch frames.
func (c *Client) SendTouches(frames []protocol.TouchFrame) error {
	return c.send(protocol.ClientCommand{Type: protocol.ClientCmdTouches, Frames: frames})
}

// SendJudges sends judge events.
func (c *Client) SendJudges(judges []protocol.JudgeEvent) error {
	return c.send(protocol.ClientCommand{Type: protocol.ClientCmdJudges, Judges: judges})
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (c *Client) send(cmd protocol.ClientCommand) error {
	c.mu.RLock()
	strm := c.strm
	c.mu.RUnlock()
	if strm == nil {
		return errors.New("client not connected")
	}
	return strm.Send(cmd)
}

func (c *Client) rcall(replyType protocol.ServerCommandType, cmd protocol.ClientCommand) (protocol.ServerCommand, error) {
	if err := c.send(cmd); err != nil {
		return protocol.ServerCommand{}, err
	}

	c.rpcMu.Lock()
	if _, exists := c.rpcCallbacks[replyType]; exists {
		c.rpcMu.Unlock()
		return protocol.ServerCommand{}, errors.New("rpc already in flight")
	}

	ch := make(chan struct {
		res protocol.ServerCommand
		err error
	}, 1)

	timer := time.AfterFunc(time.Duration(c.opts.TimeoutMs)*time.Millisecond, func() {
		c.rpcMu.Lock()
		delete(c.rpcCallbacks, replyType)
		c.rpcMu.Unlock()
		ch <- struct {
			res protocol.ServerCommand
			err error
		}{err: errors.New("client timeout")}
	})

	c.rpcCallbacks[replyType] = &rpcPending{
		resolve: func(v any) {
			timer.Stop()
			ch <- struct {
				res protocol.ServerCommand
				err error
			}{res: v.(protocol.ServerCommand)}
		},
		reject: func(e error) {
			timer.Stop()
			ch <- struct {
				res protocol.ServerCommand
				err error
			}{err: e}
		},
		timer: timer,
	}
	c.rpcMu.Unlock()

	result := <-ch
	return result.res, result.err
}

func (c *Client) rcallUnit(replyType protocol.ServerCommandType, cmd protocol.ClientCommand) error {
	res, err := c.rcall(replyType, cmd)
	if err != nil {
		return err
	}
	if !res.Result.Ok {
		return errors.New(res.Result.Error)
	}
	return nil
}

func (c *Client) onServerCommand(cmd protocol.ServerCommand) error {
	switch cmd.Type {
	case protocol.ServerCmdPong:
		c.resolvePongWaiter()
		return nil
	case protocol.ServerCmdMessage:
		c.mu.Lock()
		c.messages = append(c.messages, cmd.Message)
		if c.roomValue != nil {
			switch cmd.Message.Type {
			case protocol.MessageLockRoom:
				c.roomValue.Locked = cmd.Message.Lock
			case protocol.MessageCycleRoom:
				c.roomValue.Cycle = cmd.Message.Cycle
			case protocol.MessageLeaveRoom:
				if c.hasMe && cmd.Message.User == c.meValue.ID {
					c.roomValue = nil
				}
			}
		}
		c.mu.Unlock()
		return nil
	case protocol.ServerCmdChangeState:
		c.mu.Lock()
		if c.roomValue != nil {
			c.roomValue.State = cmd.State
			if cmd.State.Type != protocol.RoomStateWaitingForReady {
				c.roomValue.IsReady = false
			}
		}
		c.mu.Unlock()
		return nil
	case protocol.ServerCmdChangeHost:
		c.mu.Lock()
		if c.roomValue != nil {
			c.roomValue.IsHost = cmd.IsHost
		}
		c.mu.Unlock()
		return nil
	case protocol.ServerCmdOnJoinRoom:
		c.mu.Lock()
		if c.roomValue != nil {
			c.roomValue.Users[cmd.UserInfo.ID] = cmd.UserInfo
		}
		c.mu.Unlock()
		return nil
	case protocol.ServerCmdTouches:
		p := c.LivePlayer(cmd.Player)
		p.TouchFrames = append(p.TouchFrames, cmd.Frames...)
		return nil
	case protocol.ServerCmdJudges:
		p := c.LivePlayer(cmd.Player)
		p.JudgeEvents = append(p.JudgeEvents, cmd.JudgeEvents...)
		return nil
	}

	// RPC replies
	c.rpcMu.Lock()
	pending, ok := c.rpcCallbacks[cmd.Type]
	if ok {
		delete(c.rpcCallbacks, cmd.Type)
	}
	c.rpcMu.Unlock()
	if ok {
		pending.resolve(cmd)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Heartbeat
// ---------------------------------------------------------------------------

func (c *Client) startHeartbeat() {
	var runHeartbeat func()
	runHeartbeat = func() {
		c.mu.RLock()
		strm := c.strm
		c.mu.RUnlock()
		if strm == nil {
			return
		}

		start := time.Now()
		if err := c.send(protocol.ClientCommand{Type: protocol.ClientCmdPing}); err != nil {
			c.pingFailCount++
		} else if err := c.waitPong(heartbeatTimeoutMs); err != nil {
			c.pingFailCount++
			if c.pingFailCount >= 3 {
				// Force disconnect to trigger reconnect
				c.mu.RLock()
				s := c.strm
				c.mu.RUnlock()
				if s != nil {
					s.Close()
				}
				return
			}
		} else {
			c.pingFailCount = 0
		}
		c.DelayMs = int(time.Since(start).Milliseconds())

		// Adaptive interval
		nextInterval := heartbeatIntervalMs
		if c.DelayMs < 100 {
			nextInterval = 5000
		} else if c.DelayMs > 300 {
			nextInterval = 1000
		}

		c.mu.Lock()
		if !c.closed {
			c.pingTimer = time.AfterFunc(time.Duration(nextInterval)*time.Millisecond, runHeartbeat)
		}
		c.mu.Unlock()
	}

	c.mu.Lock()
	if !c.closed {
		c.pingTimer = time.AfterFunc(time.Duration(heartbeatIntervalMs)*time.Millisecond, runHeartbeat)
	}
	c.mu.Unlock()
}

func (c *Client) stopHeartbeat() {
	c.mu.Lock()
	if c.pingTimer != nil {
		c.pingTimer.Stop()
		c.pingTimer = nil
	}
	c.mu.Unlock()
}

func (c *Client) waitPong(timeoutMs int) error {
	c.pongMu.Lock()
	if c.pongWaiter != nil {
		c.pongMu.Unlock()
		return errors.New("ping already in flight")
	}

	done := make(chan error, 1)
	timer := time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, func() {
		c.pongMu.Lock()
		c.pongWaiter = nil
		c.pongMu.Unlock()
		done <- errors.New("heartbeat timeout")
	})

	c.pongWaiter = &rpcPending{
		resolve: func(any) {
			timer.Stop()
			c.pongMu.Lock()
			c.pongWaiter = nil
			c.pongMu.Unlock()
			done <- nil
		},
		reject: func(e error) {
			timer.Stop()
			c.pongMu.Lock()
			c.pongWaiter = nil
			c.pongMu.Unlock()
			done <- e
		},
		timer: timer,
	}
	c.pongMu.Unlock()

	return <-done
}

func (c *Client) resolvePongWaiter() {
	c.pongMu.Lock()
	p := c.pongWaiter
	c.pongWaiter = nil
	c.pongMu.Unlock()
	if p != nil {
		p.resolve(nil)
	}
}

func (c *Client) rejectAllPending(e error) {
	c.pongMu.Lock()
	p := c.pongWaiter
	c.pongWaiter = nil
	c.pongMu.Unlock()
	if p != nil {
		p.reject(e)
	}

	c.rpcMu.Lock()
	callbacks := make(map[protocol.ServerCommandType]*rpcPending, len(c.rpcCallbacks))
	for k, v := range c.rpcCallbacks {
		callbacks[k] = v
	}
	c.rpcCallbacks = make(map[protocol.ServerCommandType]*rpcPending)
	c.rpcMu.Unlock()
	for _, p := range callbacks {
		p.reject(e)
	}
}

// ---------------------------------------------------------------------------
// Auto-reconnect
// ---------------------------------------------------------------------------

func (c *Client) scheduleReconnect() {
	if c.reconnectAttempts >= c.opts.MaxReconnectAttempts {
		c.isReconnecting = false
		if c.opts.OnReconnectFailed != nil {
			c.opts.OnReconnectFailed()
		}
		return
	}

	c.isReconnecting = true
	c.reconnectAttempts++

	// Exponential backoff with jitter
	delay := c.opts.ReconnectBaseDelayMs * (1 << (c.reconnectAttempts - 1))
	if delay > 30000 {
		delay = 30000
	}
	jitter := int(rand.Float64() * float64(delay) * 0.2)
	totalDelay := delay + jitter

	c.rejectAllPending(fmt.Errorf("reconnecting attempt %d", c.reconnectAttempts))

	c.mu.Lock()
	if !c.closed {
		c.reconnectTimer = time.AfterFunc(time.Duration(totalDelay)*time.Millisecond, func() {
			c.reconnectTimer = nil
			c.attemptReconnect()
		})
	}
	c.mu.Unlock()
}

func (c *Client) attemptReconnect() {
	if err := c.doConnect(); err != nil {
		c.scheduleReconnect()
		return
	}
	c.reconnectAttempts = 0
	c.isReconnecting = false

	// Restore authentication and room
	if c.lastToken != "" {
		_ = c.Authenticate(c.lastToken)
		if c.lastRoomID.String() != "" {
			_, _ = c.JoinRoom(c.lastRoomID.String(), c.lastMonitor)
		}
	}

	if c.opts.OnReconnect != nil {
		c.opts.OnReconnect()
	}
}
