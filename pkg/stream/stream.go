// Package stream provides a batched, framed TCP stream for the Phira MP protocol.
package stream

import (
	"net"
	"sync"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

const (
	sendTimeoutMs     = 5000
	batchSendDelayMs  = 5
	maxBatchSize      = 20
	protocolVersion   = 1
	maxHandlerWorkers = 64
)

// Codec defines encode/decode and priority for a stream.
type Codec[S, R any] struct {
	EncodeSend     func(S) []byte
	DecodeRecv     func([]byte) (R, error)
	IsHighPriority func(S) bool
}

// Stream manages a single TCP connection with framing, batching, and fast path.
type Stream[S, R any] struct {
	conn     net.Conn
	version  byte
	codec    Codec[S, R]
	handler  func(R) error
	fastPath func(R) bool
	onError  func(phase string, err error)

	recvBuf []byte
	closed  bool
	closeMu sync.Mutex

	sendBatch [][]byte
	sendTimer *time.Timer
	sendMu    sync.Mutex
	sending   bool

	handlerSem chan struct{}
}

// New creates a new stream after version negotiation (server-side).
func New[S, R any](conn net.Conn, codec Codec[S, R], handler func(R) error, fastPath func(R) bool, onError func(string, error)) (*Stream[S, R], error) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
	}

	// Version negotiation: server expects version 1
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != nil {
		return nil, err
	}
	if buf[0] != protocolVersion {
		conn.Close()
		return nil, protocol.ErrInvalidLength
	}

	s := &Stream[S, R]{
		conn:       conn,
		version:    protocolVersion,
		codec:      codec,
		handler:    handler,
		fastPath:   fastPath,
		onError:    onError,
		handlerSem: make(chan struct{}, maxHandlerWorkers),
	}
	go s.readLoop()
	return s, nil
}

// NewClient creates a client-side stream (sends version byte, no ack expected).
func NewClient[S, R any](conn net.Conn, codec Codec[S, R], handler func(R) error, fastPath func(R) bool, onError func(string, error)) (*Stream[S, R], error) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)
	}

	// Client sends version first (matching TS behavior: no ack expected)
	if _, err := conn.Write([]byte{protocolVersion}); err != nil {
		return nil, err
	}

	s := &Stream[S, R]{
		conn:       conn,
		version:    protocolVersion,
		codec:      codec,
		handler:    handler,
		fastPath:   fastPath,
		onError:    onError,
		handlerSem: make(chan struct{}, maxHandlerWorkers),
	}
	go s.readLoop()
	return s, nil
}

// readLoop continuously reads and decodes frames.
func (s *Stream[S, R]) readLoop() {
	var buf []byte
	for {
		tmp := make([]byte, 4096)
		n, err := s.conn.Read(tmp)
		if err != nil {
			s.close()
			return
		}
		buf = append(buf, tmp[:n]...)

		for {
			res := protocol.TryDecodeFrame(buf, protocol.MaxPayloadBytes)
			if res.NeedMore {
				break
			}
			if res.Error != nil {
				if s.onError != nil {
					s.onError("decode", res.Error)
				}
				s.close()
				return
			}
			buf = res.Remaining

			pkt, err := s.codec.DecodeRecv(res.Payload)
			if err != nil {
				if s.onError != nil {
					s.onError("decode", err)
				}
				s.close()
				return
			}

			if s.handler == nil {
				continue
			}

			if s.fastPath != nil && s.fastPath(pkt) {
				if err := s.handler(pkt); err != nil {
					if s.onError != nil {
						s.onError("handler", err)
					}
					s.close()
				}
				continue
			}

			s.handlerSem <- struct{}{}

			go func(p R) {
				defer func() { <-s.handlerSem }()
				if err := s.handler(p); err != nil {
					if s.onError != nil {
						s.onError("handler", err)
					}
					s.close()
				}
			}(pkt)
		}
	}
}

// Send sends a message. High priority messages bypass batching.
func (s *Stream[S, R]) Send(msg S) error {
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return protocol.ErrUnexpectedEOF
	}
	s.closeMu.Unlock()

	body := s.codec.EncodeSend(msg)
	header := protocol.EncodeLengthPrefixU32(uint32(len(body)))
	frame := append(header, body...)

	if s.codec.IsHighPriority != nil && s.codec.IsHighPriority(msg) {
		s.flushSendBatch()
		return s.writeFrame(frame)
	}

	s.sendMu.Lock()
	s.sendBatch = append(s.sendBatch, frame)
	shouldFlush := len(s.sendBatch) >= maxBatchSize
	if s.sendTimer == nil {
		s.sendTimer = time.AfterFunc(batchSendDelayMs*time.Millisecond, s.flushSendBatch)
	}
	s.sendMu.Unlock()

	if shouldFlush {
		s.flushSendBatch()
	}
	return nil
}

// flushSendBatch writes all pending frames.
func (s *Stream[S, R]) flushSendBatch() {
	s.sendMu.Lock()
	if s.sendTimer != nil {
		s.sendTimer.Stop()
		s.sendTimer = nil
	}
	if len(s.sendBatch) == 0 || s.sending {
		s.sendMu.Unlock()
		return
	}
	batch := s.sendBatch
	s.sendBatch = nil
	s.sending = true
	s.sendMu.Unlock()

	var combined []byte
	for _, f := range batch {
		combined = append(combined, f...)
	}
	_ = s.writeFrame(combined)

	s.sendMu.Lock()
	s.sending = false
	if len(s.sendBatch) > 0 && s.sendTimer == nil {
		s.sendTimer = time.AfterFunc(0, s.flushSendBatch)
	}
	s.sendMu.Unlock()
}

func (s *Stream[S, R]) writeFrame(frame []byte) error {
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return protocol.ErrUnexpectedEOF
	}
	conn := s.conn
	s.closeMu.Unlock()

	conn.SetWriteDeadline(time.Now().Add(sendTimeoutMs * time.Millisecond))
	_, err := conn.Write(frame)
	return err
}

// Close closes the stream and connection.
func (s *Stream[S, R]) Close() {
	s.close()
}

func (s *Stream[S, R]) close() {
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return
	}
	s.closed = true
	s.closeMu.Unlock()

	s.sendMu.Lock()
	if s.sendTimer != nil {
		s.sendTimer.Stop()
		s.sendTimer = nil
	}
	s.sendMu.Unlock()

	s.conn.Close()
}

// IsClosed returns true if the stream is closed.
func (s *Stream[S, R]) IsClosed() bool {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	return s.closed
}
