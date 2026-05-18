package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/client"
)

type connectArgs struct {
	globalArgs
	clients  int
	rate     int
	duration int
	token    string
}

func runConnect(argv []string) {
	var args connectArgs
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	parseGlobalArgs(fs, &args.globalArgs)
	fs.IntVar(&args.clients, "clients", 10, "Target concurrent clients")
	fs.IntVar(&args.rate, "rate", 10, "New connections per second")
	fs.IntVar(&args.duration, "duration", 30, "Benchmark duration in seconds")
	fs.StringVar(&args.token, "token", "", "Auth token (or set BENCH_TOKEN env)")
	_ = fs.Parse(argv)

	if args.token == "" {
		args.token = os.Getenv("BENCH_TOKEN")
	}

	printBenchHeader("Connect Benchmark", map[string]interface{}{
		"host":     args.host,
		"port":     args.port,
		"clients":  args.clients,
		"rate":     fmt.Sprintf("%d/s", args.rate),
		"duration": fmt.Sprintf("%ds", args.duration),
		"token":    boolStr(args.token != ""),
	})

	metrics := NewMetricsCollector(1000)
	metrics.Start()

	var results []connectResult
	var clients []*client.Client
	startTime := time.Now()
	intervalMs := 1000.0 / float64(args.rate)

	for i := 0; i < args.clients; i++ {
		expectedDelay := float64(i) * intervalMs
		elapsed := time.Since(startTime).Milliseconds()
		wait := int(expectedDelay) - int(elapsed)
		if wait > 0 {
			time.Sleep(time.Duration(wait) * time.Millisecond)
		}

		printProgress("Connecting", i+1, args.clients, "")

		connectStart := time.Now()
		var c *client.Client
		var err error

		c, err = client.Connect(args.host, args.port, nil)
		connectLatency := int(time.Since(connectStart).Milliseconds())

		if err != nil {
			results = append(results, connectResult{
				connected:        false,
				connectLatencyMs: connectLatency,
				err:              err.Error(),
			})
			if c != nil {
				_ = c.Close()
			}
			continue
		}

		var authLatency int
		authenticated := false
		if args.token != "" {
			authStart := time.Now()
			if err := c.Authenticate(args.token); err != nil {
				authLatency = int(time.Since(authStart).Milliseconds())
				authenticated = false
			} else {
				authLatency = int(time.Since(authStart).Milliseconds())
				authenticated = true
			}
		}

		results = append(results, connectResult{
			connected:        true,
			connectLatencyMs: connectLatency,
			authenticated:    authenticated,
			authLatencyMs:    authLatency,
		})
		clients = append(clients, c)
	}

	clearProgress()
	connected := countConnected(results)
	fmt.Printf("Launched %d connections. Connected: %d. Keeping for %ds...\n", args.clients, connected, args.duration)

	remaining := args.duration*1000 - int(time.Since(startTime).Milliseconds())
	if remaining > 0 {
		endAt := time.Now().Add(time.Duration(remaining) * time.Millisecond)
		for time.Now().Before(endAt) {
			left := int(time.Until(endAt).Seconds())
			elapsed := args.duration - left
			printProgress("Running", elapsed, args.duration, fmt.Sprintf("| Active: %d", len(clients)))
			time.Sleep(minDuration(time.Second, time.Until(endAt)))
		}
		clearProgress()
	}

	fmt.Println("Closing connections...")
	for _, c := range clients {
		_ = c.Close()
	}

	endedAt := time.Now()
	metricsSamples := metrics.Stop()
	metricsSummary := SummarizeMetrics(metricsSamples)

	errors := summarizeErrors(results)
	connectLatencies := filterLatencies(results, func(r connectResult) bool { return r.connected })
	authLatencies := filterLatencies(results, func(r connectResult) bool { return r.authenticated })

	report := &BenchReport{
		BenchType: "connect-bench",
		Params: map[string]interface{}{
			"host":     args.host,
			"port":     args.port,
			"clients":  args.clients,
			"rate":     args.rate,
			"duration": args.duration,
		},
		StartedAt:      startTime.UTC().Format(time.RFC3339),
		EndedAt:        endedAt.UTC().Format(time.RFC3339),
		Duration:       int(endedAt.Sub(startTime).Seconds()),
		MetricsSamples: metricsSamples,
		MetricsSummary: metricsSummary,
		Summary: map[string]interface{}{
			"targetClients":      args.clients,
			"connected":          countConnected(results),
			"connectFailed":      countConnectFailed(results),
			"authenticated":      countAuthenticated(results),
			"authFailed":         countAuthFailed(results, args.token),
			"avgConnectLatency":  fmt.Sprintf("%.2f ms", avg(connectLatencies)),
			"avgAuthLatency":     fmt.Sprintf("%.2f ms", avg(authLatencies)),
			"duration":           fmt.Sprintf("%ds", args.duration),
		},
		Errors: errors,
	}

	filepath := saveReport(report)
	fmt.Printf("Report saved to: %s\n", filepath)
	printBenchFooter(report, metricsSummary)
}

type connectResult struct {
	connected        bool
	connectLatencyMs int
	authenticated    bool
	authLatencyMs    int
	err              string
}

func countConnected(results []connectResult) int {
	c := 0
	for _, r := range results {
		if r.connected {
			c++
		}
	}
	return c
}

func countConnectFailed(results []connectResult) int {
	c := 0
	for _, r := range results {
		if !r.connected {
			c++
		}
	}
	return c
}

func countAuthenticated(results []connectResult) int {
	c := 0
	for _, r := range results {
		if r.authenticated {
			c++
		}
	}
	return c
}

func countAuthFailed(results []connectResult, token string) int {
	if token == "" {
		return 0
	}
	c := 0
	for _, r := range results {
		if r.connected && !r.authenticated {
			c++
		}
	}
	return c
}

func filterLatencies(results []connectResult, predicate func(connectResult) bool) []int {
	var out []int
	for _, r := range results {
		if predicate(r) {
			out = append(out, r.connectLatencyMs)
		}
	}
	return out
}

func summarizeErrors(results []connectResult) []BenchError {
	m := make(map[string]int)
	for _, r := range results {
		if r.err != "" {
			m[r.err]++
		}
	}
	var out []BenchError
	for msg, count := range m {
		out = append(out, BenchError{Message: msg, Count: count})
	}
	return out
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func avg(arr []int) float64 {
	if len(arr) == 0 {
		return 0
	}
	sum := 0
	for _, v := range arr {
		sum += v
	}
	return float64(sum) / float64(len(arr))
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
