package cli

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/internal/replay"
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

const cliTempAdminTokenTTL = 4 * time.Hour

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
		case "user":
			c.userInfo(args)
		case "kick":
			c.cmdKickUser(args)
		case "ban":
			c.banUser(args)
		case "unban":
			c.unbanUser(args)
		case "banlist":
			c.banList()
		case "banroom":
			c.banRoomUser(args)
		case "unbanroom":
			c.unbanRoomUser(args)
		case "broadcast", "say":
			c.broadcast(args)
		case "roomsay":
			c.roomSay(args)
		case "maxusers":
			c.maxUsers(args)
		case "disband":
			c.disbandRoom(args)
		case "replay":
			c.toggleReplay(args)
		case "roomcreation":
			c.toggleRoomCreation(args)
		case "ipblacklist":
			c.handleIPBlacklist(args)
		case "contest":
			c.handleContest(args)
		case "approve":
			c.approveCLIApproval(args)
		case "deny", "reject":
			c.denyCLIApproval(args)
		case "pending":
			c.listPendingApprovals()
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

func (c *CLI) parseUserIDArg(arg string) (int32, bool) {
	id, err := strconv.ParseInt(arg, 10, 32)
	if err != nil {
		fmt.Println(c.state.ServerLang.Format("cli-invalid-user-id", nil))
		return 0, false
	}
	return int32(id), true
}

func (c *CLI) parseRoomIDArg(arg string) (roomid.RoomID, bool) {
	rid, err := roomid.Parse(arg)
	if err != nil {
		fmt.Println(c.state.ServerLang.Format("cli-invalid-room-id", nil))
		return "", false
	}
	return rid, true
}

func parseToggleArg(arg string) (string, bool) {
	if arg == "" || strings.EqualFold(arg, "status") {
		return "status", true
	}
	lower := strings.ToLower(arg)
	if lower == "on" || lower == "off" {
		return lower, true
	}
	return "", false
}

func (c *CLI) validateCLIMessage(message string) (string, bool) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		fmt.Println(c.state.ServerLang.Format("cli-message-empty", nil))
		return "", false
	}
	if len(msg) > 200 {
		fmt.Println(c.state.ServerLang.Format("cli-message-too-long", map[string]string{"max": "200"}))
		return "", false
	}
	return msg, true
}

func (c *CLI) findUser(id int32) *game.User {
	var u *game.User
	c.state.WithRLock(func() {
		u = c.state.Users[id]
	})
	return u
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

func (c *CLI) userInfo(args []string) {
	if len(args) < 1 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-user", nil))
		return
	}
	userID, ok := c.parseUserIDArg(args[0])
	if !ok {
		return
	}

	var out map[string]string
	c.state.WithRLock(func() {
		u := c.state.Users[userID]
		if u == nil {
			return
		}
		room := c.state.ServerLang.Format("cli-none", nil)
		if u.GetRoom() != nil {
			room = string(u.GetRoom().ID)
		}
		role := c.state.ServerLang.Format("cli-user-role-player", nil)
		if u.IsMonitor() {
			role = c.state.ServerLang.Format("cli-user-role-monitor", nil)
		}
		status := c.state.ServerLang.Format("cli-user-status-offline", nil)
		if u.HasSession() {
			status = c.state.ServerLang.Format("cli-user-status-online", nil)
		}
		banned := c.state.ServerLang.Format("cli-no", nil)
		if _, ok := c.state.BannedUsers[userID]; ok {
			banned = c.state.ServerLang.Format("cli-yes", nil)
		}
		out = map[string]string{
			"id":       fmt.Sprintf("%d", u.ID),
			"name":     u.GetName(),
			"status":   status,
			"role":     role,
			"room":     room,
			"banned":   banned,
			"time":     fmt.Sprintf("%.3f", u.GetGameTime()),
			"language": u.GetLang().Lang(),
		}
	})
	if out == nil {
		fmt.Println(c.state.ServerLang.Format("cli-user-not-found", map[string]string{"id": fmt.Sprintf("%d", userID)}))
		return
	}
	fmt.Println(c.state.ServerLang.Format("cli-user-info-header", nil))
	fmt.Println(c.state.ServerLang.Format("cli-user-info-id", out))
	fmt.Println(c.state.ServerLang.Format("cli-user-info-name", out))
	fmt.Println(c.state.ServerLang.Format("cli-user-info-status", out))
	fmt.Println(c.state.ServerLang.Format("cli-user-info-role", out))
	fmt.Println(c.state.ServerLang.Format("cli-user-info-room", out))
	fmt.Println(c.state.ServerLang.Format("cli-user-info-banned", out))
	fmt.Println(c.state.ServerLang.Format("cli-user-info-game-time", out))
	fmt.Println(c.state.ServerLang.Format("cli-user-info-language", map[string]string{"lang": out["language"]}))
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

func (c *CLI) banRoomUser(args []string) {
	if len(args) < 2 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-banroom", nil))
		return
	}
	userID, ok := c.parseUserIDArg(args[0])
	if !ok {
		return
	}
	rid, ok := c.parseRoomIDArg(args[1])
	if !ok {
		return
	}
	c.state.WithLock(func() {
		set := c.state.BannedRoomUsers[rid]
		if set == nil {
			set = make(map[int32]struct{})
			c.state.BannedRoomUsers[rid] = set
		}
		set[userID] = struct{}{}
	})
	_ = c.state.SaveAdminData()
	fmt.Println(c.state.ServerLang.Format("cli-room-user-banned", map[string]string{"userId": fmt.Sprintf("%d", userID), "room": string(rid)}))
}

func (c *CLI) unbanRoomUser(args []string) {
	if len(args) < 2 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-unbanroom", nil))
		return
	}
	userID, ok := c.parseUserIDArg(args[0])
	if !ok {
		return
	}
	rid, ok := c.parseRoomIDArg(args[1])
	if !ok {
		return
	}
	c.state.WithLock(func() {
		if set := c.state.BannedRoomUsers[rid]; set != nil {
			delete(set, userID)
			if len(set) == 0 {
				delete(c.state.BannedRoomUsers, rid)
			}
		}
	})
	_ = c.state.SaveAdminData()
	fmt.Println(c.state.ServerLang.Format("cli-room-user-unbanned", map[string]string{"userId": fmt.Sprintf("%d", userID), "room": string(rid)}))
}

func (c *CLI) broadcast(args []string) {
	if len(args) < 1 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-broadcast", nil))
		return
	}
	msg, ok := c.validateCLIMessage(strings.Join(args, " "))
	if !ok {
		return
	}
	cmd := protocol.ServerCommand{
		Type: protocol.ServerCmdMessage,
		Message: protocol.Message{
			Type:    protocol.MessageChat,
			User:    0,
			Content: msg,
		},
	}

	var roomIDs []roomid.RoomID
	c.state.WithRLock(func() {
		roomIDs = make([]roomid.RoomID, 0, len(c.state.Rooms))
		for rid := range c.state.Rooms {
			roomIDs = append(roomIDs, rid)
		}
	})
	for _, rid := range roomIDs {
		var room *game.Room
		c.state.WithRLock(func() {
			room = c.state.Rooms[rid]
		})
		if room == nil {
			continue
		}
		for _, id := range room.AllParticipantIDs() {
			if u := c.findUser(id); u != nil {
				_ = u.TrySend(cmd)
			}
		}
	}
	if c.logger != nil {
		c.logger.Info("admin broadcast", "message", msg, "rooms", len(roomIDs))
	}
	fmt.Println(c.state.ServerLang.Format("cli-broadcast-sent", nil))
}

func (c *CLI) roomSay(args []string) {
	if len(args) < 2 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-roomsay", nil))
		return
	}
	rid, ok := c.parseRoomIDArg(args[0])
	if !ok {
		return
	}
	msg, ok := c.validateCLIMessage(strings.Join(args[1:], " "))
	if !ok {
		return
	}

	var room *game.Room
	c.state.WithRLock(func() {
		room = c.state.Rooms[rid]
	})
	if room == nil {
		fmt.Println(c.state.ServerLang.Format("cli-room-not-found", nil))
		return
	}
	room.AddLog(msg)
	cmd := protocol.ServerCommand{
		Type:    protocol.ServerCmdMessage,
		Message: protocol.Message{Type: protocol.MessageChat, User: 0, Content: msg},
	}
	for _, id := range room.AllParticipantIDs() {
		if u := c.findUser(id); u != nil {
			_ = u.TrySend(cmd)
		}
	}
	if c.state.WSServer != nil {
		c.state.WSServer.BroadcastRoomLog(rid, msg, time.Now().UnixMilli())
	}
	fmt.Println(c.state.ServerLang.Format("cli-room-message-sent", map[string]string{"room": string(rid)}))
}

func (c *CLI) maxUsers(args []string) {
	if len(args) < 2 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-maxusers", nil))
		return
	}
	rid, ok := c.parseRoomIDArg(args[0])
	if !ok {
		return
	}
	n, err := strconv.Atoi(args[1])
	if err != nil || n < 1 || n > 64 {
		fmt.Println(c.state.ServerLang.Format("cli-bad-max-users", nil))
		return
	}
	var updated bool
	c.state.WithLock(func() {
		if room := c.state.Rooms[rid]; room != nil {
			room.SetMaxUsers(n)
			updated = true
		}
	})
	if !updated {
		fmt.Println(c.state.ServerLang.Format("cli-room-not-found", nil))
		return
	}
	fmt.Println(c.state.ServerLang.Format("cli-room-max-users-set", map[string]string{"room": string(rid), "count": fmt.Sprintf("%d", n)}))
	if c.state.WSServer != nil {
		c.state.WSServer.BroadcastRoomUpdate(rid, nil)
	}
}

func (c *CLI) disbandRoom(args []string) {
	if len(args) < 1 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-disband", nil))
		return
	}
	rid, ok := c.parseRoomIDArg(args[0])
	if !ok {
		return
	}
	var room *game.Room
	c.state.WithRLock(func() {
		room = c.state.Rooms[rid]
	})
	if room == nil {
		fmt.Println(c.state.ServerLang.Format("cli-room-not-found", nil))
		return
	}

	msg := c.state.ServerLang.Format("room-disbanded-by-admin", nil)
	if msg == "room-disbanded-by-admin" {
		msg = "Room disbanded by admin"
	}
	cmd := protocol.ServerCommand{
		Type:    protocol.ServerCmdMessage,
		Message: protocol.Message{Type: protocol.MessageChat, User: 0, Content: msg},
	}
	for _, id := range room.AllParticipantIDs() {
		if u := c.findUser(id); u != nil {
			_ = u.TrySend(cmd)
			u.ClearRoom()
		}
	}
	c.state.WithLock(func() {
		delete(c.state.Rooms, rid)
	})
	if c.state.ReplayEnabled && c.state.ReplayRecorder != nil && room.ReplayEligible {
		c.state.ReplayRecorder.EndRoom(rid)
	}
	if c.state.WSServer != nil {
		c.state.WSServer.BroadcastRoomUpdate(rid, nil)
	}
	fmt.Println(c.state.ServerLang.Format("cli-room-disbanded", map[string]string{"room": string(rid)}))
}

func (c *CLI) toggleReplay(args []string) {
	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}
	toggle, ok := parseToggleArg(arg)
	if !ok {
		fmt.Println(c.state.ServerLang.Format("cli-usage-replay", nil))
		return
	}
	if toggle == "status" {
		stateText := c.state.ServerLang.Format("cli-state-off", nil)
		if c.state.ReplayEnabled {
			stateText = c.state.ServerLang.Format("cli-state-on", nil)
		}
		fmt.Println(c.state.ServerLang.Format("cli-replay-status", map[string]string{"state": stateText}))
		return
	}
	enabled := toggle == "on"
	var roomsToEnd []roomid.RoomID
	c.state.WithLock(func() {
		c.state.ReplayEnabled = enabled
		c.state.Config.ReplayEnabled = &enabled
		if !enabled {
			for rid := range c.state.Rooms {
				roomsToEnd = append(roomsToEnd, rid)
			}
		}
		for _, room := range c.state.Rooms {
			room.RefreshLive(enabled)
		}
	})
	if !enabled && c.state.ReplayRecorder != nil {
		for _, rid := range roomsToEnd {
			c.state.ReplayRecorder.EndRoom(rid)
		}
	}
	if enabled {
		fmt.Println(c.state.ServerLang.Format("cli-replay-toggled-on", nil))
	} else {
		fmt.Println(c.state.ServerLang.Format("cli-replay-toggled-off", nil))
	}
}

func (c *CLI) toggleRoomCreation(args []string) {
	arg := ""
	if len(args) > 0 {
		arg = args[0]
	}
	toggle, ok := parseToggleArg(arg)
	if !ok {
		fmt.Println(c.state.ServerLang.Format("cli-usage-roomcreation", nil))
		return
	}
	if toggle == "status" {
		stateText := c.state.ServerLang.Format("cli-state-off", nil)
		if c.state.RoomCreationEnabled {
			stateText = c.state.ServerLang.Format("cli-state-on", nil)
		}
		fmt.Println(c.state.ServerLang.Format("cli-room-creation-status", map[string]string{"state": stateText}))
		return
	}
	enabled := toggle == "on"
	c.state.WithLock(func() {
		c.state.RoomCreationEnabled = enabled
		c.state.Config.RoomCreationEnabled = &enabled
	})
	if enabled {
		fmt.Println(c.state.ServerLang.Format("cli-room-creation-toggled-on", nil))
	} else {
		fmt.Println(c.state.ServerLang.Format("cli-room-creation-toggled-off", nil))
	}
}

func (c *CLI) handleIPBlacklist(args []string) {
	if len(args) == 0 {
		fmt.Println(c.state.ServerLang.Format("cli-usage-ipblacklist", nil))
		return
	}
	switch strings.ToLower(args[0]) {
	case "list":
		blacklist := c.logger.GetBlacklistedIPs()
		if len(blacklist) == 0 {
			fmt.Println(c.state.ServerLang.Format("cli-blacklist-empty", nil))
			return
		}
		fmt.Println(c.state.ServerLang.Format("cli-blacklist-header", map[string]string{"count": fmt.Sprintf("%d", len(blacklist))}))
		for _, item := range blacklist {
			minutes := (item.ExpiresIn + 59999) / 60000
			fmt.Println(c.state.ServerLang.Format("cli-blacklist-line", map[string]string{"ip": item.IP, "minutes": fmt.Sprintf("%d", minutes)}))
		}
	case "remove":
		if len(args) < 2 {
			fmt.Println(c.state.ServerLang.Format("cli-usage-ipblacklist-remove", nil))
			return
		}
		c.logger.RemoveFromBlacklist(args[1])
		fmt.Println(c.state.ServerLang.Format("cli-blacklist-removed", map[string]string{"ip": args[1]}))
	case "clear":
		c.logger.ClearBlacklist()
		fmt.Println(c.state.ServerLang.Format("cli-blacklist-cleared", nil))
	default:
		fmt.Println(c.state.ServerLang.Format("cli-ipblacklist-unknown-subcommand", nil))
	}
}

func (c *CLI) findApprovalSessionLocked(input string) (string, *state.CLIApprovalSession, string) {
	if sess := c.state.CLIApprovalSessions[input]; sess != nil {
		return input, sess, ""
	}
	var found string
	for ssid := range c.state.CLIApprovalSessions {
		if strings.HasPrefix(ssid, input) {
			if found != "" {
				return "", nil, "ambiguous"
			}
			found = ssid
		}
	}
	if found == "" {
		return "", nil, "not-found"
	}
	return found, c.state.CLIApprovalSessions[found], ""
}

func shortApprovalID(ssid string) string {
	if len(ssid) <= 8 {
		return ssid
	}
	return ssid[:8]
}

func (c *CLI) approveCLIApproval(args []string) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		fmt.Println(c.state.ServerLang.Format("cli-usage-approve", nil))
		return
	}
	input := strings.TrimSpace(args[0])

	var printed string
	var logMsg string
	c.state.WithLock(func() {
		ssid, session, status := c.findApprovalSessionLocked(input)
		switch status {
		case "ambiguous":
			printed = c.state.ServerLang.Format("cli-approve-ambiguous", map[string]string{"input": input})
			return
		case "not-found":
			printed = c.state.ServerLang.Format("cli-approve-not-found", map[string]string{"input": input})
			return
		}
		short := shortApprovalID(ssid)
		if time.Now().UnixMilli() > session.ExpiresAt {
			delete(c.state.CLIApprovalSessions, ssid)
			printed = c.state.ServerLang.Format("cli-approve-expired", map[string]string{"ssid": short})
			return
		}
		if session.Status != "" && session.Status != "pending" {
			printed = c.state.ServerLang.Format("cli-approve-already-handled", map[string]string{"ssid": short, "status": session.Status})
			return
		}

		token := newCLIToken()
		tokenExpiresAt := time.Now().Add(cliTempAdminTokenTTL).UnixMilli()
		c.state.TempAdminTokens[token] = &state.TempAdminToken{IP: session.IP, ExpiresAt: tokenExpiresAt}
		session.Status = "approved"
		session.Token = token
		session.TokenExpiresAt = tokenExpiresAt
		printed = c.state.ServerLang.Format("cli-approve-success", map[string]string{"ssid": short, "ip": session.IP})
		logMsg = fmt.Sprintf("[OTP CLI Approve] session %s approved, issued temporary token %s... (IP: %s)", short, token[:8], session.IP)
	})
	if logMsg != "" && c.logger != nil {
		c.logger.Info(logMsg)
	}
	fmt.Println(printed)
}

func (c *CLI) denyCLIApproval(args []string) {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		fmt.Println(c.state.ServerLang.Format("cli-usage-deny", nil))
		return
	}
	input := strings.TrimSpace(args[0])

	var printed string
	var logMsg string
	c.state.WithLock(func() {
		ssid, session, status := c.findApprovalSessionLocked(input)
		switch status {
		case "ambiguous":
			printed = c.state.ServerLang.Format("cli-approve-ambiguous", map[string]string{"input": input})
			return
		case "not-found":
			printed = c.state.ServerLang.Format("cli-approve-not-found", map[string]string{"input": input})
			return
		}
		short := shortApprovalID(ssid)
		if session.Status != "" && session.Status != "pending" {
			printed = c.state.ServerLang.Format("cli-approve-already-handled", map[string]string{"ssid": short, "status": session.Status})
			return
		}
		session.Status = "denied"
		printed = c.state.ServerLang.Format("cli-deny-success", map[string]string{"ssid": short, "ip": session.IP})
		logMsg = fmt.Sprintf("[OTP CLI Deny] session %s denied (IP: %s)", short, session.IP)
	})
	if logMsg != "" && c.logger != nil {
		c.logger.Info(logMsg)
	}
	fmt.Println(printed)
}

func (c *CLI) listPendingApprovals() {
	type pendingApproval struct {
		SSID         string
		IP           string
		RemainingSec int64
	}
	var items []pendingApproval
	now := time.Now().UnixMilli()
	c.state.WithLock(func() {
		for ssid, session := range c.state.CLIApprovalSessions {
			if session == nil || now > session.ExpiresAt {
				delete(c.state.CLIApprovalSessions, ssid)
				continue
			}
			if session.Status != "" && session.Status != "pending" {
				continue
			}
			remaining := (session.ExpiresAt - now + 999) / 1000
			items = append(items, pendingApproval{SSID: ssid, IP: session.IP, RemainingSec: remaining})
		}
	})
	sort.Slice(items, func(i, j int) bool { return items[i].SSID < items[j].SSID })
	if len(items) == 0 {
		fmt.Println(c.state.ServerLang.Format("cli-pending-empty", nil))
		return
	}
	fmt.Println(c.state.ServerLang.Format("cli-pending-header", map[string]string{"count": fmt.Sprintf("%d", len(items))}))
	for _, item := range items {
		fmt.Println(c.state.ServerLang.Format("cli-pending-line", map[string]string{
			"ssid":    shortApprovalID(item.SSID),
			"full":    item.SSID,
			"ip":      item.IP,
			"seconds": fmt.Sprintf("%d", item.RemainingSec),
		}))
	}
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
		}
		for _, id := range room.UserIDs() {
			whitelist[id] = struct{}{}
		}
		for _, id := range room.MonitorIDs() {
			whitelist[id] = struct{}{}
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
		if c.state.ReplayEnabled && c.state.ReplayRecorder != nil && room.ReplayEligible && room.Chart != nil {
			var participants []replay.Participant
			for _, uid := range users {
				name := fmt.Sprintf("%d", uid)
				if u := c.state.Users[uid]; u != nil {
					name = u.GetName()
				}
				participants = append(participants, replay.Participant{ID: uid, Name: name})
			}
			c.state.ReplayRecorder.StartRoom(room.ID, room.Chart.ID, room.Chart.Name, participants)
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

func newCLIToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
