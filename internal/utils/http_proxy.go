package utils

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
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

	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("invalid proxy host")
	}
	port := u.Port()
	if port == "" {
		port = defaultProxyPort(scheme)
	}
	if p, err := strconv.Atoi(port); err != nil || p <= 0 || p > 65535 {
		return nil, fmt.Errorf("invalid proxy port")
	}

	cfg := &ProxyConfig{
		Scheme: scheme,
		Host:   net.JoinHostPort(host, port),
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
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return socks4DialContext(ctx, cfg.Host, cfg.Username, network, addr)
		}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func defaultProxyPort(scheme string) string {
	switch scheme {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return "1080"
	}
}

func socks4DialContext(ctx context.Context, proxyAddr, username, network, targetAddr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("unsupported socks4 network: %s", network)
	}
	targetHost, targetPortText, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, err
	}
	targetPort, err := strconv.Atoi(targetPortText)
	if err != nil || targetPort <= 0 || targetPort > 65535 {
		return nil, fmt.Errorf("invalid target port: %s", targetPortText)
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	defer conn.SetDeadline(time.Time{})

	ip := net.ParseIP(targetHost).To4()
	hostBytes := []byte{0, 0, 0, 1}
	var domain []byte
	if ip != nil {
		hostBytes = []byte(ip)
	} else {
		domain = []byte(targetHost)
	}

	req := make([]byte, 0, 9+len(username)+len(domain))
	req = append(req, 0x04, 0x01)
	var portBuf [2]byte
	binary.BigEndian.PutUint16(portBuf[:], uint16(targetPort))
	req = append(req, portBuf[:]...)
	req = append(req, hostBytes...)
	req = append(req, []byte(username)...)
	req = append(req, 0x00)
	if len(domain) > 0 {
		req = append(req, domain...)
		req = append(req, 0x00)
	}
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	var resp [8]byte
	if _, err := io.ReadFull(conn, resp[:]); err != nil {
		return nil, err
	}
	if resp[1] != 0x5a {
		return nil, fmt.Errorf("socks4 connect failed: %d", resp[1])
	}
	success = true
	return conn, nil
}
