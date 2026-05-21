package utils

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

// DefaultMaxRetries is the default number of retries for DoWithRetry when
// the caller passes a non-positive value.
const DefaultMaxRetries = 5

// retryableErrorKeywords mirrors TS httpClient.ts retryable substrings.
var retryableErrorKeywords = []string{
	"connection reset",
	"connection refused",
	"socket hang up",
	"network error",
	"no such host",
	"i/o timeout",
	"timeout",
	"aborted",
	"eof",
}

// DoWithRetry sends an HTTP request with exponential backoff retries.
//
// Retries are triggered when:
//   - The response status is 429 or 5xx (response is closed before retrying).
//   - The transport returns a retryable network error (timeout, reset, EOF,
//     DNS failure, refused, or a matching message substring).
//
// 4xx responses (other than 429) are returned to the caller as-is. The caller
// must close the returned response body.
//
// Backoff uses base = min(100 * 2^attempt, 5000) ms with ±20% jitter, matching
// fetchWithRetry in tphira-mp-main/src/common/httpClient.ts.
//
// If req.GetBody is set, the body is re-read before each retry; otherwise the
// caller is responsible for ensuring the body is reusable (GET requests are
// always safe).
func DoWithRetry(client *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("rewind request body: %w", err)
			}
			req.Body = body
		}

		resp, err := client.Do(req)
		if err == nil {
			if !isRetryableStatus(resp.StatusCode) {
				return resp, nil
			}
			lastErr = fmt.Errorf("http-%d", resp.StatusCode)
			_ = resp.Body.Close()
		} else {
			if !isRetryableError(err) {
				return nil, err
			}
			lastErr = err
		}

		if attempt < maxRetries {
			sleepBackoff(attempt)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("retry exhausted")
	}
	return nil, lastErr
}

func isRetryableStatus(status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500 && status < 600
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNABORTED) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, kw := range retryableErrorKeywords {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

func sleepBackoff(attempt int) {
	base := math.Min(100*math.Pow(2, float64(attempt)), 5000)
	jitter := (rand.Float64()*0.4 - 0.2) * base
	delay := time.Duration(base+jitter) * time.Millisecond
	if delay < 0 {
		delay = time.Duration(base) * time.Millisecond
	}
	time.Sleep(delay)
}
