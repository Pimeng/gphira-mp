package main

import (
	"runtime"
	"time"
)

type MetricsSample struct {
	Timestamp      int64 `json:"timestamp"`
	RSS            int64 `json:"rss"`
	HeapUsed       int64 `json:"heapUsed"`
	HeapTotal      int64 `json:"heapTotal"`
	HeapObjects    int64 `json:"heapObjects"`
	Goroutines     int   `json:"goroutines"`
	GCNum          uint32 `json:"gcNum"`
	GCTimeMs       float64 `json:"gcTimeMs"`
}

type MetricsSummary struct {
	RSSAvg      int64 `json:"rssAvg"`
	RSSPeak     int64 `json:"rssPeak"`
	HeapUsedAvg int64 `json:"heapUsedAvg"`
	HeapUsedPeak int64 `json:"heapUsedPeak"`
	GoroutinesAvg int64 `json:"goroutinesAvg"`
	GoroutinesPeak int64 `json:"goroutinesPeak"`
}

type MetricsCollector struct {
	interval time.Duration
	samples  []MetricsSample
	timer    *time.Timer
	running  bool
}

func NewMetricsCollector(intervalMs int) *MetricsCollector {
	return &MetricsCollector{interval: time.Duration(intervalMs) * time.Millisecond}
}

func (m *MetricsCollector) Start() {
	if m.running {
		return
	}
	m.running = true
	m.sample()
	m.schedule()
}

func (m *MetricsCollector) schedule() {
	m.timer = time.AfterFunc(m.interval, func() {
		if !m.running {
			return
		}
		m.sample()
		m.schedule()
	})
}

func (m *MetricsCollector) sample() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.samples = append(m.samples, MetricsSample{
		Timestamp:   time.Now().UnixMilli(),
		RSS:         int64(ms.Sys),
		HeapUsed:    int64(ms.HeapAlloc),
		HeapTotal:   int64(ms.HeapSys),
		HeapObjects: int64(ms.HeapObjects),
		Goroutines:  runtime.NumGoroutine(),
		GCNum:       ms.NumGC,
		GCTimeMs:    float64(ms.PauseTotalNs) / 1e6,
	})
}

func (m *MetricsCollector) Stop() []MetricsSample {
	m.running = false
	if m.timer != nil {
		m.timer.Stop()
	}
	return m.samples
}

func SummarizeMetrics(samples []MetricsSample) MetricsSummary {
	if len(samples) == 0 {
		return MetricsSummary{}
	}
	var rssSum, heapSum, goroutineSum int64
	var rssPeak, heapPeak, goroutinePeak int64
	for _, s := range samples {
		rssSum += s.RSS
		heapSum += s.HeapUsed
		goroutineSum += int64(s.Goroutines)
		if s.RSS > rssPeak {
			rssPeak = s.RSS
		}
		if s.HeapUsed > heapPeak {
			heapPeak = s.HeapUsed
		}
		if int64(s.Goroutines) > goroutinePeak {
			goroutinePeak = int64(s.Goroutines)
		}
	}
	n := int64(len(samples))
	return MetricsSummary{
		RSSAvg:         rssSum / n,
		RSSPeak:        rssPeak,
		HeapUsedAvg:    heapSum / n,
		HeapUsedPeak:   heapPeak,
		GoroutinesAvg:  goroutineSum / n,
		GoroutinesPeak: goroutinePeak,
	}
}
