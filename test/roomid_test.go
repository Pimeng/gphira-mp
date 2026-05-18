package test

import (
	"testing"

	"github.com/Pimeng/gphira-mp-next/pkg/roomid"
)

func TestParse(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   error
	}{
		{"hello", "hello", nil},
		{"hello-world", "hello-world", nil},
		{"hello_world", "hello_world", nil},
		{"HelloWorld123", "HelloWorld123", nil},
		{"a", "a", nil},
		{"abcdefghijklmnopqrst", "abcdefghijklmnopqrst", nil}, // 20 chars
		{"", "", roomid.ErrEmpty},
		{"abcdefghijklmnopqrstu", "", roomid.ErrTooLong}, // 21 chars
		{"hello world", "", roomid.ErrInvalid},
		{"room@id", "", roomid.ErrInvalid},
		{"你好", "", roomid.ErrInvalid},
	}
	for _, tt := range tests {
		got, err := roomid.Parse(tt.input)
		if err != tt.err {
			t.Errorf("Parse(%q) error = %v, want %v", tt.input, err, tt.err)
			continue
		}
		if err == nil && string(got) != tt.want {
			t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
