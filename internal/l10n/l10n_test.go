package l10n

import (
	"strings"
	"testing"
)

func TestFormatPlainKey(t *testing.T) {
	lang := New("zh-CN")
	got := lang.Format("join-room-locked", nil)
	if got != "房间已锁定" {
		t.Errorf("expected 房间已锁定, got %q", got)
	}
}

func TestFormatVariableSubstitution(t *testing.T) {
	lang := New("zh-CN")
	got := lang.Format("room-banned", map[string]string{"id": "A1B2C"})
	if !strings.Contains(got, "A1B2C") {
		t.Errorf("expected room-banned to contain id, got %q", got)
	}
}

func TestFormatMissingKeyFallback(t *testing.T) {
	lang := New("zh-CN")
	got := lang.Format("nonexistent-key-xyz", nil)
	if got != "nonexistent-key-xyz" {
		t.Errorf("expected fallback to key, got %q", got)
	}
}

func TestFormatMultilineMessageProducesNewline(t *testing.T) {
	lang := New("zh-CN")
	got := lang.Format("chat-game-summary", map[string]string{
		"scoreText": "S",
		"accText":   "A",
		"stdText":   "T",
	})
	if !strings.Contains(got, "\n") {
		t.Errorf("expected FTL multiline to produce real newline, got %q", got)
	}
	if strings.Contains(got, `\n`) {
		t.Errorf("literal \\n must not appear in output, got %q", got)
	}
}

func TestFormatEnglish(t *testing.T) {
	lang := New("en-US")
	got := lang.Format("join-room-locked", nil)
	if got != "Room is locked" {
		t.Errorf("expected English translation, got %q", got)
	}
}

func TestFormatNilLanguage(t *testing.T) {
	var lang *Language
	got := lang.Format("any-key", nil)
	if got != "any-key" {
		t.Errorf("expected key fallback on nil Language, got %q", got)
	}
}

func TestLangCode(t *testing.T) {
	if New("en-US").Lang() != "en-US" {
		t.Error("expected en-US")
	}
	if New("").Lang() != "" {
		t.Error("expected empty input returns empty code")
	}
}
