package network

import (
	"fmt"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

const monitorFlushIntervalMs = 50

// MonitorBuffer aggregates touches/judges for monitors.
type MonitorBuffer struct {
	mu           sync.Mutex
	touchBuffer  []touchEntry
	judgeBuffer  []judgeEntry
	timer        *time.Timer
	broadcastFast func(player int32, frames []protocol.TouchFrame, judges []protocol.JudgeEvent, ids []int32)
}

type touchEntry struct {
	player int32
	frames []protocol.TouchFrame
	ids    []int32
}

type judgeEntry struct {
	player int32
	judges []protocol.JudgeEvent
	ids    []int32
}

// NewMonitorBuffer creates a new monitor buffer.
func NewMonitorBuffer(broadcastFast func(player int32, frames []protocol.TouchFrame, judges []protocol.JudgeEvent, ids []int32)) *MonitorBuffer {
	return &MonitorBuffer{broadcastFast: broadcastFast}
}

// BufferTouches queues touch frames for broadcast to monitors.
func (b *MonitorBuffer) BufferTouches(player int32, frames []protocol.TouchFrame, ids []int32) {
	if len(ids) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.touchBuffer = append(b.touchBuffer, touchEntry{player: player, frames: frames, ids: ids})
	b.scheduleFlush()
}

// BufferJudges queues judge events for broadcast to monitors.
func (b *MonitorBuffer) BufferJudges(player int32, judges []protocol.JudgeEvent, ids []int32) {
	if len(ids) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.judgeBuffer = append(b.judgeBuffer, judgeEntry{player: player, judges: judges, ids: ids})
	b.scheduleFlush()
}

// Flush immediately broadcasts aggregated data.
func (b *MonitorBuffer) Flush() {
	b.mu.Lock()
	touches := b.touchBuffer
	judges := b.judgeBuffer
	b.touchBuffer = nil
	b.judgeBuffer = nil
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	b.mu.Unlock()

	// Merge touches by (player, ids)
	touchMap := make(map[string]struct {
		player int32
		frames []protocol.TouchFrame
		ids    []int32
	})
	for _, t := range touches {
		key := monitorKey(t.player, t.ids)
		entry := touchMap[key]
		entry.player = t.player
		entry.frames = append(entry.frames, t.frames...)
		entry.ids = t.ids
		touchMap[key] = entry
	}
	for _, entry := range touchMap {
		b.broadcastFast(entry.player, entry.frames, nil, entry.ids)
	}

	// Merge judges by (player, ids)
	judgeMap := make(map[string]struct {
		player int32
		judges []protocol.JudgeEvent
		ids    []int32
	})
	for _, j := range judges {
		key := monitorKey(j.player, j.ids)
		entry := judgeMap[key]
		entry.player = j.player
		entry.judges = append(entry.judges, j.judges...)
		entry.ids = j.ids
		judgeMap[key] = entry
	}
	for _, entry := range judgeMap {
		b.broadcastFast(entry.player, nil, entry.judges, entry.ids)
	}
}

// Destroy flushes pending data and stops the timer.
func (b *MonitorBuffer) Destroy() {
	b.Flush()
}

func (b *MonitorBuffer) scheduleFlush() {
	if b.timer == nil {
		b.timer = time.AfterFunc(monitorFlushIntervalMs*time.Millisecond, func() {
			b.mu.Lock()
			b.timer = nil
			b.mu.Unlock()
			b.Flush()
		})
	}
}

func monitorKey(player int32, ids []int32) string {
	// Simple key: player:len:hash of ids
	// For correctness, we sort ids and concatenate
	// Simplified: just use player and first/last id
	if len(ids) == 0 {
		return fmt.Sprintf("%d:", player)
	}
	return fmt.Sprintf("%d:%d-%d", player, ids[0], ids[len(ids)-1])
}
