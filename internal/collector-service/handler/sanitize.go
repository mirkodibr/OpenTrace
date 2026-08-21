package handler

import (
	"fmt"
	"strings"
)

// Sanitisation limits for user-supplied log_attributes. The collector sits
// on the trust boundary: no client (including our own SDKs) is trusted to
// respect these, so they are enforced here regardless of SDK version.
const (
	maxAttrDepth    = 5     // deeper nesting is flattened to dot-notation keys
	maxAttrKeys     = 200   // total keys across all nesting levels
	maxAttrKeyLen   = 255   // key characters
	maxAttrValueLen = 32768 // string value characters
)

// reservedAttrPrefixes are attribute namespaces the platform populates from
// resource metadata; user attributes must not spoof them.
var reservedAttrPrefixes = []string{"host.", "service.", "process."}

// sanitizeAttributes enforces the attribute limits on a raw log_attributes
// map. It returns the sanitised map and one warning per corrective action
// taken. The input map is not modified.
func sanitizeAttributes(raw map[string]any) (map[string]any, []string) {
	if len(raw) == 0 {
		return raw, nil
	}
	s := &sanitizer{root: make(map[string]any, len(raw))}
	s.sanitizeMap(s.root, raw, "", 1)
	return s.root, s.warnings
}

type sanitizer struct {
	root     map[string]any // top-level output; flattened deep keys land here
	warnings []string
	keyCount int
}

// sanitizeMap sanitises m into dst. path is the dotted prefix ("" at the
// root); depth is the current nesting level (root = 1).
func (s *sanitizer) sanitizeMap(dst, m map[string]any, path string, depth int) {
	for key, value := range m {
		cleanKey := s.cleanKey(key)
		fullPath := cleanKey
		if path != "" {
			fullPath = path + "." + cleanKey
		}

		// Reserved namespaces apply to top-level keys only.
		if path == "" {
			if reserved := matchReservedPrefix(cleanKey); reserved != "" {
				s.warnf("dropped attribute %q: %q is a reserved resource namespace", key, reserved)
				continue
			}
		}

		if child, isMap := value.(map[string]any); isMap {
			if depth >= maxAttrDepth {
				// Too deep: express every leaf below as a single
				// dot-notation key at the top level (see the prompt's
				// canonical example a.b.c.d.e.f -> one root key).
				s.warnf("flattened attribute %q: nesting exceeds %d levels", fullPath, maxAttrDepth)
				s.flatten(child, fullPath)
				continue
			}
			if s.keyCount >= maxAttrKeys {
				s.warnf("dropped attribute %q: total key limit (%d) exceeded", fullPath, maxAttrKeys)
				continue
			}
			s.keyCount++
			nested := make(map[string]any, len(child))
			dst[cleanKey] = nested
			s.sanitizeMap(nested, child, fullPath, depth+1)
			continue
		}

		if s.keyCount >= maxAttrKeys {
			s.warnf("dropped attribute %q: total key limit (%d) exceeded", fullPath, maxAttrKeys)
			continue
		}
		s.keyCount++
		dst[cleanKey] = s.cleanValue(value, fullPath)
	}
}

// flatten writes every leaf of m into the ROOT output under dot-notation
// keys rooted at prefix, adding no structural depth.
func (s *sanitizer) flatten(m map[string]any, prefix string) {
	for key, value := range m {
		fullPath := prefix + "." + s.cleanKey(key)
		if child, isMap := value.(map[string]any); isMap {
			s.flatten(child, fullPath)
			continue
		}
		if s.keyCount >= maxAttrKeys {
			s.warnf("dropped attribute %q: total key limit (%d) exceeded", fullPath, maxAttrKeys)
			continue
		}
		s.keyCount++
		s.root[fullPath] = s.cleanValue(value, fullPath)
	}
}

// cleanKey enforces key length and the [a-zA-Z][a-zA-Z0-9_.]* charset,
// replacing invalid characters with underscores.
func (s *sanitizer) cleanKey(key string) string {
	if len(key) > maxAttrKeyLen {
		s.warnf("truncated attribute key %q… to %d chars", key[:32], maxAttrKeyLen)
		key = key[:maxAttrKeyLen]
	}
	if len(key) == 0 {
		s.warnf("replaced empty attribute key with _")
		return "_"
	}
	cleaned := []byte(key)
	changed := false
	for i := 0; i < len(cleaned); i++ {
		c := cleaned[i]
		valid := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(i > 0 && ((c >= '0' && c <= '9') || c == '_' || c == '.'))
		if !valid {
			cleaned[i] = '_'
			changed = true
		}
	}
	if changed {
		s.warnf("rewrote attribute key %q to %q (invalid characters)", key, cleaned)
	}
	return string(cleaned)
}

// cleanValue truncates oversized string values.
func (s *sanitizer) cleanValue(value any, path string) any {
	if str, ok := value.(string); ok && len(str) > maxAttrValueLen {
		s.warnf("truncated attribute %q value to %d chars", path, maxAttrValueLen)
		return str[:maxAttrValueLen]
	}
	return value
}

func (s *sanitizer) warnf(format string, args ...any) {
	s.warnings = append(s.warnings, fmt.Sprintf(format, args...))
}

// matchReservedPrefix returns the reserved namespace key starts with, or "".
func matchReservedPrefix(key string) string {
	for _, p := range reservedAttrPrefixes {
		if strings.HasPrefix(key, p) {
			return p
		}
	}
	return ""
}
