package common

import (
	"encoding/binary"
	"fmt"
	"math"
)

// BinaryReader 二进制数据读取器
type BinaryReader struct {
	data []byte
	pos  int
}

// NewBinaryReader 创建新的二进制读取器
func NewBinaryReader(data []byte) *BinaryReader {
	return &BinaryReader{data: data, pos: 0}
}

// Byte 读取一个字节
func (r *BinaryReader) Byte() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("unexpected EOF")
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

// Take 读取指定长度的字节
func (r *BinaryReader) Take(n int) ([]byte, error) {
	if r.pos+n > len(r.data) {
		return nil, fmt.Errorf("unexpected EOF")
	}
	result := r.data[r.pos : r.pos+n]
	r.pos += n
	return result, nil
}

// Uleb 读取无符号LEB128编码
func (r *BinaryReader) Uleb() (uint64, error) {
	var result uint64
	var shift uint
	for {
		b, err := r.Byte()
		if err != nil {
			return 0, err
		}
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
	}
	return result, nil
}

// Read 读取实现了BinaryData接口的类型
func (r *BinaryReader) Read(v BinaryData) error {
	return v.ReadBinary(r)
}

// BinaryWriter 二进制数据写入器
type BinaryWriter struct {
	data []byte
}

// NewBinaryWriter 创建新的二进制写入器
func NewBinaryWriter() *BinaryWriter {
	return &BinaryWriter{data: make([]byte, 0)}
}

// Data 获取写入的数据
func (w *BinaryWriter) Data() []byte {
	return w.data
}

// WriteByte 写入一个字节
func (w *BinaryWriter) WriteByte(b byte) {
	w.data = append(w.data, b)
}

// WriteBytes 写入字节切片
func (w *BinaryWriter) WriteBytes(b []byte) {
	w.data = append(w.data, b...)
}

// Uleb 写入无符号LEB128编码
func (w *BinaryWriter) Uleb(v uint64) {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		w.WriteByte(b)
		if v == 0 {
			break
		}
	}
}

// Write 写入实现了BinaryData接口的类型
func (w *BinaryWriter) Write(v BinaryData) error {
	return v.WriteBinary(w)
}

// BinaryData 二进制数据接口
type BinaryData interface {
	ReadBinary(r *BinaryReader) error
	WriteBinary(w *BinaryWriter) error
}

// ReadInt8 读取int8
func ReadInt8(r *BinaryReader) (int8, error) {
	b, err := r.Byte()
	if err != nil {
		return 0, err
	}
	return int8(b), nil
}

// WriteInt8 写入int8
func WriteInt8(w *BinaryWriter, v int8) {
	w.WriteByte(byte(v))
}

// ReadUint8 读取uint8
func ReadUint8(r *BinaryReader) (uint8, error) {
	return r.Byte()
}

// WriteUint8 写入uint8
func WriteUint8(w *BinaryWriter, v uint8) {
	w.WriteByte(v)
}

// ReadUint16 读取uint16
func ReadUint16(r *BinaryReader) (uint16, error) {
	data, err := r.Take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(data), nil
}

// WriteUint16 写入uint16
func WriteUint16(w *BinaryWriter, v uint16) {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	w.WriteBytes(b)
}

// ReadUint32 读取uint32
func ReadUint32(r *BinaryReader) (uint32, error) {
	data, err := r.Take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(data), nil
}

// WriteUint32 写入uint32
func WriteUint32(w *BinaryWriter, v uint32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	w.WriteBytes(b)
}

// ReadInt32 读取int32
func ReadInt32(r *BinaryReader) (int32, error) {
	data, err := r.Take(4)
	if err != nil {
		return 0, err
	}
	return int32(binary.LittleEndian.Uint32(data)), nil
}

// WriteInt32 写入int32
func WriteInt32(w *BinaryWriter, v int32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(v))
	w.WriteBytes(b)
}

// ReadFloat32 读取float32
func ReadFloat32(r *BinaryReader) (float32, error) {
	data, err := r.Take(4)
	if err != nil {
		return 0, err
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(data)), nil
}

// WriteFloat32 写入float32
func WriteFloat32(w *BinaryWriter, v float32) {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
	w.WriteBytes(b)
}

// ReadBool 读取bool
func ReadBool(r *BinaryReader) (bool, error) {
	b, err := r.Byte()
	if err != nil {
		return false, err
	}
	return b == 1, nil
}

// WriteBool 写入bool
func WriteBool(w *BinaryWriter, v bool) {
	if v {
		w.WriteByte(1)
	} else {
		w.WriteByte(0)
	}
}

// ReadString 读取string
func ReadString(r *BinaryReader) (string, error) {
	len, err := r.Uleb()
	if err != nil {
		return "", err
	}
	data, err := r.Take(int(len))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteString 写入string
func WriteString(w *BinaryWriter, v string) {
	w.Uleb(uint64(len(v)))
	w.WriteBytes([]byte(v))
}
