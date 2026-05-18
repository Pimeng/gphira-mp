package test

import (
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/l10n"
)

func TestFormat(t *testing.T) {
	lang := l10n.New("zh-CN")
	got := lang.Format("chat-disabled-by-server", nil)
	if got == "" {
		t.Error("format should return non-empty string")
	}
}

func TestFormatWithArgs(t *testing.T) {
	lang := l10n.New("zh-CN")
	// Test a key that likely has variable substitution
	got := lang.Format("auth-repeated-authenticate", nil)
	if got == "" {
		t.Error("format should return non-empty string")
	}
}

func TestTl(t *testing.T) {
	lang := l10n.New("en-US")
	got := lang.Format("room-not-found", nil)
	if got == "" {
		t.Error("format should return non-empty string")
	}
}

func TestUnknownLang(t *testing.T) {
	lang := l10n.New("fr-FR")
	got := lang.Format("room-not-found", nil)
	if got == "" {
		t.Error("unknown lang should fallback and return non-empty string")
	}
}
