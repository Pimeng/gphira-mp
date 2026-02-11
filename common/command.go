package common

import (
	"fmt"
	"math"
)

// CompactPos 紧凑位置表示（使用float16）
type CompactPos struct {
	X uint16 // f16 bits
	Y uint16 // f16 bits
}

// NewCompactPos 从float32创建CompactPos
func NewCompactPos(x, y float32) CompactPos {
	return CompactPos{
		X: float32ToF16Bits(x),
		Y: float32ToF16Bits(y),
	}
}

// XFloat 获取X的float32值
func (p CompactPos) XFloat() float32 {
	return f16BitsToFloat32(p.X)
}

// YFloat 获取Y的float32值
func (p CompactPos) YFloat() float32 {
	return f16BitsToFloat32(p.Y)
}

func (p *CompactPos) ReadBinary(r *BinaryReader) error {
	x, err := ReadUint16(r)
	if err != nil {
		return err
	}
	y, err := ReadUint16(r)
	if err != nil {
		return err
	}
	p.X = x
	p.Y = y
	return nil
}

func (p *CompactPos) WriteBinary(w *BinaryWriter) error {
	WriteUint16(w, p.X)
	WriteUint16(w, p.Y)
	return nil
}

// float32转float16 bits（简化实现）
func float32ToF16Bits(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := uint16((bits >> 31) & 0x1)
	exp := int16((bits>>23)&0xFF) - 127 + 15
	frac := uint16((bits >> 13) & 0x3FF)

	if exp <= 0 {
		return sign << 15
	}
	if exp >= 31 {
		return (sign << 15) | 0x7C00
	}
	return (sign << 15) | (uint16(exp) << 10) | frac
}

// float16 bits转float32
func f16BitsToFloat32(bits uint16) float32 {
	sign := uint32((bits >> 15) & 0x1)
	exp := int16((bits >> 10) & 0x1F)
	frac := uint32(bits & 0x3FF)

	if exp == 0 {
		if frac == 0 {
			return math.Float32frombits(sign << 31)
		}
		exp = -14
		for frac&0x400 == 0 {
			frac <<= 1
			exp--
		}
		frac &= 0x3FF
	} else if exp == 31 {
		if frac == 0 {
			return math.Float32frombits((sign << 31) | 0x7F800000)
		}
		return math.Float32frombits((sign << 31) | 0x7FC00000)
	} else {
		exp = exp - 15 + 127
	}

	result := (sign << 31) | (uint32(exp) << 23) | (frac << 13)
	return math.Float32frombits(result)
}

// Varchar 变长字符串（带最大长度限制）
type Varchar struct {
	MaxLen int
	Value  string
}

func NewVarchar(maxLen int, value string) (Varchar, error) {
	if len(value) > maxLen {
		return Varchar{}, fmt.Errorf("string too long")
	}
	return Varchar{MaxLen: maxLen, Value: value}, nil
}

func (v *Varchar) ReadBinary(r *BinaryReader) error {
	length, err := r.Uleb()
	if err != nil {
		return err
	}
	if int(length) > v.MaxLen {
		return fmt.Errorf("string too long")
	}
	data, err := r.Take(int(length))
	if err != nil {
		return err
	}
	v.Value = string(data)
	return nil
}

func (v *Varchar) WriteBinary(w *BinaryWriter) error {
	WriteString(w, v.Value)
	return nil
}

// RoomId 房间ID
type RoomId struct {
	Value string
}

func NewRoomId(value string) (RoomId, error) {
	if value == "" {
		return RoomId{}, fmt.Errorf("invalid room id")
	}
	for _, c := range value {
		if c != '-' && c != '_' && !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return RoomId{}, fmt.Errorf("invalid room id")
		}
	}
	return RoomId{Value: value}, nil
}

func (r *RoomId) ReadBinary(reader *BinaryReader) error {
	v := Varchar{MaxLen: 20}
	if err := v.ReadBinary(reader); err != nil {
		return err
	}
	roomId, err := NewRoomId(v.Value)
	if err != nil {
		return err
	}
	*r = roomId
	return nil
}

func (r *RoomId) WriteBinary(w *BinaryWriter) error {
	v := Varchar{MaxLen: 20, Value: r.Value}
	return v.WriteBinary(w)
}

// TouchFrame 触摸帧
type TouchFrame struct {
	Time   float32
	Points []TouchPoint
}

type TouchPoint struct {
	ID  int8
	Pos CompactPos
}

func (t *TouchFrame) ReadBinary(r *BinaryReader) error {
	time, err := ReadFloat32(r)
	if err != nil {
		return err
	}
	t.Time = time

	length, err := r.Uleb()
	if err != nil {
		return err
	}
	t.Points = make([]TouchPoint, length)
	for i := uint64(0); i < length; i++ {
		id, err := ReadInt8(r)
		if err != nil {
			return err
		}
		pos := CompactPos{}
		if err := pos.ReadBinary(r); err != nil {
			return err
		}
		t.Points[i] = TouchPoint{ID: id, Pos: pos}
	}
	return nil
}

func (t *TouchFrame) WriteBinary(w *BinaryWriter) error {
	WriteFloat32(w, t.Time)
	w.Uleb(uint64(len(t.Points)))
	for _, p := range t.Points {
		WriteInt8(w, p.ID)
		p.Pos.WriteBinary(w)
	}
	return nil
}

// Judgement 判定类型
type Judgement uint8

const (
	JudgementPerfect Judgement = iota
	JudgementGood
	JudgementBad
	JudgementMiss
	JudgementHoldPerfect
	JudgementHoldGood
)

func (j *Judgement) ReadBinary(r *BinaryReader) error {
	v, err := ReadUint8(r)
	if err != nil {
		return err
	}
	*j = Judgement(v)
	return nil
}

func (j *Judgement) WriteBinary(w *BinaryWriter) error {
	WriteUint8(w, uint8(*j))
	return nil
}

// JudgeEvent 判定事件
type JudgeEvent struct {
	Time      float32
	LineID    uint32
	NoteID    uint32
	Judgement Judgement
}

func (j *JudgeEvent) ReadBinary(r *BinaryReader) error {
	time, err := ReadFloat32(r)
	if err != nil {
		return err
	}
	j.Time = time

	lineId, err := ReadUint32(r)
	if err != nil {
		return err
	}
	j.LineID = lineId

	noteId, err := ReadUint32(r)
	if err != nil {
		return err
	}
	j.NoteID = noteId

	judgement, err := ReadUint8(r)
	if err != nil {
		return err
	}
	j.Judgement = Judgement(judgement)
	return nil
}

func (j *JudgeEvent) WriteBinary(w *BinaryWriter) error {
	WriteFloat32(w, j.Time)
	WriteUint32(w, j.LineID)
	WriteUint32(w, j.NoteID)
	WriteUint8(w, uint8(j.Judgement))
	return nil
}

// ClientCommandType 客户端命令类型
type ClientCommandType uint8

const (
	ClientCmdPing ClientCommandType = iota
	ClientCmdAuthenticate
	ClientCmdChat
	ClientCmdTouches
	ClientCmdJudges
	ClientCmdCreateRoom
	ClientCmdJoinRoom
	ClientCmdLeaveRoom
	ClientCmdLockRoom
	ClientCmdCycleRoom
	ClientCmdSelectChart
	ClientCmdRequestStart
	ClientCmdReady
	ClientCmdCancelReady
	ClientCmdPlayed
	ClientCmdAbort
)

// ClientCommand 客户端命令
type ClientCommand struct {
	Type     ClientCommandType
	Token    string       // Authenticate
	Message  string       // Chat
	Frames   []TouchFrame // Touches
	Judges   []JudgeEvent // Judges
	RoomId   RoomId       // CreateRoom, JoinRoom
	Monitor  bool         // JoinRoom
	Lock     bool         // LockRoom
	Cycle    bool         // CycleRoom
	ChartID  int32        // SelectChart
	RecordID int32        // Played
}

func (c *ClientCommand) ReadBinary(r *BinaryReader) error {
	cmdType, err := ReadUint8(r)
	if err != nil {
		return err
	}
	c.Type = ClientCommandType(cmdType)

	switch c.Type {
	case ClientCmdPing:
		// 无数据
	case ClientCmdAuthenticate:
		v := Varchar{MaxLen: 32}
		if err := v.ReadBinary(r); err != nil {
			return err
		}
		c.Token = v.Value
	case ClientCmdChat:
		v := Varchar{MaxLen: 200}
		if err := v.ReadBinary(r); err != nil {
			return err
		}
		c.Message = v.Value
	case ClientCmdTouches:
		length, err := r.Uleb()
		if err != nil {
			return err
		}
		c.Frames = make([]TouchFrame, length)
		for i := uint64(0); i < length; i++ {
			if err := c.Frames[i].ReadBinary(r); err != nil {
				return err
			}
		}
	case ClientCmdJudges:
		length, err := r.Uleb()
		if err != nil {
			return err
		}
		c.Judges = make([]JudgeEvent, length)
		for i := uint64(0); i < length; i++ {
			if err := c.Judges[i].ReadBinary(r); err != nil {
				return err
			}
		}
	case ClientCmdCreateRoom:
		if err := c.RoomId.ReadBinary(r); err != nil {
			return err
		}
	case ClientCmdJoinRoom:
		if err := c.RoomId.ReadBinary(r); err != nil {
			return err
		}
		monitor, err := ReadBool(r)
		if err != nil {
			return err
		}
		c.Monitor = monitor
	case ClientCmdLeaveRoom:
		// 无数据
	case ClientCmdLockRoom:
		lock, err := ReadBool(r)
		if err != nil {
			return err
		}
		c.Lock = lock
	case ClientCmdCycleRoom:
		cycle, err := ReadBool(r)
		if err != nil {
			return err
		}
		c.Cycle = cycle
	case ClientCmdSelectChart:
		id, err := ReadInt32(r)
		if err != nil {
			return err
		}
		c.ChartID = id
	case ClientCmdRequestStart:
		// 无数据
	case ClientCmdReady:
		// 无数据
	case ClientCmdCancelReady:
		// 无数据
	case ClientCmdPlayed:
		id, err := ReadInt32(r)
		if err != nil {
			return err
		}
		c.RecordID = id
	case ClientCmdAbort:
		// 无数据
	default:
		return fmt.Errorf("unknown client command type: %d", c.Type)
	}
	return nil
}

func (c *ClientCommand) WriteBinary(w *BinaryWriter) error {
	WriteUint8(w, uint8(c.Type))

	switch c.Type {
	case ClientCmdPing:
		// 无数据
	case ClientCmdAuthenticate:
		v := Varchar{MaxLen: 32, Value: c.Token}
		v.WriteBinary(w)
	case ClientCmdChat:
		v := Varchar{MaxLen: 200, Value: c.Message}
		v.WriteBinary(w)
	case ClientCmdTouches:
		w.Uleb(uint64(len(c.Frames)))
		for _, f := range c.Frames {
			f.WriteBinary(w)
		}
	case ClientCmdJudges:
		w.Uleb(uint64(len(c.Judges)))
		for _, j := range c.Judges {
			j.WriteBinary(w)
		}
	case ClientCmdCreateRoom:
		c.RoomId.WriteBinary(w)
	case ClientCmdJoinRoom:
		c.RoomId.WriteBinary(w)
		WriteBool(w, c.Monitor)
	case ClientCmdLeaveRoom:
		// 无数据
	case ClientCmdLockRoom:
		WriteBool(w, c.Lock)
	case ClientCmdCycleRoom:
		WriteBool(w, c.Cycle)
	case ClientCmdSelectChart:
		WriteInt32(w, c.ChartID)
	case ClientCmdRequestStart:
		// 无数据
	case ClientCmdReady:
		// 无数据
	case ClientCmdCancelReady:
		// 无数据
	case ClientCmdPlayed:
		WriteInt32(w, c.RecordID)
	case ClientCmdAbort:
		// 无数据
	}
	return nil
}

// MessageType 消息类型
type MessageType uint8

const (
	MsgChat MessageType = iota
	MsgCreateRoom
	MsgJoinRoom
	MsgLeaveRoom
	MsgNewHost
	MsgSelectChart
	MsgGameStart
	MsgReady
	MsgCancelReady
	MsgCancelGame
	MsgStartPlaying
	MsgPlayed
	MsgGameEnd
	MsgAbort
	MsgLockRoom
	MsgCycleRoom
)

// Message 房间消息
type Message struct {
	Type      MessageType
	User      int32
	Content   string
	Name      string
	ChartID   int32
	Score     int32
	Accuracy  float32
	FullCombo bool
	Lock      bool
	Cycle     bool
}

func (m *Message) ReadBinary(r *BinaryReader) error {
	msgType, err := ReadUint8(r)
	if err != nil {
		return err
	}
	m.Type = MessageType(msgType)

	switch m.Type {
	case MsgChat:
		m.User, _ = ReadInt32(r)
		m.Content, _ = ReadString(r)
	case MsgCreateRoom:
		m.User, _ = ReadInt32(r)
	case MsgJoinRoom:
		m.User, _ = ReadInt32(r)
		m.Name, _ = ReadString(r)
	case MsgLeaveRoom:
		m.User, _ = ReadInt32(r)
		m.Name, _ = ReadString(r)
	case MsgNewHost:
		m.User, _ = ReadInt32(r)
	case MsgSelectChart:
		m.User, _ = ReadInt32(r)
		m.Name, _ = ReadString(r)
		m.ChartID, _ = ReadInt32(r)
	case MsgGameStart:
		m.User, _ = ReadInt32(r)
	case MsgReady:
		m.User, _ = ReadInt32(r)
	case MsgCancelReady:
		m.User, _ = ReadInt32(r)
	case MsgCancelGame:
		m.User, _ = ReadInt32(r)
	case MsgStartPlaying:
		// 无数据
	case MsgPlayed:
		m.User, _ = ReadInt32(r)
		m.Score, _ = ReadInt32(r)
		m.Accuracy, _ = ReadFloat32(r)
		m.FullCombo, _ = ReadBool(r)
	case MsgGameEnd:
		// 无数据
	case MsgAbort:
		m.User, _ = ReadInt32(r)
	case MsgLockRoom:
		m.Lock, _ = ReadBool(r)
	case MsgCycleRoom:
		m.Cycle, _ = ReadBool(r)
	}
	return nil
}

func (m *Message) WriteBinary(w *BinaryWriter) error {
	WriteUint8(w, uint8(m.Type))

	switch m.Type {
	case MsgChat:
		WriteInt32(w, m.User)
		WriteString(w, m.Content)
	case MsgCreateRoom:
		WriteInt32(w, m.User)
	case MsgJoinRoom:
		WriteInt32(w, m.User)
		WriteString(w, m.Name)
	case MsgLeaveRoom:
		WriteInt32(w, m.User)
		WriteString(w, m.Name)
	case MsgNewHost:
		WriteInt32(w, m.User)
	case MsgSelectChart:
		WriteInt32(w, m.User)
		WriteString(w, m.Name)
		WriteInt32(w, m.ChartID)
	case MsgGameStart:
		WriteInt32(w, m.User)
	case MsgReady:
		WriteInt32(w, m.User)
	case MsgCancelReady:
		WriteInt32(w, m.User)
	case MsgCancelGame:
		WriteInt32(w, m.User)
	case MsgStartPlaying:
		// 无数据
	case MsgPlayed:
		WriteInt32(w, m.User)
		WriteInt32(w, m.Score)
		WriteFloat32(w, m.Accuracy)
		WriteBool(w, m.FullCombo)
	case MsgGameEnd:
		// 无数据
	case MsgAbort:
		WriteInt32(w, m.User)
	case MsgLockRoom:
		WriteBool(w, m.Lock)
	case MsgCycleRoom:
		WriteBool(w, m.Cycle)
	}
	return nil
}

// RoomStateType 房间状态类型
type RoomStateType uint8

const (
	RoomStateSelectChart RoomStateType = iota
	RoomStateWaitingForReady
	RoomStatePlaying
)

// RoomState 房间状态
type RoomState struct {
	Type    RoomStateType
	ChartID *int32 // SelectChart时有效
}

func (rs *RoomState) ReadBinary(r *BinaryReader) error {
	stateType, err := ReadUint8(r)
	if err != nil {
		return err
	}
	rs.Type = RoomStateType(stateType)

	if rs.Type == RoomStateSelectChart {
		hasChart, _ := ReadBool(r)
		if hasChart {
			id, _ := ReadInt32(r)
			rs.ChartID = new(int32)
			*rs.ChartID = id
		}
	}
	return nil
}

func (rs *RoomState) WriteBinary(w *BinaryWriter) error {
	WriteUint8(w, uint8(rs.Type))

	if rs.Type == RoomStateSelectChart {
		if rs.ChartID != nil {
			WriteBool(w, true)
			WriteInt32(w, *rs.ChartID)
		} else {
			WriteBool(w, false)
		}
	}
	return nil
}

// UserInfo 用户信息
type UserInfo struct {
	ID      int32
	Name    string
	Monitor bool
}

func (u *UserInfo) ReadBinary(r *BinaryReader) error {
	id, err := ReadInt32(r)
	if err != nil {
		return err
	}
	u.ID = id

	name, err := ReadString(r)
	if err != nil {
		return err
	}
	u.Name = name

	monitor, err := ReadBool(r)
	if err != nil {
		return err
	}
	u.Monitor = monitor
	return nil
}

func (u *UserInfo) WriteBinary(w *BinaryWriter) error {
	WriteInt32(w, u.ID)
	WriteString(w, u.Name)
	WriteBool(w, u.Monitor)
	return nil
}

// ClientRoomState 客户端房间状态
type ClientRoomState struct {
	ID      RoomId
	State   RoomState
	Live    bool
	Locked  bool
	Cycle   bool
	IsHost  bool
	IsReady bool
	Users   map[int32]UserInfo
}

func (crs *ClientRoomState) ReadBinary(r *BinaryReader) error {
	if err := crs.ID.ReadBinary(r); err != nil {
		return err
	}
	if err := crs.State.ReadBinary(r); err != nil {
		return err
	}

	crs.Live, _ = ReadBool(r)
	crs.Locked, _ = ReadBool(r)
	crs.Cycle, _ = ReadBool(r)
	crs.IsHost, _ = ReadBool(r)
	crs.IsReady, _ = ReadBool(r)

	// 读取用户map
	length, _ := r.Uleb()
	crs.Users = make(map[int32]UserInfo)
	for i := uint64(0); i < length; i++ {
		key, _ := ReadInt32(r)
		var user UserInfo
		user.ReadBinary(r)
		crs.Users[key] = user
	}
	return nil
}

func (crs *ClientRoomState) WriteBinary(w *BinaryWriter) error {
	crs.ID.WriteBinary(w)
	crs.State.WriteBinary(w)
	WriteBool(w, crs.Live)
	WriteBool(w, crs.Locked)
	WriteBool(w, crs.Cycle)
	WriteBool(w, crs.IsHost)
	WriteBool(w, crs.IsReady)

	w.Uleb(uint64(len(crs.Users)))
	for k, v := range crs.Users {
		WriteInt32(w, k)
		v.WriteBinary(w)
	}
	return nil
}

// JoinRoomResponse 加入房间响应
type JoinRoomResponse struct {
	State RoomState
	Users []UserInfo
	Live  bool
}

func (jrr *JoinRoomResponse) ReadBinary(r *BinaryReader) error {
	if err := jrr.State.ReadBinary(r); err != nil {
		return err
	}

	length, _ := r.Uleb()
	jrr.Users = make([]UserInfo, length)
	for i := uint64(0); i < length; i++ {
		jrr.Users[i].ReadBinary(r)
	}

	jrr.Live, _ = ReadBool(r)
	return nil
}

func (jrr *JoinRoomResponse) WriteBinary(w *BinaryWriter) error {
	jrr.State.WriteBinary(w)
	w.Uleb(uint64(len(jrr.Users)))
	for _, u := range jrr.Users {
		u.WriteBinary(w)
	}
	WriteBool(w, jrr.Live)
	return nil
}

// ServerCommandType 服务器命令类型
type ServerCommandType uint8

const (
	ServerCmdPong ServerCommandType = iota
	ServerCmdAuthenticate
	ServerCmdChat
	ServerCmdTouches
	ServerCmdJudges
	ServerCmdMessage
	ServerCmdChangeState
	ServerCmdChangeHost
	ServerCmdCreateRoom
	ServerCmdJoinRoom
	ServerCmdOnJoinRoom
	ServerCmdLeaveRoom
	ServerCmdLockRoom
	ServerCmdCycleRoom
	ServerCmdSelectChart
	ServerCmdRequestStart
	ServerCmdReady
	ServerCmdCancelReady
	ServerCmdPlayed
	ServerCmdAbort
)

// ServerCommand 服务器命令
type ServerCommand struct {
	Type               ServerCommandType
	TouchesPlayer      int32
	TouchesFrames      []TouchFrame
	JudgesPlayer       int32
	JudgesEvents       []JudgeEvent
	Message            *Message
	ChangeState        *RoomState
	ChangeHost         bool
	OnJoinRoomUser     *UserInfo
	AuthenticateResult *Result[AuthResult]
	ChatResult         *Result[struct{}]
	CreateRoomResult   *Result[struct{}]
	JoinRoomResult     *Result[JoinRoomResponse]
	LeaveRoomResult    *Result[struct{}]
	LockRoomResult     *Result[struct{}]
	CycleRoomResult    *Result[struct{}]
	SelectChartResult  *Result[struct{}]
	RequestStartResult *Result[struct{}]
	ReadyResult        *Result[struct{}]
	CancelReadyResult  *Result[struct{}]
	PlayedResult       *Result[struct{}]
	AbortResult        *Result[struct{}]
}

// AuthResult 认证结果
type AuthResult struct {
	User UserInfo
	Room *ClientRoomState
}

func (ar *AuthResult) ReadBinary(r *BinaryReader) error {
	if err := ar.User.ReadBinary(r); err != nil {
		return err
	}
	hasRoom, _ := ReadBool(r)
	if hasRoom {
		ar.Room = &ClientRoomState{}
		ar.Room.ReadBinary(r)
	}
	return nil
}

func (ar *AuthResult) WriteBinary(w *BinaryWriter) error {
	ar.User.WriteBinary(w)
	if ar.Room != nil {
		WriteBool(w, true)
		ar.Room.WriteBinary(w)
	} else {
		WriteBool(w, false)
	}
	return nil
}

// Result 结果包装
type Result[T any] struct {
	Ok  *T
	Err *string
}

func (r *Result[T]) ReadBinary(reader *BinaryReader, readValue func(*BinaryReader) (T, error)) error {
	isOk, err := ReadBool(reader)
	if err != nil {
		return err
	}
	if isOk {
		v, err := readValue(reader)
		if err != nil {
			return err
		}
		r.Ok = &v
	} else {
		errStr, _ := ReadString(reader)
		r.Err = &errStr
	}
	return nil
}

func (r *Result[T]) WriteBinary(w *BinaryWriter, writeValue func(*BinaryWriter, T)) error {
	if r.Ok != nil {
		WriteBool(w, true)
		writeValue(w, *r.Ok)
	} else if r.Err != nil {
		WriteBool(w, false)
		WriteString(w, *r.Err)
	}
	return nil
}

func (sc *ServerCommand) ReadBinary(r *BinaryReader) error {
	cmdType, err := ReadUint8(r)
	if err != nil {
		return err
	}
	sc.Type = ServerCommandType(cmdType)

	switch sc.Type {
	case ServerCmdPong:
		// 无数据
	case ServerCmdAuthenticate:
		isOk, _ := ReadBool(r)
		sc.AuthenticateResult = &Result[AuthResult]{}
		if isOk {
			var v AuthResult
			v.ReadBinary(r)
			sc.AuthenticateResult.Ok = &v
		} else {
			errStr, _ := ReadString(r)
			sc.AuthenticateResult.Err = &errStr
		}
	case ServerCmdChat:
		isOk, _ := ReadBool(r)
		sc.ChatResult = &Result[struct{}]{}
		if isOk {
			sc.ChatResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.ChatResult.Err = &errStr
		}
	case ServerCmdTouches:
		sc.TouchesPlayer, _ = ReadInt32(r)
		length, _ := r.Uleb()
		sc.TouchesFrames = make([]TouchFrame, length)
		for i := uint64(0); i < length; i++ {
			sc.TouchesFrames[i].ReadBinary(r)
		}
	case ServerCmdJudges:
		sc.JudgesPlayer, _ = ReadInt32(r)
		length, _ := r.Uleb()
		sc.JudgesEvents = make([]JudgeEvent, length)
		for i := uint64(0); i < length; i++ {
			sc.JudgesEvents[i].ReadBinary(r)
		}
	case ServerCmdMessage:
		sc.Message = &Message{}
		sc.Message.ReadBinary(r)
	case ServerCmdChangeState:
		sc.ChangeState = &RoomState{}
		sc.ChangeState.ReadBinary(r)
	case ServerCmdChangeHost:
		sc.ChangeHost, _ = ReadBool(r)
	case ServerCmdCreateRoom:
		isOk, _ := ReadBool(r)
		sc.CreateRoomResult = &Result[struct{}]{}
		if isOk {
			sc.CreateRoomResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.CreateRoomResult.Err = &errStr
		}
	case ServerCmdJoinRoom:
		isOk, _ := ReadBool(r)
		sc.JoinRoomResult = &Result[JoinRoomResponse]{}
		if isOk {
			var v JoinRoomResponse
			v.ReadBinary(r)
			sc.JoinRoomResult.Ok = &v
		} else {
			errStr, _ := ReadString(r)
			sc.JoinRoomResult.Err = &errStr
		}
	case ServerCmdOnJoinRoom:
		sc.OnJoinRoomUser = &UserInfo{}
		sc.OnJoinRoomUser.ReadBinary(r)
	case ServerCmdLeaveRoom:
		isOk, _ := ReadBool(r)
		sc.LeaveRoomResult = &Result[struct{}]{}
		if isOk {
			sc.LeaveRoomResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.LeaveRoomResult.Err = &errStr
		}
	case ServerCmdLockRoom:
		isOk, _ := ReadBool(r)
		sc.LockRoomResult = &Result[struct{}]{}
		if isOk {
			sc.LockRoomResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.LockRoomResult.Err = &errStr
		}
	case ServerCmdCycleRoom:
		isOk, _ := ReadBool(r)
		sc.CycleRoomResult = &Result[struct{}]{}
		if isOk {
			sc.CycleRoomResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.CycleRoomResult.Err = &errStr
		}
	case ServerCmdSelectChart:
		isOk, _ := ReadBool(r)
		sc.SelectChartResult = &Result[struct{}]{}
		if isOk {
			sc.SelectChartResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.SelectChartResult.Err = &errStr
		}
	case ServerCmdRequestStart:
		isOk, _ := ReadBool(r)
		sc.RequestStartResult = &Result[struct{}]{}
		if isOk {
			sc.RequestStartResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.RequestStartResult.Err = &errStr
		}
	case ServerCmdReady:
		isOk, _ := ReadBool(r)
		sc.ReadyResult = &Result[struct{}]{}
		if isOk {
			sc.ReadyResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.ReadyResult.Err = &errStr
		}
	case ServerCmdCancelReady:
		isOk, _ := ReadBool(r)
		sc.CancelReadyResult = &Result[struct{}]{}
		if isOk {
			sc.CancelReadyResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.CancelReadyResult.Err = &errStr
		}
	case ServerCmdPlayed:
		isOk, _ := ReadBool(r)
		sc.PlayedResult = &Result[struct{}]{}
		if isOk {
			sc.PlayedResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.PlayedResult.Err = &errStr
		}
	case ServerCmdAbort:
		isOk, _ := ReadBool(r)
		sc.AbortResult = &Result[struct{}]{}
		if isOk {
			sc.AbortResult.Ok = &struct{}{}
		} else {
			errStr, _ := ReadString(r)
			sc.AbortResult.Err = &errStr
		}
	}
	return nil
}

func (sc *ServerCommand) WriteBinary(w *BinaryWriter) error {
	WriteUint8(w, uint8(sc.Type))

	switch sc.Type {
	case ServerCmdPong:
		// 无数据
	case ServerCmdAuthenticate:
		if sc.AuthenticateResult != nil {
			if sc.AuthenticateResult.Ok != nil {
				WriteBool(w, true)
				sc.AuthenticateResult.Ok.WriteBinary(w)
			} else if sc.AuthenticateResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.AuthenticateResult.Err)
			}
		}
	case ServerCmdChat:
		if sc.ChatResult != nil {
			if sc.ChatResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.ChatResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.ChatResult.Err)
			}
		}
	case ServerCmdTouches:
		WriteInt32(w, sc.TouchesPlayer)
		w.Uleb(uint64(len(sc.TouchesFrames)))
		for _, f := range sc.TouchesFrames {
			f.WriteBinary(w)
		}
	case ServerCmdJudges:
		WriteInt32(w, sc.JudgesPlayer)
		w.Uleb(uint64(len(sc.JudgesEvents)))
		for _, j := range sc.JudgesEvents {
			j.WriteBinary(w)
		}
	case ServerCmdMessage:
		sc.Message.WriteBinary(w)
	case ServerCmdChangeState:
		sc.ChangeState.WriteBinary(w)
	case ServerCmdChangeHost:
		WriteBool(w, sc.ChangeHost)
	case ServerCmdCreateRoom:
		if sc.CreateRoomResult != nil {
			if sc.CreateRoomResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.CreateRoomResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.CreateRoomResult.Err)
			}
		}
	case ServerCmdJoinRoom:
		if sc.JoinRoomResult != nil {
			if sc.JoinRoomResult.Ok != nil {
				WriteBool(w, true)
				sc.JoinRoomResult.Ok.WriteBinary(w)
			} else if sc.JoinRoomResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.JoinRoomResult.Err)
			}
		}
	case ServerCmdOnJoinRoom:
		sc.OnJoinRoomUser.WriteBinary(w)
	case ServerCmdLeaveRoom:
		if sc.LeaveRoomResult != nil {
			if sc.LeaveRoomResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.LeaveRoomResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.LeaveRoomResult.Err)
			}
		}
	case ServerCmdLockRoom:
		if sc.LockRoomResult != nil {
			if sc.LockRoomResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.LockRoomResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.LockRoomResult.Err)
			}
		}
	case ServerCmdCycleRoom:
		if sc.CycleRoomResult != nil {
			if sc.CycleRoomResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.CycleRoomResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.CycleRoomResult.Err)
			}
		}
	case ServerCmdSelectChart:
		if sc.SelectChartResult != nil {
			if sc.SelectChartResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.SelectChartResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.SelectChartResult.Err)
			}
		}
	case ServerCmdRequestStart:
		if sc.RequestStartResult != nil {
			if sc.RequestStartResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.RequestStartResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.RequestStartResult.Err)
			}
		}
	case ServerCmdReady:
		if sc.ReadyResult != nil {
			if sc.ReadyResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.ReadyResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.ReadyResult.Err)
			}
		}
	case ServerCmdCancelReady:
		if sc.CancelReadyResult != nil {
			if sc.CancelReadyResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.CancelReadyResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.CancelReadyResult.Err)
			}
		}
	case ServerCmdPlayed:
		if sc.PlayedResult != nil {
			if sc.PlayedResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.PlayedResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.PlayedResult.Err)
			}
		}
	case ServerCmdAbort:
		if sc.AbortResult != nil {
			if sc.AbortResult.Ok != nil {
				WriteBool(w, true)
			} else if sc.AbortResult.Err != nil {
				WriteBool(w, false)
				WriteString(w, *sc.AbortResult.Err)
			}
		}
	}
	return nil
}
