package test

import (
	"os"
	"testing"

	"github.com/Pimeng/gphira-mp-next/internal/version"
)

func TestReadVersionFromEnv(t *testing.T) {
	os.Setenv("PHIRA_MP_VERSION", "1.2.3")
	defer os.Unsetenv("PHIRA_MP_VERSION")

	v := version.ReadVersion()
	if v != "1.2.3" {
		t.Errorf("version = %q, want 1.2.3", v)
	}
}

func TestReadVersionDefault(t *testing.T) {
	os.Unsetenv("PHIRA_MP_VERSION")
	v := version.ReadVersion()
	if v != "unknown" {
		t.Errorf("version = %q, want unknown", v)
	}
}
