package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/client"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

func runGameplay(argv []string) {
	var host string
	var port int
	var rooms, playersPerRoom, monitorsPerRoom int
	var rate, duration, hz int
	var tokensStr string

	fs := flag.NewFlagSet("gameplay", flag.ExitOnError)
	fs.StringVar(&host, "host", "127.0.0.1", "Server host")
	fs.IntVar(&port, "port", 12346, "Server port")
	fs.IntVar(&rooms, "rooms", 2, "Number of rooms")
	fs.IntVar(&playersPerRoom, "players-per-room", 2, "Players per room")
	fs.IntVar(&monitorsPerRoom, "monitors-per-room", 0, "Monitors per room")
	fs.IntVar(&rate, "rate", 10, "Operations per second")
	fs.IntVar(&duration, "duration", 30, "Duration in seconds")
	fs.IntVar(&hz, "hz", 20, "Messages per second per client")
	fs.StringVar(&tokensStr, "tokens", "", "Comma-separated auth tokens")
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

	printBenchHeader("Gameplay Benchmark", map[string]interface{}{
		"host":              host,
		"port":              port,
		"rooms":             rooms,
		"playersPerRoom":    playersPerRoom,
		"monitorsPerRoom":   monitorsPerRoom,
		"hz":                hz,
		"totalClients":      totalClients,
		"duration":          fmt.Sprintf("%ds", duration),
		"tokens":            len(tokens),
	})

	metrics := NewMetricsCollector(1000)
	metrics.Start()

	startTime := time.Now()
	var clients []*client.Client
	var clientsConnected, clientsConnectFailed int
	var authsSucceeded, authsFailed int
	var roomsCreated, roomsCreateFailed int
	var joinsSucceeded, joinsFailed int
	var monitorJoinsSucceeded, monitorJoinsFailed int
	var selectChartsSucceeded, selectChartsFailed int
	var requestStartsSucceeded, requestStartsFailed int
	var readiesSucceeded, readiesFailed int
	setupErrors := make(map[string]int)

	recordSetupError := func(stage string, err error) {
		key := fmt.Sprintf("%s: %s", stage, err.Error())
		setupErrors[key]++
	}

	intervalMs := 1000.0 / float64(rate)

	// Phase 1: Connect
	for i := 0; i < totalClients; i++ {
		expectedDelay := float64(i) * intervalMs
		elapsed := time.Since(startTime).Milliseconds()
		wait := int(expectedDelay) - int(elapsed)
		if wait > 0 {
			time.Sleep(time.Duration(wait) * time.Millisecond)
		}
		printProgress("Connecting", i+1, totalClients, "")

		c, err := client.Connect(host, port, nil)
		if err != nil {
			clientsConnectFailed++
			recordSetupError("connect", err)
			continue
		}
		clientsConnected++

		token := tokens[len(clients)%len(tokens)]
		if token != "" {
			if err := c.Authenticate(token); err != nil {
				authsFailed++
				recordSetupError("auth", err)
			} else {
				authsSucceeded++
			}
		}
		clients = append(clients, c)
	}

	clearProgress()
	fmt.Printf("Connected %d/%d clients (auth OK: %d).\n", len(clients), totalClients, authsSucceeded)

	if len(clients) == 0 {
		fmt.Println("No clients connected. Aborting.")
		os.Exit(1)
	}

	// Phase 2: Assign and create rooms
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
		roomID := fmt.Sprintf("benchgp%d_%d", r, time.Now().UnixMilli())
		roomAssignments = append(roomAssignments, roomAssignment{host: hostClient, players: players, monitors: monitors, roomID: roomID})
	}

	for _, room := range roomAssignments {
		if err := room.host.CreateRoom(room.roomID); err != nil {
			roomsCreateFailed++
			recordSetupError("createRoom", err)
		} else {
			roomsCreated++
		}
	}
	fmt.Printf("Created %d/%d rooms.\n", roomsCreated, len(roomAssignments))

	// Phase 3: Join
	for _, room := range roomAssignments {
		roomID, ok := room.host.RoomID()
		if !ok {
			continue
		}
		for _, p := range room.players {
			if _, err := p.JoinRoom(roomID.String(), false); err != nil {
				joinsFailed++
				recordSetupError("joinRoom", err)
			} else {
				joinsSucceeded++
			}
		}
		for _, m := range room.monitors {
			if _, err := m.JoinRoom(roomID.String(), true); err != nil {
				monitorJoinsFailed++
				recordSetupError("monitorJoin", err)
			} else {
				monitorJoinsSucceeded++
			}
		}
	}
	fmt.Printf("Joined %d/%d players, %d/%d monitors.\n",
		joinsSucceeded, joinsSucceeded+joinsFailed,
		monitorJoinsSucceeded, monitorJoinsSucceeded+monitorJoinsFailed)

	// Phase 4: Gameplay setup (selectChart -> requestStart -> ready)
	var wgSetup sync.WaitGroup
	for _, room := range roomAssignments {
		room := room
		wgSetup.Add(1)
		go func() {
			defer wgSetup.Done()
			if _, ok := room.host.RoomID(); !ok {
				return
			}
			if err := room.host.SelectChart(1); err != nil {
				selectChartsFailed++
				recordSetupError("selectChart", err)
				return
			}
			selectChartsSucceeded++

			if err := room.host.RequestStart(); err != nil {
				requestStartsFailed++
				recordSetupError("requestStart", err)
				return
			}
			requestStartsSucceeded++

			for _, p := range room.players {
				if err := p.Ready(); err != nil {
					readiesFailed++
					recordSetupError("ready", err)
				} else {
					readiesSucceeded++
				}
			}
		}()
	}
	wgSetup.Wait()
	fmt.Printf("Gameplay setup: selectChart=%d/%d, requestStart=%d/%d, ready=%d/%d\n",
		selectChartsSucceeded, selectChartsSucceeded+selectChartsFailed,
		requestStartsSucceeded, requestStartsSucceeded+requestStartsFailed,
		readiesSucceeded, readiesSucceeded+readiesFailed)

	time.Sleep(500 * time.Millisecond)

	// Phase 5: High-frequency load
	sendInterval := time.Second / time.Duration(hz)
	endTime := startTime.Add(time.Duration(duration) * time.Second)
	var messagesAttempted, messagesSent, messagesFailed int
	sendLatencies := []int{}
	sendErrors := make(map[string]int)

	var allSenders []*client.Client
	for _, room := range roomAssignments {
		if _, ok := room.host.RoomID(); !ok {
			continue
		}
		allSenders = append(allSenders, room.host)
		allSenders = append(allSenders, room.players...)
	}

	if len(allSenders) == 0 {
		fmt.Println("[CRITICAL] No senders available for gameplay load test.")
		fmt.Printf("Setup: connected=%d/%d, rooms=%d/%d, joins=%d/%d\n",
			clientsConnected, totalClients, roomsCreated, len(roomAssignments), joinsSucceeded, joinsSucceeded+joinsFailed)
		os.Exit(1)
	}

	fmt.Printf("Starting gameplay load: %d clients x %d msg/s for %ds\n", len(allSenders), hz, duration)

	stopCh := make(chan struct{})
	for _, sender := range allSenders {
		sender := sender
		go func() {
			ticker := time.NewTicker(sendInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					if time.Now().After(endTime) {
						return
					}
					t := float32(time.Now().UnixMilli()) / 1000.0
					messagesAttempted += 2

					sendStart := time.Now()
					if err := sender.SendTouches([]protocol.TouchFrame{makeTouchFrame(t)}); err != nil {
						messagesFailed++
						sendErrors[err.Error()]++
					} else {
						messagesSent++
					}
					if err := sender.SendJudges([]protocol.JudgeEvent{makeJudgeEvent(t)}); err != nil {
						messagesFailed++
						sendErrors[err.Error()]++
					} else {
						messagesSent++
					}
					sendLatencies = append(sendLatencies, int(time.Since(sendStart).Milliseconds()))
				case <-stopCh:
					return
				}
			}
		}()
	}

	// Wait for duration
	for time.Now().Before(endTime) {
		left := int(time.Until(endTime).Seconds())
		elapsed := duration - left
		printProgress("Sending", elapsed, duration, fmt.Sprintf("| Attempted: %d", messagesAttempted))
		time.Sleep(minDuration(time.Second, time.Until(endTime)))
	}
	clearProgress()
	close(stopCh)

	fmt.Println("Leaving rooms and closing connections...")
	for _, c := range clients {
		_ = c.LeaveRoom()
		_ = c.Close()
	}

	endedAt := time.Now()
	metricsSamples := metrics.Stop()
	metricsSummary := SummarizeMetrics(metricsSamples)

	actualDurationSec := endedAt.Sub(startTime).Seconds()
	msgsPerSec := 0.0
	if actualDurationSec > 0 {
		msgsPerSec = float64(messagesSent) / actualDurationSec
	}

	sorted := make([]int, len(sendLatencies))
	copy(sorted, sendLatencies)
	sort.Ints(sorted)
	avgSendLatency := avgInt(sendLatencies)
	p95SendLatency := percentileInt(sorted, 0.95)
	p99SendLatency := percentileInt(sorted, 0.99)

	allErrors := make(map[string]int)
	for k, v := range setupErrors {
		allErrors[k] = v
	}
	for k, v := range sendErrors {
		allErrors[k] = v
	}

	report := &BenchReport{
		BenchType: "gameplay-bench",
		Params: map[string]interface{}{
			"host":              host,
			"port":              port,
			"rooms":             rooms,
			"playersPerRoom":    playersPerRoom,
			"monitorsPerRoom":   monitorsPerRoom,
			"hz":                hz,
			"duration":          duration,
		},
		StartedAt:      startTime.UTC().Format(time.RFC3339),
		EndedAt:        endedAt.UTC().Format(time.RFC3339),
		Duration:       int(actualDurationSec),
		MetricsSamples: metricsSamples,
		MetricsSummary: metricsSummary,
		Summary: map[string]interface{}{
			"mode":                  "real-gameplay",
			"clients":               len(clients),
			"clientsConnected":      clientsConnected,
			"clientsConnectFailed":  clientsConnectFailed,
			"authsSucceeded":        authsSucceeded,
			"authsFailed":           authsFailed,
			"roomsCreated":          roomsCreated,
			"roomsCreateFailed":     roomsCreateFailed,
			"joinsSucceeded":        joinsSucceeded,
			"joinsFailed":           joinsFailed,
			"monitorJoinsSucceeded": monitorJoinsSucceeded,
			"monitorJoinsFailed":    monitorJoinsFailed,
			"selectChartsSucceeded": selectChartsSucceeded,
			"selectChartsFailed":    selectChartsFailed,
			"requestStartsSucceeded": requestStartsSucceeded,
			"requestStartsFailed":   requestStartsFailed,
			"readiesSucceeded":      readiesSucceeded,
			"readiesFailed":         readiesFailed,
			"rooms":                 rooms,
			"playersPerRoom":        playersPerRoom,
			"messagesAttempted":     messagesAttempted,
			"messagesSent":          messagesSent,
			"messagesFailed":        messagesFailed,
			"messagesPerSecond":     fmt.Sprintf("%.2f", msgsPerSec),
			"avgSendLatency":        fmt.Sprintf("%.2f ms", avgSendLatency),
			"p95SendLatency":        fmt.Sprintf("%.2f ms", p95SendLatency),
			"p99SendLatency":        fmt.Sprintf("%.2f ms", p99SendLatency),
			"duration":              fmt.Sprintf("%ds", duration),
		},
		Errors: mapToBenchErrors(allErrors),
	}

	filepath := saveReport(report)
	fmt.Printf("Report saved to: %s\n", filepath)
	printBenchFooter(report, metricsSummary)
}

func makeTouchFrame(t float32) protocol.TouchFrame {
	return protocol.TouchFrame{
		Time: t,
		Points: []protocol.TouchPoint{
			{ID: 0, Pos: protocol.CompactPos{X: rand.Float32(), Y: rand.Float32()}},
		},
	}
}

func makeJudgeEvent(t float32) protocol.JudgeEvent {
	return protocol.JudgeEvent{
		Time:      t,
		LineID:    uint32(rand.Intn(4)),
		NoteID:    uint32(rand.Intn(100)),
		Judgement: protocol.Judgement(rand.Intn(6)),
	}
}

func percentileInt(sorted []int, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return float64(sorted[idx])
}


