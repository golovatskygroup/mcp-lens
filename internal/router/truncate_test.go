package router

import "testing"

func TestTruncateExecutedStepsForLLM(t *testing.T) {
	steps := []ExecutedStep{
		{Name: "t", OK: true, Result: map[string]any{"x": make([]any, 0)}},
		{Name: "big", OK: true, Result: map[string]any{"payload": string(make([]byte, 20000))}},
		{Name: "art", OK: true, Result: map[string]any{"artifact_uri": "artifact://abc", "preview": "x"}},
	}
	out := TruncateExecutedStepsForLLM(steps)
	if m, ok := out[1].Result.(map[string]any); !ok || m["truncated"] != true {
		t.Fatalf("expected truncated result")
	}
	if m, ok := out[2].Result.(map[string]any); !ok || m["artifact_uri"] != "artifact://abc" {
		t.Fatalf("expected artifact ref preserved")
	}
}
