package client

import (
	"fmt"
	"net"
	"sync"
	"time"

	"phira-mp/common"
)

const (
	Timeout = 7 * time.Second
)

// LivePlayer 实时玩家数据
type LivePlayer struct {
	TouchFrames []common.TouchFrame
	JudgeEvents []common.JudgeEvent
	mu          sync.Mutex
}

// Client Phira客户端
type Client struct {
	stream *common.ClientStream

	// 状态
	me   *common.UserInfo
	room *common.ClientRoomState
	mu   sync.RWMutex

	// 回调
	callbacks   map[uint16]chan interface{}
	callbackMu  sync.Mutex
	callbackSeq uint16

	// 消息队列
	messages []common.Message
	msgMu    sync.Mutex

	// 实时玩家
	livePlayers sync.Map // map[int32]*LivePlayer

	// 心跳
	pingFailCount int
	stopChan      chan struct{}
}

// NewClient 创建新客户端
func NewClient(address string) (*Client, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}

	// 使用版本 1
	stream, err := common.NewClientStream(conn, 1)
	if err != nil {
		conn.Close()
		return nil, err
	}

	client := &Client{
		stream:    stream,
		callbacks: make(map[uint16]chan interface{}),
		stopChan:  make(chan struct{}),
	}

	// 启动接收循环
	go client.recvLoop()
	// 启动心跳
	go client.pingLoop()

	return client, nil
}

// Close 关闭客户端
func (c *Client) Close() {
	close(c.stopChan)
	c.stream.Close()
}

// recvLoop 接收循环
func (c *Client) recvLoop() {
	for {
		select {
		case <-c.stopChan:
			return
		default:
		}

		cmd, err := c.stream.Recv()
		if err != nil {
			return
		}

		c.handleCommand(cmd)
	}
}

// pingLoop 心跳循环
func (c *Client) pingLoop() {
	ticker := time.NewTicker(common.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			if err := c.Ping(); err != nil {
				c.pingFailCount++
				if c.pingFailCount > 3 {
					// 心跳失败过多，断开连接
					c.Close()
					return
				}
			} else {
				c.pingFailCount = 0
			}
		}
	}
}

// handleCommand 处理服务器命令
func (c *Client) handleCommand(cmd common.ServerCommand) {
	switch cmd.Type {
	case common.ServerCmdPong:
		// 心跳响应，已在Ping中处理

	case common.ServerCmdAuthenticate:
		if cmd.AuthenticateResult != nil {
			if cmd.AuthenticateResult.Ok != nil {
				c.mu.Lock()
				c.me = &cmd.AuthenticateResult.Ok.User
				c.room = cmd.AuthenticateResult.Ok.Room
				c.mu.Unlock()
			}
			c.triggerCallback(0, cmd.AuthenticateResult)
		}

	case common.ServerCmdChat:
		if cmd.ChatResult != nil {
			c.triggerCallback(1, cmd.ChatResult)
		}

	case common.ServerCmdTouches:
		player := c.getLivePlayer(cmd.TouchesPlayer)
		player.mu.Lock()
		player.TouchFrames = append(player.TouchFrames, cmd.TouchesFrames...)
		player.mu.Unlock()

	case common.ServerCmdJudges:
		player := c.getLivePlayer(cmd.JudgesPlayer)
		player.mu.Lock()
		player.JudgeEvents = append(player.JudgeEvents, cmd.JudgesEvents...)
		player.mu.Unlock()

	case common.ServerCmdMessage:
		if cmd.Message != nil {
			c.msgMu.Lock()
			c.messages = append(c.messages, *cmd.Message)
			c.msgMu.Unlock()

			// 更新房间状态
			c.mu.Lock()
			if c.room != nil {
				switch cmd.Message.Type {
				case common.MsgLockRoom:
					c.room.Locked = cmd.Message.Lock
				case common.MsgCycleRoom:
					c.room.Cycle = cmd.Message.Cycle
				case common.MsgLeaveRoom:
					delete(c.room.Users, cmd.Message.User)
				}
			}
			c.mu.Unlock()
		}

	case common.ServerCmdChangeState:
		if cmd.ChangeState != nil {
			c.mu.Lock()
			if c.room != nil {
				c.room.State = *cmd.ChangeState
				c.room.IsReady = c.room.IsHost
			}
			c.mu.Unlock()
			// 清空实时玩家数据
			c.livePlayers = sync.Map{}
		}

	case common.ServerCmdChangeHost:
		c.mu.Lock()
		if c.room != nil {
			c.room.IsHost = cmd.ChangeHost
		}
		c.mu.Unlock()

	case common.ServerCmdCreateRoom:
		if cmd.CreateRoomResult != nil {
			c.triggerCallback(2, cmd.CreateRoomResult)
		}

	case common.ServerCmdJoinRoom:
		if cmd.JoinRoomResult != nil {
			if cmd.JoinRoomResult.Ok != nil {
				c.mu.Lock()
				users := make(map[int32]common.UserInfo)
				for _, u := range cmd.JoinRoomResult.Ok.Users {
					users[u.ID] = u
				}
				c.room = &common.ClientRoomState{
					State:   cmd.JoinRoomResult.Ok.State,
					Live:    cmd.JoinRoomResult.Ok.Live,
					Users:   users,
					IsHost:  false,
					IsReady: false,
				}
				c.mu.Unlock()
			}
			c.triggerCallback(3, cmd.JoinRoomResult)
		}

	case common.ServerCmdOnJoinRoom:
		if cmd.OnJoinRoomUser != nil {
			c.mu.Lock()
			if c.room != nil {
				c.room.Live = c.room.Live || cmd.OnJoinRoomUser.Monitor
				c.room.Users[cmd.OnJoinRoomUser.ID] = *cmd.OnJoinRoomUser
			}
			c.mu.Unlock()
		}

	case common.ServerCmdLeaveRoom:
		if cmd.LeaveRoomResult != nil {
			if cmd.LeaveRoomResult.Ok != nil {
				c.mu.Lock()
				c.room = nil
				c.mu.Unlock()
			}
			c.triggerCallback(4, cmd.LeaveRoomResult)
		}

	case common.ServerCmdLockRoom:
		if cmd.LockRoomResult != nil {
			c.triggerCallback(5, cmd.LockRoomResult)
		}

	case common.ServerCmdCycleRoom:
		if cmd.CycleRoomResult != nil {
			c.triggerCallback(6, cmd.CycleRoomResult)
		}

	case common.ServerCmdSelectChart:
		if cmd.SelectChartResult != nil {
			c.triggerCallback(7, cmd.SelectChartResult)
		}

	case common.ServerCmdRequestStart:
		if cmd.RequestStartResult != nil {
			if cmd.RequestStartResult.Ok != nil {
				c.mu.Lock()
				if c.room != nil {
					c.room.IsReady = true
				}
				c.mu.Unlock()
			}
			c.triggerCallback(8, cmd.RequestStartResult)
		}

	case common.ServerCmdReady:
		if cmd.ReadyResult != nil {
			if cmd.ReadyResult.Ok != nil {
				c.mu.Lock()
				if c.room != nil {
					c.room.IsReady = true
				}
				c.mu.Unlock()
			}
			c.triggerCallback(9, cmd.ReadyResult)
		}

	case common.ServerCmdCancelReady:
		if cmd.CancelReadyResult != nil {
			if cmd.CancelReadyResult.Ok != nil {
				c.mu.Lock()
				if c.room != nil {
					c.room.IsReady = false
				}
				c.mu.Unlock()
			}
			c.triggerCallback(10, cmd.CancelReadyResult)
		}

	case common.ServerCmdPlayed:
		if cmd.PlayedResult != nil {
			c.triggerCallback(11, cmd.PlayedResult)
		}

	case common.ServerCmdAbort:
		if cmd.AbortResult != nil {
			c.triggerCallback(12, cmd.AbortResult)
		}
	}
}

// getLivePlayer 获取实时玩家
func (c *Client) getLivePlayer(playerID int32) *LivePlayer {
	if val, ok := c.livePlayers.Load(playerID); ok {
		return val.(*LivePlayer)
	}
	player := &LivePlayer{}
	c.livePlayers.Store(playerID, player)
	return player
}

// registerCallback 注册回调
func (c *Client) registerCallback(cmdType uint16) (uint16, chan interface{}) {
	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()
	c.callbackSeq++
	seq := c.callbackSeq
	ch := make(chan interface{}, 1)
	c.callbacks[seq] = ch
	return seq, ch
}

// triggerCallback 触发回调
func (c *Client) triggerCallback(cmdType uint16, result interface{}) {
	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()
	for seq, ch := range c.callbacks {
		ch <- result
		delete(c.callbacks, seq)
		close(ch)
	}
}

// waitCallback 等待回调
func (c *Client) waitCallback(seq uint16, ch chan interface{}, timeout time.Duration) (interface{}, error) {
	select {
	case result := <-ch:
		c.callbackMu.Lock()
		delete(c.callbacks, seq)
		c.callbackMu.Unlock()
		return result, nil
	case <-time.After(timeout):
		c.callbackMu.Lock()
		delete(c.callbacks, seq)
		c.callbackMu.Unlock()
		return nil, fmt.Errorf("timeout")
	}
}

// Public API

// Me 获取当前用户信息
func (c *Client) Me() *common.UserInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.me == nil {
		return nil
	}
	info := *c.me
	return &info
}

// RoomState 获取房间状态
func (c *Client) RoomState() *common.ClientRoomState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.room == nil {
		return nil
	}
	state := *c.room
	return &state
}

// IsHost 是否是房主
func (c *Client) IsHost() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.room == nil {
		return false
	}
	return c.room.IsHost
}

// IsReady 是否已准备
func (c *Client) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.room == nil {
		return false
	}
	return c.room.IsReady
}

// TakeMessages 获取并清空消息队列
func (c *Client) TakeMessages() []common.Message {
	c.msgMu.Lock()
	defer c.msgMu.Unlock()
	msgs := c.messages
	c.messages = nil
	return msgs
}

// LivePlayer 获取实时玩家
func (c *Client) LivePlayer(playerID int32) *LivePlayer {
	return c.getLivePlayer(playerID)
}

// Ping 发送心跳
func (c *Client) Ping() error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdPing})
}

// Authenticate 认证
func (c *Client) Authenticate(token string) error {
	if err := c.stream.Send(common.ClientCommand{Type: common.ClientCmdAuthenticate, Token: token}); err != nil {
		return err
	}
	// 这里简化处理，实际应该使用回调机制
	time.Sleep(100 * time.Millisecond)
	return nil
}

// Chat 发送聊天消息
func (c *Client) Chat(message string) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdChat, Message: message})
}

// CreateRoom 创建房间
func (c *Client) CreateRoom(roomID common.RoomId) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdCreateRoom, RoomId: roomID})
}

// JoinRoom 加入房间
func (c *Client) JoinRoom(roomID common.RoomId, monitor bool) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdJoinRoom, RoomId: roomID, Monitor: monitor})
}

// LeaveRoom 离开房间
func (c *Client) LeaveRoom() error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdLeaveRoom})
}

// LockRoom 锁定/解锁房间
func (c *Client) LockRoom(lock bool) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdLockRoom, Lock: lock})
}

// CycleRoom 设置循环模式
func (c *Client) CycleRoom(cycle bool) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdCycleRoom, Cycle: cycle})
}

// SelectChart 选择谱面
func (c *Client) SelectChart(chartID int32) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdSelectChart, ChartID: chartID})
}

// RequestStart 请求开始游戏
func (c *Client) RequestStart() error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdRequestStart})
}

// Ready 准备
func (c *Client) Ready() error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdReady})
}

// CancelReady 取消准备
func (c *Client) CancelReady() error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdCancelReady})
}

// Played 上传成绩
func (c *Client) Played(recordID int32) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdPlayed, RecordID: recordID})
}

// Abort 放弃游戏
func (c *Client) Abort() error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdAbort})
}

// SendTouches 发送触摸数据
func (c *Client) SendTouches(frames []common.TouchFrame) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdTouches, Frames: frames})
}

// SendJudges 发送判定数据
func (c *Client) SendJudges(judges []common.JudgeEvent) error {
	return c.stream.Send(common.ClientCommand{Type: common.ClientCmdJudges, Judges: judges})
}
