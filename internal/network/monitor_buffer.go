package network

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/internal/game"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

const monitorFlushIntervalMs = 50

// MonitorBuffer aggregates touches/judges for monitors.
type MonitorBuffer struct {
	mu            sync.Mutex
	touchBuffer   []touchEntry
	judgeBuffer   []judgeEntry
	timer         *time.Timer
	broadcastFast func(player int32, frames []protocol.TouchFrame, judges []protocol.JudgeEvent, ids []int32)
}

type touchEntry struct {
	player int32
	frames []protocol.TouchFrame
	ids    []int32
	room   *game.Room
}

type judgeEntry struct {
	player int32
	judges []protocol.JudgeEvent
	ids    []int32
	room   *game.Room
}

// NewMonitorBuffer creates a new monitor buffer.
func NewMonitorBuffer(broadcastFast func(player int32, frames []protocol.TouchFrame, judges []protocol.JudgeEvent, ids []int32)) *MonitorBuffer {
	return &MonitorBuffer{broadcastFast: broadcastFast}
}

// BufferTouches queues touch frames for broadcast to monitors.
func (b *MonitorBuffer) BufferTouches(player int32, frames []protocol.TouchFrame, ids []int32, room *game.Room) {
	if len(ids) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.touchBuffer = append(b.touchBuffer, touchEntry{player: player, frames: frames, ids: ids, room: room})
	b.scheduleFlush()
}

// BufferJudges queues judge events for broadcast to monitors.
func (b *MonitorBuffer) BufferJudges(player int32, judges []protocol.JudgeEvent, ids []int32, room *game.Room) {
	if len(ids) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.judgeBuffer = append(b.judgeBuffer, judgeEntry{player: player, judges: judges, ids: ids, room: room})
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

	// Merge touches by (player, ids), filtering monitors that are still in the room.
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
		entry.ids = filterLiveMonitors(t.ids, t.room)
		touchMap[key] = entry
	}
	for _, entry := range touchMap {
		if len(entry.ids) > 0 {
			b.broadcastFast(entry.player, entry.frames, nil, entry.ids)
		}
	}

	// Merge judges by (player, ids), filtering monitors that are still in the room.
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
		entry.ids = filterLiveMonitors(j.ids, j.room)
		judgeMap[key] = entry
	}
	for _, entry := range judgeMap {
		if len(entry.ids) > 0 {
			b.broadcastFast(entry.player, nil, entry.judges, entry.ids)
		}
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

func filterLiveMonitors(ids []int32, room *game.Room) []int32 {
	if room == nil {
		return ids
	}
	live := make([]int32, 0, len(ids))
	roomMonitors := room.MonitorIDs()
	for _, id := range ids {
		for _, mid := range roomMonitors {
			if mid == id {
				live = append(live, id)
				break
			}
		}
	}
	return live
}

func monitorKey(player int32, ids []int32) string {
	if len(ids) == 0 {
		return fmt.Sprintf("%d:", player)
	}
	copied := append([]int32(nil), ids...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })

	var b strings.Builder
	b.WriteString(strconv.FormatInt(int64(player), 10))
	b.WriteByte(':')
	for i, id := range copied {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(int64(id), 10))
	}
	return b.String()
}
