package protocol

import (
	"sort"

	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	HeartbeatIntervalMs          = 3000
	HeartbeatTimeoutMs           = 2000
	HeartbeatDisconnectTimeoutMs = 10000
)

// ---------------------------------------------------------------------------
// Basic types
// ---------------------------------------------------------------------------

// CompactPos is a compact 2D position using float16.
type CompactPos struct {
	X float32
	Y float32
}

// TouchFrame represents a frame of touch input.
type TouchFrame struct {
	Time   float32
	Points []TouchPoint
}

// TouchPoint is a single touch point with an ID.
type TouchPoint struct {
	ID  int8
	Pos CompactPos
}

// Judgement is a rhythm game judgement result.
type Judgement uint8

const (
	JudgementPerfect    Judgement = 0
	JudgementGood       Judgement = 1
	JudgementBad        Judgement = 2
	JudgementMiss       Judgement = 3
	JudgementHoldPerfect Judgement = 4
	JudgementHoldGood   Judgement = 5
)

// JudgeEvent represents a single judgement event.
type JudgeEvent struct {
	Time      float32
	LineID    uint32
	NoteID    uint32
	Judgement Judgement
}

// UserInfo is basic user information.
type UserInfo struct {
	ID      int32
	Name    string
	Monitor bool
}

// RoomState represents the client-visible room state.
type RoomState struct {
	Type     RoomStateType
	ChartID  *int32 // only for SelectChart
}

type RoomStateType uint8

const (
	RoomStateSelectChart     RoomStateType = 0
	RoomStateWaitingForReady RoomStateType = 1
	RoomStatePlaying         RoomStateType = 2
)

// ClientRoomState is the full room state sent to a client.
type ClientRoomState struct {
	ID      roomid.RoomID
	State   RoomState
	Live    bool
	Locked  bool
	Cycle   bool
	IsHost  bool
	IsReady bool
	Users   map[int32]UserInfo
}

// JoinRoomResponse is sent when a user joins a room.
type JoinRoomResponse struct {
	State RoomState
	Users []UserInfo
	Live  bool
}

// ---------------------------------------------------------------------------
// Message (server broadcast)
// ---------------------------------------------------------------------------

type MessageType uint8

const (
	MessageChat         MessageType = 0
	MessageCreateRoom   MessageType = 1
	MessageJoinRoom     MessageType = 2
	MessageLeaveRoom    MessageType = 3
	MessageNewHost      MessageType = 4
	MessageSelectChart  MessageType = 5
	MessageGameStart    MessageType = 6
	MessageReady        MessageType = 7
	MessageCancelReady  MessageType = 8
	MessageCancelGame   MessageType = 9
	MessageStartPlaying MessageType = 10
	MessagePlayed       MessageType = 11
	MessageGameEnd      MessageType = 12
	MessageAbort        MessageType = 13
	MessageLockRoom     MessageType = 14
	MessageCycleRoom    MessageType = 15
)

type Message struct {
	Type MessageType

	// Common fields (not all are used for every type)
	User       int32
	Content    string // Chat
	Name       string // JoinRoom, LeaveRoom, SelectChart
	ChartID    int32  // SelectChart
	Score      int32  // Played
	Accuracy   float32 // Played
	FullCombo  bool    // Played
	Lock       bool    // LockRoom
	Cycle      bool    // CycleRoom
}

// ---------------------------------------------------------------------------
// ClientCommand
// ---------------------------------------------------------------------------

type ClientCommandType uint8

const (
	ClientCmdPing          ClientCommandType = 0
	ClientCmdAuthenticate  ClientCommandType = 1
	ClientCmdChat          ClientCommandType = 2
	ClientCmdTouches       ClientCommandType = 3
	ClientCmdJudges        ClientCommandType = 4
	ClientCmdCreateRoom    ClientCommandType = 5
	ClientCmdJoinRoom      ClientCommandType = 6
	ClientCmdLeaveRoom     ClientCommandType = 7
	ClientCmdLockRoom      ClientCommandType = 8
	ClientCmdCycleRoom     ClientCommandType = 9
	ClientCmdSelectChart   ClientCommandType = 10
	ClientCmdRequestStart  ClientCommandType = 11
	ClientCmdReady         ClientCommandType = 12
	ClientCmdCancelReady   ClientCommandType = 13
	ClientCmdPlayed        ClientCommandType = 14
	ClientCmdAbort         ClientCommandType = 15
)

type ClientCommand struct {
	Type ClientCommandType

	// Fields for specific commands
	Token   string       // Authenticate
	Message string       // Chat
	Frames  []TouchFrame // Touches
	Judges  []JudgeEvent // Judges
	RoomID  roomid.RoomID // CreateRoom, JoinRoom
	Monitor bool         // JoinRoom
	Lock    bool         // LockRoom
	Cycle   bool         // CycleRoom
	ChartID int32        // SelectChart
	RecordID int32       // Played
}

// ---------------------------------------------------------------------------
// ServerCommand
// ---------------------------------------------------------------------------

type ServerCommandType uint8

const (
	ServerCmdPong           ServerCommandType = 0
	ServerCmdAuthenticate   ServerCommandType = 1
	ServerCmdChat           ServerCommandType = 2
	ServerCmdTouches        ServerCommandType = 3
	ServerCmdJudges         ServerCommandType = 4
	ServerCmdMessage        ServerCommandType = 5
	ServerCmdChangeState    ServerCommandType = 6
	ServerCmdChangeHost     ServerCommandType = 7
	ServerCmdCreateRoom     ServerCommandType = 8
	ServerCmdJoinRoom       ServerCommandType = 9
	ServerCmdOnJoinRoom     ServerCommandType = 10
	ServerCmdLeaveRoom      ServerCommandType = 11
	ServerCmdLockRoom       ServerCommandType = 12
	ServerCmdCycleRoom      ServerCommandType = 13
	ServerCmdSelectChart    ServerCommandType = 14
	ServerCmdRequestStart   ServerCommandType = 15
	ServerCmdReady          ServerCommandType = 16
	ServerCmdCancelReady    ServerCommandType = 17
	ServerCmdPlayed         ServerCommandType = 18
	ServerCmdAbort          ServerCommandType = 19
)

type ServerCommand struct {
	Type ServerCommandType

	// Fields for specific commands
	Result          StringResult[struct{}]                    // Chat, CreateRoom, LeaveRoom, LockRoom, CycleRoom, SelectChart, RequestStart, Ready, CancelReady, Played, Abort
	AuthResult      StringResult[AuthenticateResult]          // Authenticate
	JoinResult      StringResult[JoinRoomResponse]            // JoinRoom
	Player          int32                                     // Touches, Judges
	Frames          []TouchFrame                              // Touches
	JudgeEvents     []JudgeEvent                              // Judges
	Message         Message                                   // Message
	State           RoomState                                 // ChangeState
	IsHost          bool                                      // ChangeHost
	UserInfo        UserInfo                                  // OnJoinRoom
}

type AuthenticateResult struct {
	Me   UserInfo
	Room *ClientRoomState
}

// ---------------------------------------------------------------------------
// Encode / Decode functions
// ---------------------------------------------------------------------------

func encodeCompactPos(w *BinaryWriter, v CompactPos) {
	w.WriteCompactPos(v)
}

func decodeCompactPos(r *BinaryReader) CompactPos {
	return r.ReadCompactPos()
}

func EncodeTouchFrame(w *BinaryWriter, v TouchFrame) {
	w.WriteF32(v.Time)
	WriteArray(w, v.Points, func(ww *BinaryWriter, p TouchPoint) {
		ww.WriteI8(p.ID)
		encodeCompactPos(ww, p.Pos)
	})
}

func DecodeTouchFrame(r *BinaryReader) TouchFrame {
	time := r.ReadF32()
	points := ReadArray(r, func(rr *BinaryReader) TouchPoint {
		id := rr.ReadI8()
		pos := decodeCompactPos(rr)
		return TouchPoint{ID: id, Pos: pos}
	})
	return TouchFrame{Time: time, Points: points}
}

func encodeJudgeEvent(w *BinaryWriter, v JudgeEvent) {
	w.WriteF32(v.Time)
	w.WriteU32(v.LineID)
	w.WriteU32(v.NoteID)
	w.WriteU8(uint8(v.Judgement))
}

func decodeJudgeEvent(r *BinaryReader) JudgeEvent {
	return JudgeEvent{
		Time:      r.ReadF32(),
		LineID:    r.ReadU32(),
		NoteID:    r.ReadU32(),
		Judgement: Judgement(r.ReadU8()),
	}
}

func encodeUserInfo(w *BinaryWriter, v UserInfo) {
	w.WriteI32(v.ID)
	w.WriteString(v.Name)
	w.WriteBool(v.Monitor)
}

func decodeUserInfo(r *BinaryReader) UserInfo {
	return UserInfo{
		ID:      r.ReadI32(),
		Name:    r.ReadString(),
		Monitor: r.ReadBool(),
	}
}

func EncodeRoomState(w *BinaryWriter, v RoomState) {
	w.WriteU8(uint8(v.Type))
	switch v.Type {
	case RoomStateSelectChart:
		WriteOption(w, v.ChartID, func(ww *BinaryWriter, id int32) {
			ww.WriteI32(id)
		})
	case RoomStateWaitingForReady:
	case RoomStatePlaying:
	}
}

func DecodeRoomState(r *BinaryReader) RoomState {
	tag := RoomStateType(r.ReadU8())
	switch tag {
	case RoomStateSelectChart:
		id := ReadOption(r, func(rr *BinaryReader) int32 {
			return rr.ReadI32()
		})
		return RoomState{Type: RoomStateSelectChart, ChartID: id}
	case RoomStateWaitingForReady:
		return RoomState{Type: RoomStateWaitingForReady}
	case RoomStatePlaying:
		return RoomState{Type: RoomStatePlaying}
	default:
		panic("proto-roomstate-tag-invalid")
	}
}

func EncodeClientRoomState(w *BinaryWriter, v ClientRoomState) {
	w.WriteString(v.ID.String())
	EncodeRoomState(w, v.State)
	w.WriteBool(v.Live)
	w.WriteBool(v.Locked)
	w.WriteBool(v.Cycle)
	w.WriteBool(v.IsHost)
	w.WriteBool(v.IsReady)

	// Map keys must be sorted ascending
	keys := make([]int32, 0, len(v.Users))
	for k := range v.Users {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	w.WriteUleb(uint64(len(keys)))
	for _, k := range keys {
		w.WriteI32(k)
		encodeUserInfo(w, v.Users[k])
	}
}

func DecodeClientRoomState(r *BinaryReader) ClientRoomState {
	idStr := r.ReadString()
	id, _ := roomid.Parse(idStr)
	state := DecodeRoomState(r)
	live := r.ReadBool()
	locked := r.ReadBool()
	cycle := r.ReadBool()
	isHost := r.ReadBool()
	isReady := r.ReadBool()
	users := ReadMap(r, func(rr *BinaryReader) int32 {
		return rr.ReadI32()
	}, decodeUserInfo)
	return ClientRoomState{
		ID:      id,
		State:   state,
		Live:    live,
		Locked:  locked,
		Cycle:   cycle,
		IsHost:  isHost,
		IsReady: isReady,
		Users:   users,
	}
}

func EncodeJoinRoomResponse(w *BinaryWriter, v JoinRoomResponse) {
	EncodeRoomState(w, v.State)
	WriteArray(w, v.Users, encodeUserInfo)
	w.WriteBool(v.Live)
}

func DecodeJoinRoomResponse(r *BinaryReader) JoinRoomResponse {
	state := DecodeRoomState(r)
	users := ReadArray(r, decodeUserInfo)
	live := r.ReadBool()
	return JoinRoomResponse{State: state, Users: users, Live: live}
}

func EncodeMessage(w *BinaryWriter, v Message) {
	w.WriteU8(uint8(v.Type))
	switch v.Type {
	case MessageChat:
		w.WriteI32(v.User)
		w.WriteString(v.Content)
	case MessageCreateRoom:
		w.WriteI32(v.User)
	case MessageJoinRoom:
		w.WriteI32(v.User)
		w.WriteString(v.Name)
	case MessageLeaveRoom:
		w.WriteI32(v.User)
		w.WriteString(v.Name)
	case MessageNewHost:
		w.WriteI32(v.User)
	case MessageSelectChart:
		w.WriteI32(v.User)
		w.WriteString(v.Name)
		w.WriteI32(v.ChartID)
	case MessageGameStart:
		w.WriteI32(v.User)
	case MessageReady:
		w.WriteI32(v.User)
	case MessageCancelReady:
		w.WriteI32(v.User)
	case MessageCancelGame:
		w.WriteI32(v.User)
	case MessageStartPlaying:
	case MessagePlayed:
		w.WriteI32(v.User)
		w.WriteI32(v.Score)
		w.WriteF32(v.Accuracy)
		w.WriteBool(v.FullCombo)
	case MessageGameEnd:
	case MessageAbort:
		w.WriteI32(v.User)
	case MessageLockRoom:
		w.WriteBool(v.Lock)
	case MessageCycleRoom:
		w.WriteBool(v.Cycle)
	}
}

func DecodeMessage(r *BinaryReader) Message {
	tag := MessageType(r.ReadU8())
	switch tag {
	case MessageChat:
		return Message{Type: MessageChat, User: r.ReadI32(), Content: r.ReadString()}
	case MessageCreateRoom:
		return Message{Type: MessageCreateRoom, User: r.ReadI32()}
	case MessageJoinRoom:
		return Message{Type: MessageJoinRoom, User: r.ReadI32(), Name: r.ReadString()}
	case MessageLeaveRoom:
		return Message{Type: MessageLeaveRoom, User: r.ReadI32(), Name: r.ReadString()}
	case MessageNewHost:
		return Message{Type: MessageNewHost, User: r.ReadI32()}
	case MessageSelectChart:
		return Message{Type: MessageSelectChart, User: r.ReadI32(), Name: r.ReadString(), ChartID: r.ReadI32()}
	case MessageGameStart:
		return Message{Type: MessageGameStart, User: r.ReadI32()}
	case MessageReady:
		return Message{Type: MessageReady, User: r.ReadI32()}
	case MessageCancelReady:
		return Message{Type: MessageCancelReady, User: r.ReadI32()}
	case MessageCancelGame:
		return Message{Type: MessageCancelGame, User: r.ReadI32()}
	case MessageStartPlaying:
		return Message{Type: MessageStartPlaying}
	case MessagePlayed:
		return Message{Type: MessagePlayed, User: r.ReadI32(), Score: r.ReadI32(), Accuracy: r.ReadF32(), FullCombo: r.ReadBool()}
	case MessageGameEnd:
		return Message{Type: MessageGameEnd}
	case MessageAbort:
		return Message{Type: MessageAbort, User: r.ReadI32()}
	case MessageLockRoom:
		return Message{Type: MessageLockRoom, Lock: r.ReadBool()}
	case MessageCycleRoom:
		return Message{Type: MessageCycleRoom, Cycle: r.ReadBool()}
	default:
		panic("proto-message-tag-invalid")
	}
}

func EncodeClientCommand(w *BinaryWriter, cmd ClientCommand) {
	w.WriteU8(uint8(cmd.Type))
	switch cmd.Type {
	case ClientCmdPing:
	case ClientCmdAuthenticate:
		w.WriteVarchar(32, cmd.Token)
	case ClientCmdChat:
		w.WriteVarchar(200, cmd.Message)
	case ClientCmdTouches:
		WriteArray(w, cmd.Frames, EncodeTouchFrame)
	case ClientCmdJudges:
		WriteArray(w, cmd.Judges, encodeJudgeEvent)
	case ClientCmdCreateRoom:
		w.WriteString(cmd.RoomID.String())
	case ClientCmdJoinRoom:
		w.WriteString(cmd.RoomID.String())
		w.WriteBool(cmd.Monitor)
	case ClientCmdLeaveRoom:
	case ClientCmdLockRoom:
		w.WriteBool(cmd.Lock)
	case ClientCmdCycleRoom:
		w.WriteBool(cmd.Cycle)
	case ClientCmdSelectChart:
		w.WriteI32(cmd.ChartID)
	case ClientCmdRequestStart:
	case ClientCmdReady:
	case ClientCmdCancelReady:
	case ClientCmdPlayed:
		w.WriteI32(cmd.RecordID)
	case ClientCmdAbort:
	}
}

func DecodeClientCommand(r *BinaryReader) ClientCommand {
	tag := ClientCommandType(r.ReadU8())
	switch tag {
	case ClientCmdPing:
		return ClientCommand{Type: ClientCmdPing}
	case ClientCmdAuthenticate:
		return ClientCommand{Type: ClientCmdAuthenticate, Token: r.ReadVarchar(32)}
	case ClientCmdChat:
		return ClientCommand{Type: ClientCmdChat, Message: r.ReadVarchar(200)}
	case ClientCmdTouches:
		return ClientCommand{Type: ClientCmdTouches, Frames: ReadArray(r, DecodeTouchFrame)}
	case ClientCmdJudges:
		return ClientCommand{Type: ClientCmdJudges, Judges: ReadArray(r, decodeJudgeEvent)}
	case ClientCmdCreateRoom:
		id, _ := roomid.Parse(r.ReadString())
		return ClientCommand{Type: ClientCmdCreateRoom, RoomID: id}
	case ClientCmdJoinRoom:
		id, _ := roomid.Parse(r.ReadString())
		return ClientCommand{Type: ClientCmdJoinRoom, RoomID: id, Monitor: r.ReadBool()}
	case ClientCmdLeaveRoom:
		return ClientCommand{Type: ClientCmdLeaveRoom}
	case ClientCmdLockRoom:
		return ClientCommand{Type: ClientCmdLockRoom, Lock: r.ReadBool()}
	case ClientCmdCycleRoom:
		return ClientCommand{Type: ClientCmdCycleRoom, Cycle: r.ReadBool()}
	case ClientCmdSelectChart:
		return ClientCommand{Type: ClientCmdSelectChart, ChartID: r.ReadI32()}
	case ClientCmdRequestStart:
		return ClientCommand{Type: ClientCmdRequestStart}
	case ClientCmdReady:
		return ClientCommand{Type: ClientCmdReady}
	case ClientCmdCancelReady:
		return ClientCommand{Type: ClientCmdCancelReady}
	case ClientCmdPlayed:
		return ClientCommand{Type: ClientCmdPlayed, RecordID: r.ReadI32()}
	case ClientCmdAbort:
		return ClientCommand{Type: ClientCmdAbort}
	default:
		panic("proto-clientcommand-tag-invalid")
	}
}

func EncodeServerCommand(w *BinaryWriter, cmd ServerCommand) {
	w.WriteU8(uint8(cmd.Type))
	switch cmd.Type {
	case ServerCmdPong:
	case ServerCmdAuthenticate:
		WriteResult(w, cmd.AuthResult, func(ww *BinaryWriter, v AuthenticateResult) {
			encodeUserInfo(ww, v.Me)
			WriteOption(ww, v.Room, EncodeClientRoomState)
		})
	case ServerCmdChat:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdTouches:
		w.WriteI32(cmd.Player)
		WriteArray(w, cmd.Frames, EncodeTouchFrame)
	case ServerCmdJudges:
		w.WriteI32(cmd.Player)
		WriteArray(w, cmd.JudgeEvents, encodeJudgeEvent)
	case ServerCmdMessage:
		EncodeMessage(w, cmd.Message)
	case ServerCmdChangeState:
		EncodeRoomState(w, cmd.State)
	case ServerCmdChangeHost:
		w.WriteBool(cmd.IsHost)
	case ServerCmdCreateRoom:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdJoinRoom:
		WriteResult(w, cmd.JoinResult, EncodeJoinRoomResponse)
	case ServerCmdOnJoinRoom:
		encodeUserInfo(w, cmd.UserInfo)
	case ServerCmdLeaveRoom:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdLockRoom:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdCycleRoom:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdSelectChart:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdRequestStart:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdReady:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdCancelReady:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdPlayed:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	case ServerCmdAbort:
		WriteResult(w, cmd.Result, func(ww *BinaryWriter, v struct{}) {})
	}
}

func DecodeServerCommand(r *BinaryReader) ServerCommand {
	tag := ServerCommandType(r.ReadU8())
	switch tag {
	case ServerCmdPong:
		return ServerCommand{Type: ServerCmdPong}
	case ServerCmdAuthenticate:
		res := ReadResult(r, func(rr *BinaryReader) AuthenticateResult {
			me := decodeUserInfo(rr)
			room := ReadOption(rr, DecodeClientRoomState)
			return AuthenticateResult{Me: me, Room: room}
		})
		return ServerCommand{Type: ServerCmdAuthenticate, AuthResult: res}
	case ServerCmdChat:
		return ServerCommand{Type: ServerCmdChat, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdTouches:
		return ServerCommand{Type: ServerCmdTouches, Player: r.ReadI32(), Frames: ReadArray(r, DecodeTouchFrame)}
	case ServerCmdJudges:
		return ServerCommand{Type: ServerCmdJudges, Player: r.ReadI32(), JudgeEvents: ReadArray(r, decodeJudgeEvent)}
	case ServerCmdMessage:
		return ServerCommand{Type: ServerCmdMessage, Message: DecodeMessage(r)}
	case ServerCmdChangeState:
		return ServerCommand{Type: ServerCmdChangeState, State: DecodeRoomState(r)}
	case ServerCmdChangeHost:
		return ServerCommand{Type: ServerCmdChangeHost, IsHost: r.ReadBool()}
	case ServerCmdCreateRoom:
		return ServerCommand{Type: ServerCmdCreateRoom, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdJoinRoom:
		return ServerCommand{Type: ServerCmdJoinRoom, JoinResult: ReadResult(r, DecodeJoinRoomResponse)}
	case ServerCmdOnJoinRoom:
		return ServerCommand{Type: ServerCmdOnJoinRoom, UserInfo: decodeUserInfo(r)}
	case ServerCmdLeaveRoom:
		return ServerCommand{Type: ServerCmdLeaveRoom, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdLockRoom:
		return ServerCommand{Type: ServerCmdLockRoom, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdCycleRoom:
		return ServerCommand{Type: ServerCmdCycleRoom, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdSelectChart:
		return ServerCommand{Type: ServerCmdSelectChart, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdRequestStart:
		return ServerCommand{Type: ServerCmdRequestStart, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdReady:
		return ServerCommand{Type: ServerCmdReady, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdCancelReady:
		return ServerCommand{Type: ServerCmdCancelReady, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdPlayed:
		return ServerCommand{Type: ServerCmdPlayed, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	case ServerCmdAbort:
		return ServerCommand{Type: ServerCmdAbort, Result: ReadResult(r, func(rr *BinaryReader) struct{} { return struct{}{} })}
	default:
		panic("proto-servercommand-tag-invalid")
	}
}
