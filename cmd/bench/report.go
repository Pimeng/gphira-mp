package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BenchReport struct {
	BenchType      string                 `json:"benchType"`
	Params         map[string]interface{} `json:"params"`
	StartedAt      string                 `json:"startedAt"`
	EndedAt        string                 `json:"endedAt"`
	Duration       int                    `json:"duration"`
	Summary        map[string]interface{} `json:"summary"`
	Errors         []BenchError           `json:"errors"`
	MetricsSamples []MetricsSample        `json:"metricsSamples"`
	MetricsSummary MetricsSummary         `json:"metricsSummary"`
}

type BenchError struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func saveReport(report *BenchReport) string {
	dir := os.Getenv("BENCH_RESULTS_DIR")
	if dir == "" {
		dir = "bench-results"
	}
	_ = os.MkdirAll(dir, 0755)

	ts := time.Now().UTC().Format("2006-01-02T15-04-05")
	filename := fmt.Sprintf("%s-%s.json", report.BenchType, ts)
	path := filepath.Join(dir, filename)

	data, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(path, data, 0644)
	return path
}

func printProgress(label string, current, total int, extra string) {
	pct := 0
	if total > 0 {
		pct = int(float64(current) / float64(total) * 100)
	}
	line := fmt.Sprintf("[%s] %d/%d (%d%%)", label, current, total, pct)
	if extra != "" {
		line += " " + extra
	}
	fmt.Printf("\r%-80s", line)
}

func clearProgress() {
	fmt.Printf("\r%s\r", strings.Repeat(" ", 80))
}

func printBenchHeader(benchType string, params map[string]interface{}) {
	fmt.Printf("\n========== %s ==========\n", benchType)
	for k, v := range params {
		fmt.Printf("  %s: %v\n", k, v)
	}
	fmt.Println()
}

func printBenchFooter(report *BenchReport, metrics MetricsSummary) {
	fmt.Println("\n========== Results ==========")
	for k, v := range report.Summary {
		fmt.Printf("  %s: %v\n", k, v)
	}
	fmt.Println("\n--- Client Process Metrics ---")
	fmt.Printf("  RSS avg/peak:        %s / %s\n", formatBytes(metrics.RSSAvg), formatBytes(metrics.RSSPeak))
	fmt.Printf("  HeapUsed avg/peak:   %s / %s\n", formatBytes(metrics.HeapUsedAvg), formatBytes(metrics.HeapUsedPeak))
	fmt.Printf("  Goroutines avg/peak: %d / %d\n", metrics.GoroutinesAvg, metrics.GoroutinesPeak)
	fmt.Println("=============================")
}

func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}
