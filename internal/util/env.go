package util

import "strings"

// EnvBool returns (value, ok). ok=false if the env string is empty.
// Accepts: 1/0, true/false, t/f, yes/no, y/n (case-insensitive).
func EnvBool(v string) (bool, bool) {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return false, false
	}
	switch v {
	case "1", "true", "t", "yes", "y":
		return true, true
	case "0", "false", "f", "no", "n":
		return false, true
	default:
		return false, true
	}
}
