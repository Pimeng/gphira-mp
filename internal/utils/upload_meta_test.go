package utils

import "testing"

func TestUploadedReplayMeta_LimitsToFiftyPerChart(t *testing.T) {
	m := NewUploadedReplayMeta()
	const userID int32 = 1
	const chartID int32 = 42

	for i := 0; i < 60; i++ {
		m.Add(userID, chartID, int32(1000+i), int64(2_000_000+i))
	}

	got := m.Get(userID, chartID)
	if len(got) != 50 {
		t.Fatalf("expected 50 entries after 60 adds, got %d", len(got))
	}
	if got[0].Timestamp != 2_000_010 {
		t.Errorf("expected oldest retained timestamp 2000010 (entries 10..59 kept), got %d", got[0].Timestamp)
	}
	if got[49].Timestamp != 2_000_059 {
		t.Errorf("expected newest timestamp 2000059, got %d", got[49].Timestamp)
	}
}

func TestUploadedReplayMeta_DoesNotAffectOtherCharts(t *testing.T) {
	m := NewUploadedReplayMeta()
	const userID int32 = 7
	for i := 0; i < 55; i++ {
		m.Add(userID, 1, int32(i), int64(i))
	}
	m.Add(userID, 2, 999, 12345)

	if got := len(m.Get(userID, 1)); got != 50 {
		t.Errorf("chart 1 should be trimmed to 50, got %d", got)
	}
	if got := len(m.Get(userID, 2)); got != 1 {
		t.Errorf("chart 2 should not be trimmed, got %d", got)
	}
}

func TestUploadedReplayMeta_DeleteRemovesEntry(t *testing.T) {
	m := NewUploadedReplayMeta()
	m.Add(1, 2, 100, 500)
	m.Add(1, 2, 101, 600)
	if !m.Delete(1, 2, 500) {
		t.Fatal("Delete should return true on hit")
	}
	got := m.Get(1, 2)
	if len(got) != 1 || got[0].Timestamp != 600 {
		t.Errorf("unexpected state after delete: %+v", got)
	}
	if m.Delete(1, 2, 9999) {
		t.Error("Delete should return false on miss")
	}
}
