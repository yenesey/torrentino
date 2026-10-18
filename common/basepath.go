package common

import (
	"fmt"
	"strings"
)

// NormalizeBasePath validates and normalizes web.base-path. The result is "/"
// or a path with a leading slash and no trailing slash. Query strings,
// fragments, empty segments, "." and ".." are rejected rather than silently
// repaired, because every URL, cookie path and redirect is built from this
// value.
func NormalizeBasePath(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "/", nil
	}
	if !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("must start with /")
	}
	if strings.ContainsAny(value, "?#\\") {
		return "", fmt.Errorf("must not contain a query, fragment or backslash")
	}
	if strings.Contains(value, " ") {
		return "", fmt.Errorf("must not contain spaces")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("must not contain . or .. segments")
		}
	}
	if strings.Contains(value, "//") {
		return "", fmt.Errorf("must not contain empty segments")
	}
	value = strings.TrimRight(value, "/")
	if value == "" {
		return "/", nil
	}
	return value, nil
}
