package presets

import (
	"os"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/testutil"
)

func TestMain(m *testing.M) {
	_ = testutil.LoadDotEnv()
	os.Exit(m.Run())
}
