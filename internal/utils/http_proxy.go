package utils

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"
)

// ProxyConfig holds parsed proxy settings.
type ProxyConfig struct {
	Scheme   string
	Host     string
	Username string
	Password string
}

// ParseProxyURL parses a proxy URL string.
// Supported schemes: http, https, socks, socks4, socks5.
func ParseProxyURL(proxyURL string) (*ProxyConfig, error) {
	if proxyURL == "" {
		return nil, fmt.Errorf("empty proxy URL")
	}

	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "http", "https", "socks", "socks4", "socks5":
		// supported
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s", scheme)
	}

	cfg := &ProxyConfig{
		Scheme: scheme,
		Host:   u.Host,
	}
	if u.User != nil {
		cfg.Username = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}
	return cfg, nil
}

// NewHTTPClient returns an *http.Client that routes requests through the given proxy.
//
//   - proxy == "" or proxy == "false" → direct connection (ignores HTTP_PROXY env).
//   - proxy starts with "http://" or "https://" → HTTP proxy via http.Transport.Proxy.
//   - proxy starts with "socks://", "socks4://", or "socks5://" → SOCKS proxy.
func NewHTTPClient(proxy string, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	trimmed := strings.TrimSpace(proxy)
	if trimmed == "" || strings.EqualFold(trimmed, "false") {
		// Explicit direct connection; use a transport that ignores HTTP_PROXY.
		return &http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{Proxy: nil},
		}
	}

	cfg, err := ParseProxyURL(trimmed)
	if err != nil {
		// Fallback: ignore proxy on parse error to avoid hard failures.
		return &http.Client{Timeout: timeout}
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           nil,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	switch cfg.Scheme {
	case "http", "https":
		proxyURL, _ := url.Parse(trimmed)
		transport.Proxy = http.ProxyURL(proxyURL)

	case "socks", "socks5":
		var auth *xproxy.Auth
		if cfg.Username != "" {
			auth = &xproxy.Auth{User: cfg.Username, Password: cfg.Password}
		}
		dialer, err := xproxy.SOCKS5("tcp", cfg.Host, auth, xproxy.Direct)
		if err != nil {
			return &http.Client{Timeout: timeout}
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}

	case "socks4":
		// golang.org/x/net/proxy does not have a built-in SOCKS4 dialer,
		// but SOCKS5 dialer with no auth often works for SOCKS4 proxies.
		// Fallback to a best-effort approach using the SOCKS5 package.
		dialer, err := xproxy.SOCKS5("tcp", cfg.Host, nil, xproxy.Direct)
		if err != nil {
			return &http.Client{Timeout: timeout}
		}
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
