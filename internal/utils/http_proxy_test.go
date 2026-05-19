package utils

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestParseProxyURLDefaultsSocksPort(t *testing.T) {
	cfg, err := ParseProxyURL("socks4://user@example.com")
	if err != nil {
		t.Fatalf("parse proxy: %v", err)
	}
	if cfg.Scheme != "socks4" || cfg.Host != "example.com:1080" || cfg.Username != "user" {
		t.Fatalf("unexpected proxy config: %+v", cfg)
	}
}

func TestSocks4DialContextUsesSocks4aForDomains(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		head := make([]byte, 8)
		if _, err := io.ReadFull(conn, head); err != nil {
			errCh <- err
			return
		}
		reader := bufio.NewReader(conn)
		user, err := reader.ReadString(0)
		if err != nil {
			errCh <- err
			return
		}
		domain, err := reader.ReadString(0)
		if err != nil {
			errCh <- err
			return
		}

		if head[0] != 0x04 || head[1] != 0x01 {
			errCh <- errUnexpected("bad socks4 command")
			return
		}
		if port := binary.BigEndian.Uint16(head[2:4]); port != 443 {
			errCh <- errUnexpected("bad socks4 port")
			return
		}
		if got := string(head[4:8]); got != "\x00\x00\x00\x01" {
			errCh <- errUnexpected("bad socks4a host sentinel")
			return
		}
		if user != "alice\x00" || domain != "example.com\x00" {
			errCh <- errUnexpected("bad socks4 user or domain")
			return
		}
		_, _ = conn.Write([]byte{0x00, 0x5a, 0, 0, 0, 0, 0, 0})
		errCh <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := socks4DialContext(ctx, ln.Addr().String(), "alice", "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("socks4 dial: %v", err)
	}
	_ = conn.Close()

	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

type errUnexpected string

func (e errUnexpected) Error() string { return string(e) }
