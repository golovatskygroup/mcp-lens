package tools

import (
	"os"
	"strings"
)

func devModeEnabled() bool {
	v := strings.TrimSpace(os.Getenv("MCP_LENS_DEV_MODE"))
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
