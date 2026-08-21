package handler

import (
	"strings"
	"testing"
)

func TestSanitizeAttributes_PassesCleanInput(t *testing.T) {
	in := map[string]any{
		"user_id":  "u_123",
		"count":    3.0,
		"nested":   map[string]any{"level": "gold"},
		"is_admin": true,
	}
	out, warnings := sanitizeAttributes(in)
	if len(warnings) != 0 {
		t.Fatalf("clean input produced warnings: %v", warnings)
	}
	if out["user_id"] != "u_123" || out["count"] != 3.0 || out["is_admin"] != true {
		t.Errorf("clean values altered: %v", out)
	}
	nested, ok := out["nested"].(map[string]any)
	if !ok || nested["level"] != "gold" {
		t.Errorf("nested map altered: %v", out["nested"])
	}
}

func TestSanitizeAttributes_Empty(t *testing.T) {
	out, warnings := sanitizeAttributes(nil)
	if out != nil || warnings != nil {
		t.Errorf("nil input should pass through unchanged")
	}
	out, warnings = sanitizeAttributes(map[string]any{})
	if len(out) != 0 || warnings != nil {
		t.Errorf("empty input should pass through unchanged")
	}
}

func TestSanitizeAttributes_FlattensDeepNesting(t *testing.T) {
	// 6 levels deep (a>b>c>d>e>f); depth limit is 5, so the subtree beyond
	// the limit collapses to a single dot-notation key at the top level
	// (matches the prompt's canonical a.b.c.d.e.f example).
	in := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": map[string]any{
						"e": map[string]any{
							"f": "too deep",
						},
					},
				},
			},
		},
	}
	out, warnings := sanitizeAttributes(in)
	if !hasWarning(warnings, "nesting exceeds") {
		t.Fatalf("expected a nesting warning, got: %v", warnings)
	}
	if out["a.b.c.d.e.f"] != "too deep" {
		t.Errorf("deep leaf not flattened to top-level dot key; got: %#v", out)
	}
}

func TestSanitizeAttributes_PreservesNestingUpToLimit(t *testing.T) {
	// Exactly at the boundary: a>b>c>d>e with a scalar leaf stays nested.
	in := map[string]any{
		"a": map[string]any{"b": map[string]any{"c": map[string]any{
			"d": map[string]any{"e": "ok"},
		}}},
	}
	out, warnings := sanitizeAttributes(in)
	if hasWarning(warnings, "nesting exceeds") {
		t.Fatalf("5-level nesting should not warn: %v", warnings)
	}
	m := out
	for _, k := range []string{"a", "b", "c", "d"} {
		next, ok := m[k].(map[string]any)
		if !ok {
			t.Fatalf("level %q not preserved as a map: %#v", k, out)
		}
		m = next
	}
	if m["e"] != "ok" {
		t.Errorf("leaf at depth 5 altered: %v", m)
	}
}

func TestSanitizeAttributes_EnforcesKeyCharset(t *testing.T) {
	in := map[string]any{
		"valid_key":  "ok",
		"has spaces": "x",
		"has-dashes": "y",
		"1leading":   "z", // digit first is invalid
		"emoji😀here": "w",
	}
	out, warnings := sanitizeAttributes(in)
	if !hasWarning(warnings, "invalid characters") {
		t.Fatalf("expected invalid-character warnings, got: %v", warnings)
	}
	if _, ok := out["has_spaces"]; !ok {
		t.Errorf("space not rewritten to underscore: %v", out)
	}
	if _, ok := out["has_dashes"]; !ok {
		t.Errorf("dash not rewritten to underscore: %v", out)
	}
	if _, ok := out["_leading"]; !ok {
		t.Errorf("leading digit not rewritten: %v", out)
	}
	if out["valid_key"] != "ok" {
		t.Errorf("valid key altered: %v", out)
	}
}

func TestSanitizeAttributes_TruncatesLongValue(t *testing.T) {
	long := strings.Repeat("x", maxAttrValueLen+100)
	out, warnings := sanitizeAttributes(map[string]any{"body": long})
	if !hasWarning(warnings, "truncated") {
		t.Fatalf("expected truncation warning, got: %v", warnings)
	}
	if got := out["body"].(string); len(got) != maxAttrValueLen {
		t.Errorf("value length = %d, want %d", len(got), maxAttrValueLen)
	}
}

func TestSanitizeAttributes_DropsReservedPrefixes(t *testing.T) {
	in := map[string]any{
		"service.name": "spoofed",
		"host.name":    "spoofed",
		"process.pid":  1234.0,
		"user_id":      "kept",
	}
	out, warnings := sanitizeAttributes(in)
	for _, k := range []string{"service.name", "host.name", "process.pid"} {
		if _, present := out[k]; present {
			t.Errorf("reserved key %q was not dropped", k)
		}
	}
	if out["user_id"] != "kept" {
		t.Errorf("non-reserved key dropped: %v", out)
	}
	if !hasWarning(warnings, "reserved resource namespace") {
		t.Errorf("expected reserved-namespace warnings, got: %v", warnings)
	}
}

func TestSanitizeAttributes_ReservedPrefixOnlyTopLevel(t *testing.T) {
	// A nested key named service.name is a user attribute, not a resource
	// spoof, so it is preserved.
	in := map[string]any{
		"payload": map[string]any{"service.name": "legit-nested"},
	}
	out, _ := sanitizeAttributes(in)
	nested := out["payload"].(map[string]any)
	if nested["service.name"] != "legit-nested" {
		t.Errorf("nested reserved-looking key wrongly dropped: %v", nested)
	}
}

func TestSanitizeAttributes_EnforcesKeyCountLimit(t *testing.T) {
	in := make(map[string]any, maxAttrKeys+50)
	for i := 0; i < maxAttrKeys+50; i++ {
		in["key_"+pad(i)] = i
	}
	out, warnings := sanitizeAttributes(in)
	if len(out) > maxAttrKeys {
		t.Errorf("output has %d keys, limit is %d", len(out), maxAttrKeys)
	}
	if !hasWarning(warnings, "total key limit") {
		t.Errorf("expected key-limit warnings, got %d warnings", len(warnings))
	}
}

func TestSanitizeAttributes_TruncatesLongKey(t *testing.T) {
	longKey := strings.Repeat("k", maxAttrKeyLen+50)
	out, warnings := sanitizeAttributes(map[string]any{longKey: "v"})
	if !hasWarning(warnings, "truncated attribute key") {
		t.Fatalf("expected key-truncation warning, got: %v", warnings)
	}
	for k := range out {
		if len(k) > maxAttrKeyLen {
			t.Errorf("output key length = %d, want <= %d", len(k), maxAttrKeyLen)
		}
	}
}

func hasWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func pad(i int) string {
	s := ""
	for _, d := range []int{i / 100 % 10, i / 10 % 10, i % 10} {
		s += string(rune('0' + d))
	}
	return s
}
