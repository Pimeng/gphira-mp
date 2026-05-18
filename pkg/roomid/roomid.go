// Package roomid validates and parses Phira multiplayer room IDs.
package roomid

import (
	"errors"
)

// RoomID is a validated room identifier.
type RoomID string

var (
	ErrEmpty     = errors.New("roomid-empty")
	ErrTooLong   = errors.New("roomid-too-long")
	ErrInvalid   = errors.New("roomid-invalid")
)

// Parse validates and returns a RoomID.
func Parse(value string) (RoomID, error) {
	if len(value) == 0 {
		return "", ErrEmpty
	}
	if len(value) > 20 {
		return "", ErrTooLong
	}
	for _, ch := range value {
		if ch == '-' || ch == '_' || (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
			continue
		}
		return "", ErrInvalid
	}
	return RoomID(value), nil
}

// String returns the underlying string.
func (id RoomID) String() string {
	return string(id)
}
