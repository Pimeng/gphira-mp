package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/client"
)

func runRoom(argv []string) {
	var host string
	var port int
	var rooms, playersPerRoom, monitorsPerRoom int
	var rate, duration int
	var tokensStr string

	fs := flag.NewFlagSet("room", flag.ExitOnError)
	fs.StringVar(&host, "host", "127.0.0.1", "Server host")
	fs.IntVar(&port, "port", 12346, "Server port")
	fs.IntVar(&rooms, "rooms", 5, "Number of rooms")
	fs.IntVar(&playersPerRoom, "players-per-room", 2, "Players per room")
	fs.IntVar(&monitorsPerRoom, "monitors-per-room", 0, "Monitors per room")
	fs.IntVar(&rate, "rate", 10, "Operations per second")
	fs.IntVar(&duration, "duration", 30, "Duration in seconds")
	fs.StringVar(&tokensStr, "tokens", "", "Comma-separated auth tokens (or set BENCH_TOKEN env)")
	_ = fs.Parse(argv)

	tokens := splitTokens(tokensStr)
	if len(tokens) == 0 {
		if env := os.Getenv("BENCH_TOKEN"); env != "" {
			tokens = splitTokens(env)
		}
	}

	totalClients := rooms * (playersPerRoom + monitorsPerRoom)
	if len(tokens) < totalClients {
		autoCount := totalClients - len(tokens)
		for i := 0; i < autoCount; i++ {
			tokens = append(tokens, fmt.Sprintf("bench-auto-%d", i+1))
		}
		fmt.Printf("Warning: Auto-generated %d token(s) to reach %d required clients.\n", autoCount, totalClients)
	}

	printBenchHeader("Room Benchmark", map[string]interface{}{
		"host":              host,
		"port":              port,
		"rooms":             rooms,
		"playersPerRoom":    playersPerRoom,
		"monitorsPerRoom":   monitorsPerRoom,
		"totalClients":      totalClients,
		"rate":              fmt.Sprintf("%d/s", rate),
		"duration":          fmt.Sprintf("%ds", duration),
		"tokens":            len(tokens),
	})

	metrics := NewMetricsCollector(1000)
	metrics.Start()

	startTime := time.Now()
	var clients []*client.Client
	var connectLatencies, authLatencies, createLatencies, joinLatencies []int
	var clientsConnected, clientsConnectFailed int
	var roomsCreated, roomsCreateFailed int
	var joinedPlayers, joinFailed int
	var readySuccess, readyFailed int
	var chatSuccess, chatFailed int
	errorSummary := make(map[string]int)

	intervalMs := 1000.0 / float64(rate)

	// Phase 1: Connect and authenticate
	for i := 0; i < totalClients; i++ {
		expectedDelay := float64(i) * intervalMs
		elapsed := time.Since(startTime).Milliseconds()
		wait := int(expectedDelay) - int(elapsed)
		if wait > 0 {
			time.Sleep(time.Duration(wait) * time.Millisecond)
		}
		printProgress("Connecting", i+1, totalClients, "")

		connectStart := time.Now()
		c, err := client.Connect(host, port, nil)
		connectLatency := int(time.Since(connectStart).Milliseconds())

		if err != nil {
			connectLatencies = append(connectLatencies, connectLatency)
			clientsConnectFailed++
			errorSummary[err.Error()]++
			continue
		}
		clientsConnected++
		connectLatencies = append(connectLatencies, connectLatency)

		token := tokens[len(clients)%len(tokens)]
		if token != "" {
			authStart := time.Now()
			if err := c.Authenticate(token); err != nil {
				authLatencies = append(authLatencies, int(time.Since(authStart).Milliseconds()))
				errorSummary[fmt.Sprintf("auth: %s", err.Error())]++
			} else {
				authLatencies = append(authLatencies, int(time.Since(authStart).Milliseconds()))
			}
		}
		clients = append(clients, c)
	}

	clearProgress()
	fmt.Printf("Connected %d/%d clients. Creating rooms...\n", clientsConnected, totalClients)

	// Phase 2: Assign roles and create rooms
	type roomAssignment struct {
		host     *client.Client
		players  []*client.Client
		monitors []*client.Client
		roomID   string
	}

	var roomAssignments []roomAssignment
	clientIdx := 0
	for r := 0; r < rooms && clientIdx < len(clients); r++ {
		hostClient := clients[clientIdx]
		clientIdx++
		var players []*client.Client
		var monitors []*client.Client
		for p := 1; p < playersPerRoom && clientIdx < len(clients); p++ {
			players = append(players, clients[clientIdx])
			clientIdx++
		}
		for m := 0; m < monitorsPerRoom && clientIdx < len(clients); m++ {
			monitors = append(monitors, clients[clientIdx])
			clientIdx++
		}
		roomID := fmt.Sprintf("bench%d_%d", r, time.Now().UnixMilli())
		roomAssignments = append(roomAssignments, roomAssignment{host: hostClient, players: players, monitors: monitors, roomID: roomID})
	}

	for i, room := range roomAssignments {
		printProgress("Creating rooms", i+1, len(roomAssignments), "")
		createStart := time.Now()
		if err := room.host.CreateRoom(room.roomID); err != nil {
			createLatencies = append(createLatencies, int(time.Since(createStart).Milliseconds()))
			roomsCreateFailed++
			errorSummary[fmt.Sprintf("createRoom: %s", err.Error())]++
		} else {
			createLatencies = append(createLatencies, int(time.Since(createStart).Milliseconds()))
			roomsCreated++
		}
	}

	clearProgress()
	fmt.Printf("Created %d/%d rooms. Joining players...\n", roomsCreated, len(roomAssignments))

	// Phase 3: Join rooms
	joinTotal := 0
	expectedJoins := roomsCreated * (playersPerRoom - 1 + monitorsPerRoom)
	for _, room := range roomAssignments {
		roomID, ok := room.host.RoomID()
		if !ok {
			continue
		}
		for _, p := range room.players {
			joinTotal++
			printProgress("Joining", joinTotal, expectedJoins, "")
			joinStart := time.Now()
			if _, err := p.JoinRoom(roomID.String(), false); err != nil {
				joinLatencies = append(joinLatencies, int(time.Since(joinStart).Milliseconds()))
				joinFailed++
				errorSummary[fmt.Sprintf("joinRoom: %s", err.Error())]++
			} else {
				joinLatencies = append(joinLatencies, int(time.Since(joinStart).Milliseconds()))
				joinedPlayers++
			}
		}
		for _, m := range room.monitors {
			joinTotal++
			printProgress("Joining", joinTotal, expectedJoins, "")
			joinStart := time.Now()
			if _, err := m.JoinRoom(roomID.String(), true); err != nil {
				joinLatencies = append(joinLatencies, int(time.Since(joinStart).Milliseconds()))
				joinFailed++
				errorSummary[fmt.Sprintf("monitorJoin: %s", err.Error())]++
			} else {
				joinLatencies = append(joinLatencies, int(time.Since(joinStart).Milliseconds()))
				joinedPlayers++
			}
		}
	}

	clearProgress()
	fmt.Println("Performing room actions...")

	// Phase 4: Ready + Chat
	for _, room := range roomAssignments {
		if _, ok := room.host.RoomID(); !ok {
			continue
		}
		all := append([]*client.Client{room.host}, room.players...)
		for _, c := range all {
			if err := c.Ready(); err != nil {
				readyFailed++
				errorSummary[fmt.Sprintf("ready: %s", err.Error())]++
			} else {
				readySuccess++
			}
			if err := c.Chat("bench test message"); err != nil {
				chatFailed++
				errorSummary[fmt.Sprintf("chat: %s", err.Error())]++
			} else {
				chatSuccess++
			}
		}
	}

	fmt.Printf("Actions done. Ready: %d/%d, Chat: %d/%d. Keeping for %ds...\n",
		readySuccess, readySuccess+readyFailed, chatSuccess, chatSuccess+chatFailed, duration)

	// Phase 5: Keep connections
	elapsedTotal := int(time.Since(startTime).Milliseconds())
	remaining := duration*1000 - elapsedTotal
	if remaining > 0 {
		endAt := time.Now().Add(time.Duration(remaining) * time.Millisecond)
		for time.Now().Before(endAt) {
			left := int(time.Until(endAt).Seconds())
			printProgress("Running", duration-left, duration, fmt.Sprintf("| Active: %d", len(clients)))
			time.Sleep(minDuration(time.Second, time.Until(endAt)))
		}
		clearProgress()
	}

	fmt.Println("Leaving rooms and closing connections...")
	for _, c := range clients {
		_ = c.LeaveRoom()
		_ = c.Close()
	}

	endedAt := time.Now()
	metricsSamples := metrics.Stop()
	metricsSummary := SummarizeMetrics(metricsSamples)

	report := &BenchReport{
		BenchType: "room-bench",
		Params: map[string]interface{}{
			"host":              host,
			"port":              port,
			"rooms":             rooms,
			"playersPerRoom":    playersPerRoom,
			"monitorsPerRoom":   monitorsPerRoom,
			"rate":              rate,
			"duration":          duration,
		},
		StartedAt:      startTime.UTC().Format(time.RFC3339),
		EndedAt:        endedAt.UTC().Format(time.RFC3339),
		Duration:       int(endedAt.Sub(startTime).Seconds()),
		MetricsSamples: metricsSamples,
		MetricsSummary: metricsSummary,
		Summary: map[string]interface{}{
			"clientsCreated":       totalClients,
			"clientsConnected":     clientsConnected,
			"clientsConnectFailed": clientsConnectFailed,
			"roomsCreated":         roomsCreated,
			"roomsCreateFailed":    roomsCreateFailed,
			"joinedPlayers":        joinedPlayers,
			"joinFailed":           joinFailed,
			"readySuccess":         readySuccess,
			"readyFailed":          readyFailed,
			"chatSuccess":          chatSuccess,
			"chatFailed":           chatFailed,
			"avgConnectLatency":    fmt.Sprintf("%.2f ms", avgInt(connectLatencies)),
			"avgAuthLatency":       fmt.Sprintf("%.2f ms", avgInt(authLatencies)),
			"avgCreateRoomLatency": fmt.Sprintf("%.2f ms", avgInt(createLatencies)),
			"avgJoinRoomLatency":   fmt.Sprintf("%.2f ms", avgInt(joinLatencies)),
			"duration":             fmt.Sprintf("%ds", duration),
		},
		Errors: mapToBenchErrors(errorSummary),
	}

	filepath := saveReport(report)
	fmt.Printf("Report saved to: %s\n", filepath)
	printBenchFooter(report, metricsSummary)
}

func avgInt(arr []int) float64 {
	if len(arr) == 0 {
		return 0
	}
	sum := 0
	for _, v := range arr {
		sum += v
	}
	return float64(sum) / float64(len(arr))
}

func mapToBenchErrors(m map[string]int) []BenchError {
	var out []BenchError
	for msg, count := range m {
		out = append(out, BenchError{Message: msg, Count: count})
	}
	return out
}
