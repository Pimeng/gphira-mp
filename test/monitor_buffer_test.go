package test

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/network"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

type monitorBroadcastCall struct {
	player int32
	frames int
	judges int
	ids    []int32
}

func TestMonitorBufferDoesNotMergeDifferentMonitorSets(t *testing.T) {
	var mu sync.Mutex
	calls := make([]monitorBroadcastCall, 0, 2)

	buffer := network.NewMonitorBuffer(func(player int32, frames []protocol.TouchFrame, judges []protocol.JudgeEvent, ids []int32) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, monitorBroadcastCall{
			player: player,
			frames: len(frames),
			judges: len(judges),
			ids:    append([]int32(nil), ids...),
		})
	})

	buffer.BufferTouches(1, []protocol.TouchFrame{{}}, []int32{1, 3})
	buffer.BufferTouches(1, []protocol.TouchFrame{{}}, []int32{1, 2, 3})
	buffer.Flush()

	mu.Lock()
	defer mu.Unlock()

	if len(calls) != 2 {
		t.Fatalf("broadcast count = %d, want 2", len(calls))
	}

	keys := map[string]struct{}{}
	for _, c := range calls {
		keys[idsKey(c.player, c.ids)] = struct{}{}
		if c.frames != 1 {
			t.Fatalf("unexpected merged frame count = %d, want 1", c.frames)
		}
	}

	if _, ok := keys[idsKey(1, []int32{1, 3})]; !ok {
		t.Fatal("missing broadcast for ids [1,3]")
	}
	if _, ok := keys[idsKey(1, []int32{1, 2, 3})]; !ok {
		t.Fatal("missing broadcast for ids [1,2,3]")
	}
}

func idsKey(player int32, ids []int32) string {
	copied := append([]int32(nil), ids...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })

	parts := make([]string, len(copied))
	for i, id := range copied {
		parts[i] = strconv.FormatInt(int64(id), 10)
	}
	return strconv.FormatInt(int64(player), 10) + ":" + strings.Join(parts, ",")
}
