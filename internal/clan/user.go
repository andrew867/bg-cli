package clan

import "strings"

// ResolveUserID returns the explicit flag, then the profile ps99UserId.
// The third result is false when neither is set. There is no built-in user id.
func ResolveUserID(flag, configured string) (string, bool) {
	if s := strings.TrimSpace(flag); s != "" {
		return s, true
	}
	if s := strings.TrimSpace(configured); s != "" {
		return s, true
	}
	return "", false
}
