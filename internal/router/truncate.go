package router

import (
	"encoding/json"
)

// TruncateExecutedStepsForLLM replaces large results with lightweight previews (or preserves artifact refs).
func TruncateExecutedStepsForLLM(steps []ExecutedStep) []ExecutedStep {
	const maxResultBytes = 12 * 1024

	out := make([]ExecutedStep, 0, len(steps))
	for _, st := range steps {
		if st.Result == nil {
			out = append(out, st)
			continue
		}

		// If already an artifact reference, keep as-is.
		if m, ok := st.Result.(map[string]any); ok {
			if _, ok := m["artifact_uri"]; ok {
				out = append(out, st)
				continue
			}
		}

		b, err := json.Marshal(st.Result)
		if err == nil && len(b) > maxResultBytes {
			st.Result = map[string]any{
				"truncated": true,
				"bytes":     len(b),
				"preview":   string(b[:maxResultBytes]) + "…",
			}
		}
		out = append(out, st)
	}
	return out
}
