package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// ProxyInfo holds the parsed HAProxy PROXY Protocol information.
type ProxyInfo struct {
	SourceAddress string
	SourcePort    int
	DestAddress   string
	DestPort      int
	Family        string
}

var proxyV2Signature = []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a}

// ParseProxyProtocol attempts to read a HAProxy PROXY Protocol header from
// the connection. If the header is present, it returns the parsed ProxyInfo and
// a wrapped connection that preserves any read-ahead data. If the header is not
// present, it returns nil and a wrapped connection.
func ParseProxyProtocol(conn net.Conn, timeout time.Duration) (*ProxyInfo, net.Conn, error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, conn, err
	}
	defer conn.SetReadDeadline(time.Time{})

	reader := bufio.NewReader(conn)

	peek, err := reader.Peek(6)
	if err != nil {
		return nil, newBufConn(reader, conn), err
	}

	// Check for v2 signature (12 bytes)
	if len(peek) >= 6 && bytes.Equal(peek[:6], proxyV2Signature[:6]) {
		peek2, err := reader.Peek(12)
		if err != nil {
			return nil, newBufConn(reader, conn), err
		}
		if bytes.Equal(peek2, proxyV2Signature) {
			info, err := parseProxyV2(reader)
			return info, newBufConn(reader, conn), err
		}
	}

	// Check for v1 (starts with "PROXY ")
	if strings.HasPrefix(string(peek), "PROXY ") {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, newBufConn(reader, conn), err
		}
		info := parseProxyV1(strings.TrimRight(line, "\r\n"))
		return info, newBufConn(reader, conn), nil
	}

	// Not PROXY protocol
	return nil, newBufConn(reader, conn), nil
}

func parseProxyV1(line string) *ProxyInfo {
	parts := strings.Split(line, " ")
	if len(parts) < 2 {
		return nil
	}
	if parts[1] == "UNKNOWN" {
		return nil
	}
	if len(parts) != 6 {
		return nil
	}
	family := parts[1]
	if family != "TCP4" && family != "TCP6" {
		return nil
	}
	sourcePort, err1 := strconv.Atoi(parts[4])
	destPort, err2 := strconv.Atoi(parts[5])
	if err1 != nil || err2 != nil {
		return nil
	}
	return &ProxyInfo{
		SourceAddress: parts[2],
		SourcePort:    sourcePort,
		DestAddress:   parts[3],
		DestPort:      destPort,
		Family:        family,
	}
}

func parseProxyV2(r *bufio.Reader) (*ProxyInfo, error) {
	header := make([]byte, 16)
	if _, err := r.Read(header); err != nil {
		return nil, err
	}

	verCmd := header[12]
	version := (verCmd & 0xf0) >> 4
	command := verCmd & 0x0f
	if version != 2 {
		return nil, fmt.Errorf("unsupported proxy v2 version: %d", version)
	}

	famProto := header[13]
	family := (famProto & 0xf0) >> 4
	proto := famProto & 0x0f
	addrLen := binary.BigEndian.Uint16(header[14:16])

	addrData := make([]byte, addrLen)
	if _, err := r.Read(addrData); err != nil {
		return nil, err
	}

	if command == 0x00 {
		return nil, nil // LOCAL command
	}
	if command != 0x01 {
		return nil, fmt.Errorf("unsupported proxy v2 command: %d", command)
	}

	if family == 0x01 && proto == 0x01 { // TCP over IPv4
		if len(addrData) < 12 {
			return nil, fmt.Errorf("invalid v4 addr length")
		}
		return &ProxyInfo{
			SourceAddress: fmt.Sprintf("%d.%d.%d.%d", addrData[0], addrData[1], addrData[2], addrData[3]),
			DestAddress:   fmt.Sprintf("%d.%d.%d.%d", addrData[4], addrData[5], addrData[6], addrData[7]),
			SourcePort:    int(binary.BigEndian.Uint16(addrData[8:10])),
			DestPort:      int(binary.BigEndian.Uint16(addrData[10:12])),
			Family:        "TCP4",
		}, nil
	}

	if family == 0x02 && proto == 0x01 { // TCP over IPv6
		if len(addrData) < 36 {
			return nil, fmt.Errorf("invalid v6 addr length")
		}
		return &ProxyInfo{
			SourceAddress: formatIPv6(addrData[0:16]),
			DestAddress:   formatIPv6(addrData[16:32]),
			SourcePort:    int(binary.BigEndian.Uint16(addrData[32:34])),
			DestPort:      int(binary.BigEndian.Uint16(addrData[34:36])),
			Family:        "TCP6",
		}, nil
	}

	return nil, nil // unsupported family/protocol
}

func formatIPv6(b []byte) string {
	parts := make([]string, 8)
	for i := 0; i < 16; i += 2 {
		parts[i/2] = fmt.Sprintf("%x", binary.BigEndian.Uint16(b[i:i+2]))
	}
	return strings.Join(parts, ":")
}

// bufConn wraps a bufio.Reader and net.Conn so that already-read data is not lost.
type bufConn struct {
	reader *bufio.Reader
	net.Conn
}

func newBufConn(r *bufio.Reader, c net.Conn) net.Conn {
	return &bufConn{reader: r, Conn: c}
}

func (c *bufConn) Read(b []byte) (int, error) {
	return c.reader.Read(b)
}
