package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/state"
	"github.com/Pimeng/gphira-mp-next/internal/utils"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

// CLI provides an interactive command-line interface for server management.
type CLI struct {
	state        *state.ServerState
	logger       *utils.Logger
	broadcastAll func(protocol.ServerCommand) error
	stopServer   func() error
	kickUserFn   func(int32) bool

	scanner *bufio.Scanner
	stop    chan struct{}
	wg      sync.WaitGroup
}

// NewCLI creates a new CLI instance.
func NewCLI(state *state.ServerState, logger *utils.Logger, broadcastAll func(protocol.ServerCommand) error, stopServer func() error, kickUser func(int32) bool) *CLI {
	return &CLI{
		state:        state,
		logger:       logger,
		broadcastAll: broadcastAll,
		stopServer:   stopServer,
		kickUserFn:   kickUser,
		scanner:      bufio.NewScanner(os.Stdin),
		stop:         make(chan struct{}),
	}
}

// Start begins reading commands from stdin.
func (c *CLI) Start() {
	c.wg.Add(1)
	go c.run()
}

func (c *CLI) run() {
	defer c.wg.Done()
	fmt.Println(c.state.ServerLang.Format("cli-welcome", nil))
	for {
		select {
		case <-c.stop:
			return
		default:
		}

		fmt.Print("> ")
		if !c.scanner.Scan() {
			return
		}

		line := strings.TrimSpace(c.scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])
		args := parts[1:]

		switch cmd {
		case "help", "h":
			c.printHelp()
		case "list", "rooms":
			c.listRooms()
		case "users":
			c.listUsers()
		case "kick":
			c.cmdKickUser(args)
		case "ban":
			c.banUser(args)
		case "unban":
			c.unbanUser(args)
		case "banlist":
			c.banList()
		case "broadcast", "say":
			c.broadcast(args)
		case "contest":
			c.handleContest(args)
		case "stop", "exit", "quit":
			fmt.Println("Stopping server...")
			if c.stopServer != nil {
				_ = c.stopServer()
			}
			return
		default:
			fmt.Printf("Unknown command: %s. Type 'help' for available commands.\n", cmd)
		}
	}
}

// Stop signals the CLI to stop reading input.
func (c *CLI) Stop() {
	close(c.stop)
	c.wg.Wait()
}

func (c *CLI) printHelp() {
	fmt.Println(`Available commands:
  help, h                  Show this help message
  list, rooms              List all rooms
  users                    List all online users
  kick <id>                Kick a user by ID
  ban <id>                 Ban a user by ID
  unban <id>               Unban a user by ID
  banlist                  List banned users
  broadcast <msg>          Broadcast a message to all users
  contest <room> enable    Enable contest mode for a room
  contest <room> disable   Disable contest mode for a room
  contest <room> whitelist <id>...
                           Update contest whitelist
  contest <room> start [force]
                           Manually start a contest game
  stop, exit, quit         Stop the server`)
}

func (c *CLI) listRooms() {
	c.state.WithRLock(func() {
		if len(c.state.Rooms) == 0 {
			fmt.Println("No active rooms.")
			return
		}
		fmt.Printf("%-20s %-10s %-8s %-8s %s\n", "Room ID", "Host", "Users", "Contest", "State")
		fmt.Println(strings.Repeat("-", 70))
		for rid, room := range c.state.Rooms {
			hostName := ""
			if u := c.state.Users[room.HostID]; u != nil {
				hostName = u.Name
			}
			stateStr := "select"
			switch room.State.(type) {
			case interface{ String() string }:
				stateStr = fmt.Sprintf("%T", room.State)
			}
			contestStr := "no"
			if room.Contest != nil {
				contestStr = "yes"
			}
			fmt.Printf("%-20s %-10s %-8d %-8s %s\n", rid, hostName, len(room.UserIDs()), contestStr, stateStr)
		}
	})
}

func (c *CLI) listUsers() {
	c.state.WithRLock(func() {
		if len(c.state.Users) == 0 {
			fmt.Println("No online users.")
			return
		}
		fmt.Printf("%-10s %-20s %-20s %s\n", "ID", "Name", "Room", "Monitor")
		fmt.Println(strings.Repeat("-", 70))
		for _, u := range c.state.Users {
			roomID := ""
			if u.Room != nil {
				roomID = string(u.Room.ID)
			}
			fmt.Printf("%-10d %-20s %-20s %v\n", u.ID, u.Name, roomID, u.Monitor)
		}
	})
}

func (c *CLI) cmdKickUser(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: kick <user-id>")
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println("Invalid user ID")
		return
	}
	var name string
	c.state.WithRLock(func() {
		if u := c.state.Users[int32(id)]; u != nil {
			name = u.Name
		}
	})
	if c.kickUserFn != nil && c.kickUserFn(int32(id)) {
		fmt.Printf("Kicked user %d (%s)\n", id, name)
	} else {
		fmt.Println("User not found or not online")
	}
}

func (c *CLI) banUser(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ban <user-id>")
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println("Invalid user ID")
		return
	}
	c.state.WithLock(func() {
		c.state.BannedUsers[int32(id)] = struct{}{}
	})
	fmt.Printf("Banned user %d\n", id)
	_ = c.state.SaveAdminData()
}

func (c *CLI) unbanUser(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: unban <user-id>")
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println("Invalid user ID")
		return
	}
	c.state.WithLock(func() {
		delete(c.state.BannedUsers, int32(id))
	})
	fmt.Printf("Unbanned user %d\n", id)
	_ = c.state.SaveAdminData()
}

func (c *CLI) banList() {
	c.state.WithRLock(func() {
		if len(c.state.BannedUsers) == 0 {
			fmt.Println("No banned users.")
			return
		}
		fmt.Println("Banned users:")
		for id := range c.state.BannedUsers {
			fmt.Printf("  %d\n", id)
		}
	})
}

func (c *CLI) broadcast(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: broadcast <message>")
		return
	}
	msg := strings.Join(args, " ")
	cmd := protocol.ServerCommand{
		Type: protocol.ServerCmdMessage,
		Message: protocol.Message{
			Type:    protocol.MessageChat,
			User:    0,
			Content: "[Broadcast] " + msg,
		},
	}
	if c.broadcastAll != nil {
		_ = c.broadcastAll(cmd)
	}
	fmt.Println("Broadcast sent.")
}

func (c *CLI) handleContest(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: contest <room-id> <enable|disable|whitelist|start> [args...]")
		return
	}

	rid, err := roomid.Parse(args[0])
	if err != nil {
		fmt.Println("Invalid room ID")
		return
	}

	subCmd := strings.ToLower(args[1])

	switch subCmd {
	case "enable":
		c.contestEnable(rid, args[2:])
	case "disable":
		c.contestDisable(rid)
	case "whitelist":
		c.contestWhitelist(rid, args[2:])
	case "start":
		c.contestStart(rid, args[2:])
	default:
		fmt.Printf("Unknown contest subcommand: %s\n", subCmd)
	}
}

func (c *CLI) contestEnable(rid roomid.RoomID, userArgs []string) {
	var ok bool
	c.state.WithLock(func() {
		room, exists := c.state.Rooms[rid]
		if !exists {
			return
		}
		whitelist := make(map[int32]struct{})
		if len(userArgs) > 0 {
			for _, arg := range userArgs {
				if id, err := strconv.ParseInt(arg, 10, 32); err == nil {
					whitelist[int32(id)] = struct{}{}
				}
			}
		} else {
			for _, id := range room.UserIDs() {
				whitelist[id] = struct{}{}
			}
			for _, id := range room.MonitorIDs() {
				whitelist[id] = struct{}{}
			}
		}
		room.Contest = &game.ContestConfig{
			Whitelist:   whitelist,
			ManualStart: true,
			AutoDisband: true,
		}
		ok = true
	})
	if ok {
		fmt.Printf("Contest mode enabled for room %s\n", rid)
	} else {
		fmt.Println("Room not found")
	}
}

func (c *CLI) contestDisable(rid roomid.RoomID) {
	var ok bool
	c.state.WithLock(func() {
		room, exists := c.state.Rooms[rid]
		if !exists {
			return
		}
		room.Contest = nil
		ok = true
	})
	if ok {
		fmt.Printf("Contest mode disabled for room %s\n", rid)
	} else {
		fmt.Println("Room not found")
	}
}

func (c *CLI) contestWhitelist(rid roomid.RoomID, userArgs []string) {
	if len(userArgs) == 0 {
		fmt.Println("Usage: contest <room> whitelist <user-id>...")
		return
	}
	var ok bool
	c.state.WithLock(func() {
		room, exists := c.state.Rooms[rid]
		if !exists || room.Contest == nil {
			return
		}
		whitelist := make(map[int32]struct{})
		for _, arg := range userArgs {
			if id, err := strconv.ParseInt(arg, 10, 32); err == nil {
				whitelist[int32(id)] = struct{}{}
			}
		}
		for _, id := range room.UserIDs() {
			whitelist[id] = struct{}{}
		}
		for _, id := range room.MonitorIDs() {
			whitelist[id] = struct{}{}
		}
		room.Contest.Whitelist = whitelist
		ok = true
	})
	if ok {
		fmt.Printf("Contest whitelist updated for room %s\n", rid)
	} else {
		fmt.Println("Room not found or contest not enabled")
	}
}

func (c *CLI) contestStart(rid roomid.RoomID, args []string) {
	force := len(args) > 0 && strings.ToLower(args[0]) == "force"

	var result struct {
		ok    bool
		room  *game.Room
		state string
	}
	c.state.WithLock(func() {
		room, exists := c.state.Rooms[rid]
		if !exists || room.Contest == nil {
			result.state = "contest-room-not-found"
			return
		}
		st, ok := room.State.(*game.StateWaitForReady)
		if !ok {
			result.state = "room-not-waiting"
			return
		}
		if room.Chart == nil {
			result.state = "no-chart-selected"
			return
		}
		allIds := room.AllParticipantIDs()
		allReady := true
		for _, id := range allIds {
			if _, ok := st.Started[id]; !ok {
				allReady = false
				break
			}
		}
		if !allReady && !force {
			result.state = "not-all-ready"
			return
		}
		result.ok = true
		result.room = room
	})

	if !result.ok {
		fmt.Printf("Cannot start contest: %s\n", result.state)
		return
	}

	room := result.room
	users := room.UserIDs()
	monitors := room.MonitorIDs()

	if c.state.Logger != nil && c.state.ServerLang != nil {
		sep := ", "
		if c.state.ServerLang.Format("lang-check", nil) == "zh" {
			sep = "、"
		}
		usersText := joinInt32s(users, sep)
		var monitorsSuffix string
		if len(monitors) > 0 {
			monitorsText := joinInt32s(monitors, sep)
			monitorsSuffix = c.state.ServerLang.Format("log-room-game-start-monitors", map[string]string{"monitors": monitorsText})
		}
		c.state.Logger.Info(c.state.ServerLang.Format("log-room-game-start", map[string]string{"users": usersText, "monitorsSuffix": monitorsSuffix}))
	}

	cmd := protocol.ServerCommand{
		Type:    protocol.ServerCmdMessage,
		Message: protocol.Message{Type: protocol.MessageStartPlaying},
	}
	c.state.WithRLock(func() {
		for _, uid := range users {
			if u := c.state.Users[uid]; u != nil {
				_ = u.TrySend(cmd)
			}
		}
		for _, mid := range monitors {
			if u := c.state.Users[mid]; u != nil {
				_ = u.TrySend(cmd)
			}
		}
	})

	c.state.WithLock(func() {
		for _, uid := range users {
			if u := c.state.Users[uid]; u != nil {
				u.GameTime = -1e9
			}
		}
		room.State = &game.StatePlaying{
			Results: make(map[int32]*game.RecordData),
			Aborted: make(map[int32]struct{}),
		}
	})

	changeStateCmd := protocol.ServerCommand{
		Type:  protocol.ServerCmdChangeState,
		State: room.ClientRoomState(),
	}
	c.state.WithRLock(func() {
		for _, uid := range users {
			if u := c.state.Users[uid]; u != nil {
				_ = u.TrySend(changeStateCmd)
			}
		}
		for _, mid := range monitors {
			if u := c.state.Users[mid]; u != nil {
				_ = u.TrySend(changeStateCmd)
			}
		}
	})

	fmt.Printf("Contest game started in room %s\n", rid)
}

func joinInt32s(ids []int32, sep string) string {
	if len(ids) == 0 {
		return ""
	}
	var parts []string
	for _, id := range ids {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, sep)
}
