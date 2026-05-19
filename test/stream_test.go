package test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Pimeng/gphira-mp-next/pkg/protocol"
	"github.com/Pimeng/gphira-mp-next/pkg/stream"
)

func TestStreamVersionNegotiation(t *testing.T) {
	// Server expects version 1
	clientData := []byte{0x01} // version 1
	conn := newMockConn(clientData)

	codec := stream.Codec[protocol.ServerCommand, protocol.ClientCommand]{
		EncodeSend: func(cmd protocol.ServerCommand) []byte {
			w := protocol.NewBinaryWriter()
			protocol.EncodeServerCommand(w, cmd)
			return w.Bytes()
		},
		DecodeRecv: func(data []byte) (protocol.ClientCommand, error) {
			r := protocol.NewBinaryReader(data)
			return protocol.DecodeClientCommand(r), nil
		},
		IsHighPriority: func(cmd protocol.ServerCommand) bool { return false },
	}

	stream, err := stream.New(conn, codec, func(cmd protocol.ClientCommand) error {
		return nil
	}, nil, nil)
	if err != nil {
		t.Fatalf("new stream failed: %v", err)
	}
	defer stream.Close()

	// Stream should be created successfully with version 1
	if stream == nil {
		t.Fatal("stream should not be nil")
	}
}

func TestStreamVersionMismatch(t *testing.T) {
	clientData := []byte{0x02} // version 2 (unsupported)
	conn := newMockConn(clientData)

	codec := stream.Codec[protocol.ServerCommand, protocol.ClientCommand]{}
	_, err := stream.New(conn, codec, nil, nil, nil)
	if err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestStreamSendReceive(t *testing.T) {
	// Version byte + one framed command
	w := protocol.NewBinaryWriter()
	protocol.EncodeClientCommand(w, protocol.ClientCommand{Type: protocol.ClientCmdPing})
	body := w.Bytes()
	frame := append(protocol.EncodeLengthPrefixU32(uint32(len(body))), body...)
	data := append([]byte{0x01}, frame...)
	conn := newMockConn(data)

	codec := stream.Codec[protocol.ServerCommand, protocol.ClientCommand]{
		EncodeSend: func(cmd protocol.ServerCommand) []byte {
			w := protocol.NewBinaryWriter()
			protocol.EncodeServerCommand(w, cmd)
			return w.Bytes()
		},
		DecodeRecv: func(data []byte) (protocol.ClientCommand, error) {
			r := protocol.NewBinaryReader(data)
			return protocol.DecodeClientCommand(r), nil
		},
		IsHighPriority: func(cmd protocol.ServerCommand) bool { return false },
	}

	done := make(chan protocol.ClientCommand, 1)
	stream, err := stream.New(conn, codec, func(cmd protocol.ClientCommand) error {
		done <- cmd
		return nil
	}, nil, nil)
	if err != nil {
		t.Fatalf("new stream failed: %v", err)
	}
	defer stream.Close()

	select {
	case cmd := <-done:
		if cmd.Type != protocol.ClientCmdPing {
			t.Errorf("cmd type = %d, want Ping", cmd.Type)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for command")
	}
}

func TestStreamSend(t *testing.T) {
	conn := newMockConn([]byte{0x01})

	codec := stream.Codec[protocol.ServerCommand, protocol.ClientCommand]{
		EncodeSend: func(cmd protocol.ServerCommand) []byte {
			return []byte{byte(cmd.Type)}
		},
		DecodeRecv:     func(data []byte) (protocol.ClientCommand, error) { return protocol.ClientCommand{}, nil },
		IsHighPriority: func(cmd protocol.ServerCommand) bool { return true },
	}

	stream, err := stream.New(conn, codec, nil, nil, nil)
	if err != nil {
		t.Fatalf("new stream failed: %v", err)
	}
	defer stream.Close()

	// High priority should be sent immediately
	if err := stream.Send(protocol.ServerCommand{Type: protocol.ServerCmdPong}); err != nil {
		t.Errorf("send failed: %v", err)
	}

	// Give some time for async write
	time.Sleep(50 * time.Millisecond)

	if conn.writeBuf.Len() == 0 {
		t.Error("expected data to be written")
	}
}

func TestStreamHandlerConcurrencyBounded(t *testing.T) {
	const packets = 200

	w := protocol.NewBinaryWriter()
	protocol.EncodeClientCommand(w, protocol.ClientCommand{Type: protocol.ClientCmdPing})
	body := w.Bytes()
	frame := append(protocol.EncodeLengthPrefixU32(uint32(len(body))), body...)

	data := []byte{0x01}
	for i := 0; i < packets; i++ {
		data = append(data, frame...)
	}

	conn := newMockConn(data)

	codec := stream.Codec[protocol.ServerCommand, protocol.ClientCommand]{
		EncodeSend: func(cmd protocol.ServerCommand) []byte {
			return []byte{byte(cmd.Type)}
		},
		DecodeRecv:     func(data []byte) (protocol.ClientCommand, error) { return protocol.ClientCommand{}, nil },
		IsHighPriority: func(cmd protocol.ServerCommand) bool { return false },
	}

	var inFlight int32
	var maxInFlight int32
	var done sync.WaitGroup
	done.Add(packets)

	strm, err := stream.New(conn, codec, func(cmd protocol.ClientCommand) error {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			seen := atomic.LoadInt32(&maxInFlight)
			if current <= seen || atomic.CompareAndSwapInt32(&maxInFlight, seen, current) {
				break
			}
		}

		time.Sleep(15 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		done.Done()
		return nil
	}, nil, nil)
	if err != nil {
		t.Fatalf("new stream failed: %v", err)
	}
	defer strm.Close()

	waitCh := make(chan struct{})
	go func() {
		done.Wait()
		close(waitCh)
	}()

	select {
	case <-waitCh:
	case <-time.After(8 * time.Second):
		t.Fatal("timeout waiting handlers to finish")
	}

	if atomic.LoadInt32(&maxInFlight) > 64 {
		t.Fatalf("max handler concurrency = %d, want <= 64", maxInFlight)
	}
}
