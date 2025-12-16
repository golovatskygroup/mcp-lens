package router

import "testing"

func TestOutputIncludeExclude(t *testing.T) {
	in := map[string]any{
		"a": map[string]any{
			"b": 1,
			"c": 2,
		},
		"x": 9,
	}

	out, err := ApplyOutputShaping("t", in, &OutputOptions{IncludeFields: []string{"a.b"}})
	if err != nil {
		t.Fatalf("include: %v", err)
	}
	m := out.(map[string]any)
	aa := m["a"].(map[string]any)
	switch v := aa["b"].(type) {
	case int:
		if v != 1 {
			t.Fatalf("expected a.b=1")
		}
	case float64:
		if int(v) != 1 {
			t.Fatalf("expected a.b=1")
		}
	default:
		t.Fatalf("unexpected type for a.b: %T", aa["b"])
	}
	if _, ok := aa["c"]; ok {
		t.Fatalf("expected a.c excluded")
	}

	out2, err := ApplyOutputShaping("t", in, &OutputOptions{ExcludeFields: []string{"a.c"}})
	if err != nil {
		t.Fatalf("exclude: %v", err)
	}
	m2 := out2.(map[string]any)
	a2 := m2["a"].(map[string]any)
	if _, ok := a2["c"]; ok {
		t.Fatalf("expected a.c removed")
	}
}

func TestOutputInvalidPath(t *testing.T) {
	_, err := ApplyOutputShaping("t", map[string]any{"a": 1}, &OutputOptions{IncludeFields: []string{"a["}})
	if err == nil {
		t.Fatalf("expected error")
	}
}
