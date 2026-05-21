// Package protocol implements the Phira MP custom binary protocol.
package protocol

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/Pimeng/gphira-mp-next/pkg/half"
)

var (
	ErrUnexpectedEOF   = errors.New("binary-unexpected-eof")
	ErrLengthTooLarge  = errors.New("binary-length-too-large")
	ErrStringTooLong   = errors.New("binary-string-too-long")
)

// BinaryReader reads typed data from a byte slice.
type BinaryReader struct {
	buf    []byte
	offset int
}

// NewBinaryReader creates a new reader.
func NewBinaryReader(buf []byte) *BinaryReader {
	return &BinaryReader{buf: buf}
}

func (r *BinaryReader) ensure(n int) {
	if r.offset+n > len(r.buf) {
		panic(ErrUnexpectedEOF)
	}
}

// Take returns n bytes and advances the offset.
func (r *BinaryReader) Take(n int) []byte {
	r.ensure(n)
	out := r.buf[r.offset : r.offset+n]
	r.offset += n
	return out
}

// ReadU8 reads an unsigned 8-bit integer.
func (r *BinaryReader) ReadU8() uint8 {
	r.ensure(1)
	v := r.buf[r.offset]
	r.offset++
	return v
}

// ReadI8 reads a signed 8-bit integer.
func (r *BinaryReader) ReadI8() int8 {
	return int8(r.ReadU8())
}

// ReadBool reads a boolean.
func (r *BinaryReader) ReadBool() bool {
	return r.ReadU8() == 1
}

// ReadU16 reads an unsigned 16-bit integer (little endian).
func (r *BinaryReader) ReadU16() uint16 {
	r.ensure(2)
	v := binary.LittleEndian.Uint16(r.buf[r.offset:])
	r.offset += 2
	return v
}

// ReadU32 reads an unsigned 32-bit integer (little endian).
func (r *BinaryReader) ReadU32() uint32 {
	r.ensure(4)
	v := binary.LittleEndian.Uint32(r.buf[r.offset:])
	r.offset += 4
	return v
}

// ReadI32 reads a signed 32-bit integer (little endian).
func (r *BinaryReader) ReadI32() int32 {
	return int32(r.ReadU32())
}

// ReadU64 reads an unsigned 64-bit integer (little endian).
func (r *BinaryReader) ReadU64() uint64 {
	r.ensure(8)
	v := binary.LittleEndian.Uint64(r.buf[r.offset:])
	r.offset += 8
	return v
}

// ReadI64 reads a signed 64-bit integer (little endian).
func (r *BinaryReader) ReadI64() int64 {
	return int64(r.ReadU64())
}

// ReadF32 reads a 32-bit float (little endian).
func (r *BinaryReader) ReadF32() float32 {
	v := r.ReadU32()
	return math.Float32frombits(v)
}

// ReadUlebBigInt reads an unsigned LEB128 integer as uint64.
func (r *BinaryReader) ReadUlebBigInt() uint64 {
	var result uint64
	var shift uint
	for {
		b := r.ReadU8()
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			return result
		}
		shift += 7
	}
}

// ReadUlebNumber reads an unsigned LEB128 integer as int.
func (r *BinaryReader) ReadUlebNumber() int {
	v := r.ReadUlebBigInt()
	if v > math.MaxInt32 {
		panic(ErrLengthTooLarge)
	}
	return int(v)
}

// ReadString reads a UTF-8 string (LEB128 length prefix).
func (r *BinaryReader) ReadString() string {
	len := r.ReadUlebNumber()
	return string(r.Take(len))
}

// ReadVarchar reads a UTF-8 string with max length check.
func (r *BinaryReader) ReadVarchar(maxLen int) string {
	len := r.ReadUlebNumber()
	if len > maxLen {
		panic(ErrStringTooLong)
	}
	return string(r.Take(len))
}

// ReadOption reads an optional value.
func ReadOption[T any](r *BinaryReader, decode func(*BinaryReader) T) *T {
	if r.ReadBool() {
		v := decode(r)
		return &v
	}
	return nil
}

// ReadResult reads a Result<T, string>.
func ReadResult[T any](r *BinaryReader, decodeOk func(*BinaryReader) T) StringResult[T] {
	if r.ReadBool() {
		return StringResult[T]{Ok: true, Value: decodeOk(r)}
	}
	errStr := r.ReadString()
	return StringResult[T]{Ok: false, Error: errStr}
}

// ReadResultErr reads a generic Result<T, E> where the error branch is decoded
// via decodeErr. The returned ok flag indicates which branch was taken; only
// one of value/errVal is meaningful per the flag. This mirrors the TS
// readResult<Ok, Err>(decodeOk, decodeErr) signature in common/binary.ts.
func ReadResultErr[T any, E any](r *BinaryReader, decodeOk func(*BinaryReader) T, decodeErr func(*BinaryReader) E) (value T, errVal E, ok bool) {
	if r.ReadBool() {
		return decodeOk(r), errVal, true
	}
	return value, decodeErr(r), false
}

// ReadArray reads an array of T.
func ReadArray[T any](r *BinaryReader, decode func(*BinaryReader) T) []T {
	n := r.ReadUlebNumber()
	out := make([]T, n)
	for i := 0; i < n; i++ {
		out[i] = decode(r)
	}
	return out
}

// ReadMap reads a map[K]V.
func ReadMap[K comparable, V any](r *BinaryReader, decodeK func(*BinaryReader) K, decodeV func(*BinaryReader) V) map[K]V {
	n := r.ReadUlebNumber()
	out := make(map[K]V, n)
	for i := 0; i < n; i++ {
		k := decodeK(r)
		v := decodeV(r)
		out[k] = v
	}
	return out
}

// ReadCompactPos reads a compact position (two float16 values).
func (r *BinaryReader) ReadCompactPos() CompactPos {
	xBits := r.ReadU16()
	yBits := r.ReadU16()
	return CompactPos{
		X: half.F16BitsToF32(xBits),
		Y: half.F16BitsToF32(yBits),
	}
}

// ---------------------------------------------------------------------------
// BinaryWriter
// ---------------------------------------------------------------------------

// BinaryWriter writes typed data to a growing buffer.
type BinaryWriter struct {
	buf []byte
}

// NewBinaryWriter creates a new writer.
func NewBinaryWriter() *BinaryWriter {
	return &BinaryWriter{}
}

// Bytes returns the written data.
func (w *BinaryWriter) Bytes() []byte {
	return w.buf
}

// WriteU8 writes an unsigned 8-bit integer.
func (w *BinaryWriter) WriteU8(v uint8) {
	w.buf = append(w.buf, v)
}

// WriteI8 writes a signed 8-bit integer.
func (w *BinaryWriter) WriteI8(v int8) {
	w.WriteU8(uint8(v))
}

// WriteBool writes a boolean.
func (w *BinaryWriter) WriteBool(v bool) {
	if v {
		w.WriteU8(1)
	} else {
		w.WriteU8(0)
	}
}

// WriteU16 writes an unsigned 16-bit integer (little endian).
func (w *BinaryWriter) WriteU16(v uint16) {
	w.buf = binary.LittleEndian.AppendUint16(w.buf, v)
}

// WriteU32 writes an unsigned 32-bit integer (little endian).
func (w *BinaryWriter) WriteU32(v uint32) {
	w.buf = binary.LittleEndian.AppendUint32(w.buf, v)
}

// WriteI32 writes a signed 32-bit integer (little endian).
func (w *BinaryWriter) WriteI32(v int32) {
	w.WriteU32(uint32(v))
}

// WriteU64 writes an unsigned 64-bit integer (little endian).
func (w *BinaryWriter) WriteU64(v uint64) {
	w.buf = binary.LittleEndian.AppendUint64(w.buf, v)
}

// WriteI64 writes a signed 64-bit integer (little endian).
func (w *BinaryWriter) WriteI64(v int64) {
	w.WriteU64(uint64(v))
}

// WriteF32 writes a 32-bit float (little endian).
func (w *BinaryWriter) WriteF32(v float32) {
	w.WriteU32(math.Float32bits(v))
}

// WriteUleb writes an unsigned LEB128 integer.
func (w *BinaryWriter) WriteUleb(v uint64) {
	for {
		byteVal := uint8(v & 0x7f)
		v >>= 7
		if v != 0 {
			byteVal |= 0x80
		}
		w.WriteU8(byteVal)
		if v == 0 {
			return
		}
	}
}

// WriteString writes a UTF-8 string (LEB128 length prefix).
func (w *BinaryWriter) WriteString(s string) {
	w.WriteUleb(uint64(len(s)))
	w.buf = append(w.buf, s...)
}

// WriteVarchar writes a UTF-8 string with max length check.
func (w *BinaryWriter) WriteVarchar(maxLen int, s string) {
	if len(s) > maxLen {
		panic(ErrStringTooLong)
	}
	w.WriteUleb(uint64(len(s)))
	w.buf = append(w.buf, s...)
}

// WriteOption writes an optional value.
func WriteOption[T any](w *BinaryWriter, v *T, encode func(*BinaryWriter, T)) {
	if v == nil {
		w.WriteBool(false)
		return
	}
	w.WriteBool(true)
	encode(w, *v)
}

// WriteResult writes a Result<T, string>.
func WriteResult[T any](w *BinaryWriter, v StringResult[T], encodeOk func(*BinaryWriter, T)) {
	if v.Ok {
		w.WriteBool(true)
		encodeOk(w, v.Value)
	} else {
		w.WriteBool(false)
		w.WriteString(v.Error)
	}
}

// WriteResultErr writes a generic Result<T, E> via the supplied encoders.
// Mirrors the TS writeResult<Ok, Err>(encodeOk, encodeErr) signature.
func WriteResultErr[T any, E any](w *BinaryWriter, ok bool, value T, errVal E, encodeOk func(*BinaryWriter, T), encodeErr func(*BinaryWriter, E)) {
	if ok {
		w.WriteBool(true)
		encodeOk(w, value)
	} else {
		w.WriteBool(false)
		encodeErr(w, errVal)
	}
}

// WriteArray writes an array of T.
func WriteArray[T any](w *BinaryWriter, arr []T, encode func(*BinaryWriter, T)) {
	w.WriteUleb(uint64(len(arr)))
	for _, it := range arr {
		encode(w, it)
	}
}

// WriteMap writes a map[K]V.
func WriteMap[K comparable, V any](w *BinaryWriter, m map[K]V, encodeK func(*BinaryWriter, K), encodeV func(*BinaryWriter, V)) {
	w.WriteUleb(uint64(len(m)))
	for k, v := range m {
		encodeK(w, k)
		encodeV(w, v)
	}
}

// WriteCompactPos writes a compact position (two float16 values).
func (w *BinaryWriter) WriteCompactPos(pos CompactPos) {
	w.WriteU16(half.F32ToF16Bits(pos.X))
	w.WriteU16(half.F32ToF16Bits(pos.Y))
}

// ---------------------------------------------------------------------------
// StringResult
// ---------------------------------------------------------------------------

// StringResult is a Rust-style Result with string error type.
type StringResult[T any] struct {
	Ok    bool
	Value T
	Error string
}

// Ok creates a success result.
func Ok[T any](value T) StringResult[T] {
	return StringResult[T]{Ok: true, Value: value}
}

// Err creates an error result.
func Err[T any](err string) StringResult[T] {
	return StringResult[T]{Ok: false, Error: err}
}
