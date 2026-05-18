package protocol

import (
	"errors"
)

var (
	ErrInvalidLength        = errors.New("frame-invalid-length")
	ErrPayloadTooLarge      = errors.New("frame-payload-too-large")
	ErrInvalidLengthPrefix  = errors.New("frame-invalid-length-prefix")
)

const MaxPayloadBytes = 2 * 1024 * 1024

// EncodeLengthPrefixU32 encodes a uint32 as LEB128.
func EncodeLengthPrefixU32(length uint32) []byte {
	if length < 0x80 {
		return []byte{byte(length)}
	}
	out := make([]byte, 0, 5)
	x := length
	for {
		b := byte(x & 0x7f)
		x >>= 7
		if x != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if x == 0 {
			break
		}
	}
	return out
}

// DecodeFrameResult represents the result of trying to decode a frame.
type DecodeFrameResult struct {
	Payload   []byte
	Remaining []byte
	NeedMore  bool
	Error     error
}

// TryDecodeFrame attempts to decode a length-prefixed frame from buf.
func TryDecodeFrame(buf []byte, maxPayloadBytes int) DecodeFrameResult {
	if len(buf) == 0 {
		return DecodeFrameResult{NeedMore: true}
	}

	length := uint32(0)
	offset := 0
	shift := 0

	for {
		if offset >= len(buf) {
			return DecodeFrameResult{NeedMore: true}
		}
		b := buf[offset]
		offset++
		length |= uint32(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 35 { // more than 5 bytes
			return DecodeFrameResult{Error: ErrInvalidLengthPrefix}
		}
	}

	if int(length) > maxPayloadBytes {
		return DecodeFrameResult{Error: ErrPayloadTooLarge}
	}
	if len(buf)-offset < int(length) {
		return DecodeFrameResult{NeedMore: true}
	}

	payload := buf[offset : offset+int(length)]
	remaining := buf[offset+int(length):]
	return DecodeFrameResult{Payload: payload, Remaining: remaining}
}
