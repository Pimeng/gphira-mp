package replay

import (
	"fmt"
	"os"

	"github.com/klauspost/compress/zstd"
	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
)

const (
	phiraRecordMagic      = "PHIRAREC"
	phiraRecordVersion    = 1
	phiraRecordHeaderSize = 13

	compressionNone    = 0x00
	compressionZstd    = 0x01
	compressionDeflate = 0x02
)

// buildRecordHeader creates a Phira record file header.
func buildRecordHeader(compression byte) []byte {
	header := make([]byte, phiraRecordHeaderSize)
	copy(header, phiraRecordMagic)
	header[8] = byte(phiraRecordVersion)
	header[9] = 0
	header[10] = 0
	header[11] = 0
	header[12] = compression
	return header
}

// encodeRecordContent encodes the inner record payload.
func encodeRecordContent(recordID int32, timestamp int64, chartID int32, chartName string, userID int32, userName string, frames []protocol.TouchFrame, judges []protocol.JudgeEvent) []byte {
	w := protocol.NewBinaryWriter()

	w.WriteI32(recordID)
	w.WriteI64(timestamp)
	w.WriteI32(chartID)
	w.WriteString(chartName)
	w.WriteI32(userID)
	w.WriteString(userName)

	// Touch frames
	w.WriteU32(uint32(len(frames)))
	for _, f := range frames {
		w.WriteF32(f.Time)
		w.WriteU32(uint32(len(f.Points)))
		for _, p := range f.Points {
			w.WriteI8(p.ID)
			w.WriteF32(p.Pos.X)
			w.WriteF32(p.Pos.Y)
		}
	}

	// Judge events (use I32 for line_id/note_id in replay format)
	w.WriteU32(uint32(len(judges)))
	for _, j := range judges {
		w.WriteF32(j.Time)
		w.WriteI32(int32(j.LineID))
		w.WriteI32(int32(j.NoteID))
		w.WriteU8(uint8(j.Judgement))
	}

	return w.Bytes()
}

// compressPayload compresses data using the specified algorithm.
func compressPayload(data []byte, compression byte) ([]byte, error) {
	switch compression {
	case compressionNone:
		return data, nil
	case compressionZstd:
		enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			return nil, err
		}
		return enc.EncodeAll(data, nil), nil
	default:
		return data, nil
	}
}

// ReplayFilePath returns the file path for a replay record.
func ReplayFilePath(baseDir string, userID int32, chartID int32, timestamp int64) string {
	return baseDir + "/" + itoa(int(userID)) + "/" + itoa(int(chartID)) + "/" + i64toa(timestamp) + ".phirarec"
}

// ReplayHeader holds the decoded metadata from a replay file.
type ReplayHeader struct {
	RecordID  int32
	Timestamp int64
	ChartID   int32
	ChartName string
	UserID    int32
	UserName  string
}

// ReadReplayHeader reads and decodes the header/metadata from a replay file.
func ReadReplayHeader(path string) (*ReplayHeader, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < phiraRecordHeaderSize {
		return nil, fmt.Errorf("file too short")
	}

	// Verify magic
	if string(data[:8]) != phiraRecordMagic {
		return nil, fmt.Errorf("invalid magic")
	}

	compression := data[12]
	payload := data[phiraRecordHeaderSize:]

	var content []byte
	switch compression {
	case compressionNone:
		content = payload
	case compressionZstd:
		dec, err := zstd.NewReader(nil)
		if err != nil {
			return nil, err
		}
		content, err = dec.DecodeAll(payload, nil)
		dec.Close()
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported compression: %d", compression)
	}

	r := protocol.NewBinaryReader(content)
	h := &ReplayHeader{
		RecordID:  r.ReadI32(),
		Timestamp: r.ReadI64(),
		ChartID:   r.ReadI32(),
		ChartName: r.ReadString(),
		UserID:    r.ReadI32(),
		UserName:  r.ReadString(),
	}
	return h, nil
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf) - 1
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		buf[i] = byte('0' + v%10)
		v /= 10
		i--
	}
	if neg {
		buf[i] = '-'
		i--
	}
	return string(buf[i+1:])
}

func i64toa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf) - 1
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		buf[i] = byte('0' + v%10)
		v /= 10
		i--
	}
	if neg {
		buf[i] = '-'
		i--
	}
	return string(buf[i+1:])
}
