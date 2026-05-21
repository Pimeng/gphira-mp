package utils

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoWithRetry_SuccessAfterFiveHundreds(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := DoWithRetry(client, req, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 calls, got %d", got)
	}
}

func TestDoWithRetry_FourHundredsNoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := DoWithRetry(client, req, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected 1 call (no retry on 4xx), got %d", got)
	}
}

func TestDoWithRetry_TooManyRequestsRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := DoWithRetry(client, req, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected 2 calls (1 retry on 429), got %d", got)
	}
}

func TestDoWithRetry_ExhaustsOnPersistentFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := DoWithRetry(client, req, 2)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatalf("expected error after exhausting retries")
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected 3 calls (initial + 2 retries), got %d", got)
	}
}

func TestIsRetryableError_KeywordMatching(t *testing.T) {
	cases := []struct {
		msg     string
		retryOK bool
	}{
		{"connection reset by peer", true},
		{"socket hang up", true},
		{"no such host", true},
		{"i/o timeout", true},
		{"context canceled", false},
		{"bad request", false},
	}
	for _, c := range cases {
		got := isRetryableError(&simpleErr{c.msg})
		if got != c.retryOK {
			t.Errorf("isRetryableError(%q) = %v, want %v", c.msg, got, c.retryOK)
		}
	}
}

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
