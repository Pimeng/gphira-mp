package cli

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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

	restartServer func() error
	updateState   func() *state.ServerState

	scanner *bufio.Scanner
	stop    chan struct{}
	wg      sync.WaitGroup
	done    chan struct{}
}

// NewCLI creates a new CLI instance.
func NewCLI(state *state.ServerState, logger *utils.Logger, broadcastAll func(protocol.ServerCommand) error, stopServer func() error, kickUser func(int32) bool, restartServer func() error, updateState func() *state.ServerState) *CLI {
	return &CLI{
		state:         state,
		logger:        logger,
		broadcastAll:  broadcastAll,
		stopServer:    stopServer,
		kickUserFn:    kickUser,
		restartServer: restartServer,
		updateState:   updateState,
		scanner:       bufio.NewScanner(os.Stdin),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
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
		case "restart", "r":
			fmt.Println(c.state.ServerLang.Format("cli-restarting-server", nil))
			if c.restartServer != nil {
				if err := c.restartServer(); err != nil {
					fmt.Println(c.state.ServerLang.Format("cli-restart-failed", map[string]string{"err": err.Error()}))
				} else {
					if c.updateState != nil {
						c.state = c.updateState()
					}
					fmt.Println(c.state.ServerLang.Format("cli-restarted", nil))
				}
			}
		case "stop", "exit", "quit":
			fmt.Println(c.state.ServerLang.Format("cli-stopping-server", nil))
			if c.stopServer != nil {
				_ = c.stopServer()
			}
			close(c.done)
			return
		default:
			fmt.Println(c.state.ServerLang.Format("cli-unknown-command", map[string]string{"cmd": cmd}))
		}
	}
}

// Done returns a channel that is closed when the CLI receives a stop command.
func (c *CLI) Done() <-chan struct{} {
	return c.done
}

// Stop signals the CLI to stop reading input.
func (c *CLI) Stop() {
	close(c.stop)
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func (c *CLI) printHelp() {
	fmt.Println(c.state.ServerLang.Format("cli-help", nil))
}

func (c *CLI) listRooms() {
	c.state.WithRLock(func() {
		if len(c.state.Rooms) == 0 {
			fmt.Println(c.state.ServerLang.Format("cli-no-active-rooms", nil))
			return
		}
		fmt.Printf("%-20s %-10s %-8s %-8s %s\n", c.state.ServerLang.Format("cli-header-room-id", nil), c.state.ServerLang.Format("cli-header-host", nil), c.state.ServerLang.Format("cli-header-users", nil), c.state.ServerLang.Format("cli-header-contest", nil), c.state.ServerLang.Format("cli-header-state", nil))
		fmt.Println(strings.Repeat("-", 70))
		for rid, room := range c.state.Rooms {
			hostName := ""
			if u := c.state.Users[room.HostID]; u != nil {
				hostName = u.GetName()
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
			fmt.Println(c.state.ServerLang.Format("cli-no-online-users", nil))
			return
		}
		fmt.Printf("%-10s %-20s %-20s %s\n", c.state.ServerLang.Format("cli-header-id", nil), c.state.ServerLang.Format("cli-header-name", nil), c.state.ServerLang.Format("cli-header-room", nil), c.state.ServerLang.Format("cli-header-monitor", nil))
		fmt.Println(strings.Repeat("-", 70))
		for _, u := range c.state.Users {
			roomID := ""
			if u.GetRoom() != nil {
				roomID = string(u.GetRoom().ID)
			}
			fmt.Printf("%-10d %-20s %-20s %v\n", u.ID, u.GetName(), roomID, u.IsMonitor())
		}
	})
}

func (c *CLI) cmdKickUser(args []string) {
	if len(args) < 1 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-kick", nil))
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println(c.state.ServerLang.Format("cli-invalid-user-id", nil))
		return
	}
	var name string
	c.state.WithRLock(func() {
		if u := c.state.Users[int32(id)]; u != nil {
			name = u.GetName()
		}
	})
	if c.kickUserFn != nil && c.kickUserFn(int32(id)) {
		fmt.Println(c.state.ServerLang.Format("cli-kicked-user", map[string]string{"id": fmt.Sprintf("%d", id), "name": name}))
	} else {
		fmt.Println(c.state.ServerLang.Format("cli-user-not-found", nil))
	}
}

func (c *CLI) banUser(args []string) {
	if len(args) < 1 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-ban", nil))
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println(c.state.ServerLang.Format("cli-invalid-user-id", nil))
		return
	}
	c.state.WithLock(func() {
		c.state.BannedUsers[int32(id)] = struct{}{}
	})
	fmt.Println(c.state.ServerLang.Format("cli-banned-user", map[string]string{"id": fmt.Sprintf("%d", id)}))
	_ = c.state.SaveAdminData()
}

func (c *CLI) unbanUser(args []string) {
	if len(args) < 1 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-unban", nil))
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Println(c.state.ServerLang.Format("cli-invalid-user-id", nil))
		return
	}
	c.state.WithLock(func() {
		delete(c.state.BannedUsers, int32(id))
	})
	fmt.Println(c.state.ServerLang.Format("cli-unbanned-user", map[string]string{"id": fmt.Sprintf("%d", id)}))
	_ = c.state.SaveAdminData()
}

func (c *CLI) banList() {
	c.state.WithRLock(func() {
		if len(c.state.BannedUsers) == 0 {
			fmt.Println(c.state.ServerLang.Format("cli-no-banned-users", nil))
			return
		}
		fmt.Println(c.state.ServerLang.Format("cli-banned-users", nil))
		for id := range c.state.BannedUsers {
			fmt.Printf("  %d\n", id)
		}
	})
}

func (c *CLI) broadcast(args []string) {
	if len(args) < 1 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-broadcast", nil))
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
	fmt.Println(c.state.ServerLang.Format("cli-broadcast-sent", nil))
}

func (c *CLI) handleContest(args []string) {
	if len(args) < 2 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-contest", nil))
		return
	}

	rid, err := roomid.Parse(args[0])
	if err != nil {
		fmt.Println(c.state.ServerLang.Format("cli-invalid-room-id", nil))
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
		fmt.Println(c.state.ServerLang.Format("cli-unknown-contest-subcommand", map[string]string{"cmd": subCmd}))
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
		fmt.Println(c.state.ServerLang.Format("cli-contest-enabled", map[string]string{"room": string(rid)}))
	} else {
		fmt.Println(c.state.ServerLang.Format("cli-room-not-found", nil))
	}
}

func (c *CLI) contestDisable(rid roomid.RoomID) {
	var ok bool
	c.state.WithLock(func() {
		room, exists := c.state.Rooms[rid]
		if !exists {
			return
		}
		room.ClearContest()
		ok = true
	})
	if ok {
		fmt.Println(c.state.ServerLang.Format("cli-contest-disabled", map[string]string{"room": string(rid)}))
	} else {
		fmt.Println(c.state.ServerLang.Format("cli-room-not-found", nil))
	}
}

func (c *CLI) contestWhitelist(rid roomid.RoomID, userArgs []string) {
	if len(userArgs) == 0 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-contest-whitelist", nil))
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
		_ = room.SetContestWhitelist(whitelist)
		ok = true
	})
	if ok {
		fmt.Println(c.state.ServerLang.Format("cli-contest-whitelist-updated", map[string]string{"room": string(rid)}))
	} else {
		fmt.Println(c.state.ServerLang.Format("cli-room-not-found-or-contest-disabled", nil))
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
		fmt.Println(c.state.ServerLang.Format("cli-cannot-start-contest", map[string]string{"reason": result.state}))
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
				u.ResetGameTime()
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

	fmt.Println(c.state.ServerLang.Format("cli-contest-started", map[string]string{"room": string(rid)}))
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
