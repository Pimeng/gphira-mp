package version

import (
	"os"
	"strings"
)

// version is set at build time via -ldflags.
var version string

// ReadVersion returns the application version.
// Priority: build-time ldflags > PHIRA_MP_VERSION env > "unknown"
func ReadVersion() string {
	if version != "" {
		return strings.TrimSpace(version)
	}
	if v := os.Getenv("PHIRA_MP_VERSION"); v != "" {
		return strings.TrimSpace(v)
	}
	return "unknown"
}
