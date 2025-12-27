package envutil

import (
	"os"
	"strings"
)

// DevModeEnabled returns true only for explicit opt-in values ("1", "true", "yes"),
// trimmed and case-insensitive. This is intentionally strict to avoid accidental
// enabling of developer-only capabilities.
func DevModeEnabled() bool {
	return isTruthyStrict(os.Getenv("MCP_LENS_DEV_MODE"))
}

func isTruthyStrict(v string) bool {
	v = strings.TrimSpace(v)
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
