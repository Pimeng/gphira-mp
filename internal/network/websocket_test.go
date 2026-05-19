package network

import "testing"

func TestWSClientEnqueueAfterCloseDoesNotPanic(t *testing.T) {
	c := &WSClient{send: make(chan []byte, 1)}
	c.close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("sendJSON panicked after close: %v", r)
		}
	}()

	c.sendJSON(map[string]any{"type": "ping"})
}

func TestWSClientHeartbeatAliveFlag(t *testing.T) {
	c := &WSClient{send: make(chan []byte, 1), isAlive: true}

	if !c.consumeAliveForHeartbeat() {
		t.Fatal("alive client should be kept")
	}
	if c.consumeAliveForHeartbeat() {
		t.Fatal("client should be removed on second heartbeat without pong")
	}

	c.setAlive(true)
	if !c.consumeAliveForHeartbeat() {
		t.Fatal("client should be kept after pong marks alive")
	}
}
