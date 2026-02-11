package server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

const (
	// PROXY Protocol v1 前缀
	proxyProtocolV1Prefix = "PROXY "
	// PROXY Protocol v2 签名
	proxyProtocolV2Signature = "\x0D\x0A\x0D\x0A\x00\x0D\x0A\x51\x55\x49\x54\x0A"
)

// ProxyProtocolInfo 解析后的代理协议信息
type ProxyProtocolInfo struct {
	SourceIP   net.IP
	SourcePort int
	DestIP     net.IP
	DestPort   int
	Protocol   string // TCP4, TCP6, UNKNOWN
}

// ParseProxyProtocol 从连接中解析PROXY Protocol
// 返回值: 解析后的信息, 剩余数据, 错误
func ParseProxyProtocol(conn net.Conn, buf []byte) (*ProxyProtocolInfo, []byte, error) {
	reader := bufio.NewReader(conn)
	
	// 先查看是否有预读数据
	if len(buf) > 0 {
		reader = bufio.NewReader(io.MultiReader(bytes.NewReader(buf), conn))
	}
	
	// 尝试读取前12字节来判断协议版本
	peek, err := reader.Peek(12)
	if err != nil {
		return nil, nil, err
	}
	
	// 检查是否是v2协议
	if len(peek) >= 12 && string(peek[:12]) == proxyProtocolV2Signature {
		return parseProxyProtocolV2(reader)
	}
	
	// 检查是否是v1协议
	if len(peek) >= 6 && string(peek[:6]) == proxyProtocolV1Prefix {
		return parseProxyProtocolV1(reader)
	}
	
	// 不是PROXY Protocol
	return nil, nil, fmt.Errorf("not proxy protocol")
}

// parseProxyProtocolV1 解析PROXY Protocol v1
func parseProxyProtocolV1(reader *bufio.Reader) (*ProxyProtocolInfo, []byte, error) {
	// 读取整行（以\r\n结尾）
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, nil, err
	}
	
	// 移除\r\n
	line = strings.TrimSuffix(line, "\r\n")
	line = strings.TrimSuffix(line, "\n")
	
	// 解析: PROXY <PROTO> <SRC_IP> <DST_IP> <SRC_PORT> <DST_PORT>
	parts := strings.Split(line, " ")
	if len(parts) != 6 {
		return nil, nil, fmt.Errorf("invalid proxy protocol v1 format")
	}
	
	if parts[0] != "PROXY" {
		return nil, nil, fmt.Errorf("invalid proxy protocol v1 prefix")
	}
	
	info := &ProxyProtocolInfo{
		Protocol: parts[1],
	}
	
	// 处理UNKNOWN协议
	if info.Protocol == "UNKNOWN" {
		return info, nil, nil
	}
	
	// 解析IP地址
	info.SourceIP = net.ParseIP(parts[2])
	info.DestIP = net.ParseIP(parts[3])
	
	if info.SourceIP == nil || info.DestIP == nil {
		return nil, nil, fmt.Errorf("invalid IP address in proxy protocol")
	}
	
	// 解析端口
	srcPort, err := strconv.Atoi(parts[4])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid source port: %v", err)
	}
	info.SourcePort = srcPort
	
	dstPort, err := strconv.Atoi(parts[5])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid dest port: %v", err)
	}
	info.DestPort = dstPort
	
	return info, nil, nil
}

// parseProxyProtocolV2 解析PROXY Protocol v2
func parseProxyProtocolV2(reader *bufio.Reader) (*ProxyProtocolInfo, []byte, error) {
	// 读取固定头部 (16字节)
	header := make([]byte, 16)
	if _, err := reader.Read(header); err != nil {
		return nil, nil, err
	}
	
	// 验证签名
	if string(header[:12]) != proxyProtocolV2Signature {
		return nil, nil, fmt.Errorf("invalid proxy protocol v2 signature")
	}
	
	// 解析版本和命令 (第13字节)
	verCmd := header[12]
	version := (verCmd >> 4) & 0x0F
	command := verCmd & 0x0F
	
	if version != 2 {
		return nil, nil, fmt.Errorf("unsupported proxy protocol version: %d", version)
	}
	
	// 如果是LOCAL命令，没有地址信息
	if command == 0x00 {
		return &ProxyProtocolInfo{Protocol: "LOCAL"}, nil, nil
	}
	
	// 解析协议和地址族 (第14字节)
	protoFam := header[13]
	addressFamily := (protoFam >> 4) & 0x0F
	protocol := protoFam & 0x0F
	
	// 解析长度 (第15-16字节，大端序)
	length := int(header[14])<<8 | int(header[15])
	
	// 读取地址信息
	addrData := make([]byte, length)
	if _, err := reader.Read(addrData); err != nil {
		return nil, nil, err
	}
	
	info := &ProxyProtocolInfo{}
	
	// 根据地址族解析
	switch addressFamily {
	case 0x01: // AF_INET (IPv4)
		if len(addrData) < 12 {
			return nil, nil, fmt.Errorf("insufficient data for IPv4")
		}
		info.SourceIP = net.IP(addrData[0:4])
		info.DestIP = net.IP(addrData[4:8])
		info.SourcePort = int(addrData[8])<<8 | int(addrData[9])
		info.DestPort = int(addrData[10])<<8 | int(addrData[11])
		info.Protocol = "TCP4"
		
	case 0x02: // AF_INET6 (IPv6)
		if len(addrData) < 36 {
			return nil, nil, fmt.Errorf("insufficient data for IPv6")
		}
		info.SourceIP = net.IP(addrData[0:16])
		info.DestIP = net.IP(addrData[16:32])
		info.SourcePort = int(addrData[32])<<8 | int(addrData[33])
		info.DestPort = int(addrData[34])<<8 | int(addrData[35])
		info.Protocol = "TCP6"
		
	case 0x03: // AF_UNIX
		info.Protocol = "UNIX"
		
	default:
		return nil, nil, fmt.Errorf("unsupported address family: %d", addressFamily)
	}
	
	// 忽略协议类型（0x01=STREAM, 0x02=DGRAM）
	_ = protocol
	
	return info, nil, nil
}

// IsProxyProtocol 检查数据是否是PROXY Protocol
func IsProxyProtocol(data []byte) bool {
	if len(data) >= 6 && string(data[:6]) == proxyProtocolV1Prefix {
		return true
	}
	if len(data) >= 12 && string(data[:12]) == proxyProtocolV2Signature {
		return true
	}
	return false
}

// ProxyConn 包装连接以支持PROXY Protocol
type ProxyConn struct {
	net.Conn
	RemoteAddrOverride net.Addr
}

// RemoteAddr 返回真实远程地址
func (c *ProxyConn) RemoteAddr() net.Addr {
	if c.RemoteAddrOverride != nil {
		return c.RemoteAddrOverride
	}
	return c.Conn.RemoteAddr()
}

// proxyAddr 代理地址实现
type proxyAddr struct {
	ip   net.IP
	port int
}

func (a *proxyAddr) Network() string {
	return "tcp"
}

func (a *proxyAddr) String() string {
	return net.JoinHostPort(a.ip.String(), strconv.Itoa(a.port))
}

// NewProxyConn 创建带真实IP的连接包装器
func NewProxyConn(conn net.Conn, info *ProxyProtocolInfo) *ProxyConn {
	if info == nil || info.SourceIP == nil {
		return &ProxyConn{Conn: conn}
	}
	
	return &ProxyConn{
		Conn: conn,
		RemoteAddrOverride: &proxyAddr{
			ip:   info.SourceIP,
			port: info.SourcePort,
		},
	}
}
