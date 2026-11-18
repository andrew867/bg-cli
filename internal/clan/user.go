package clan

import "strings"

// DefaultUserID is the configured Pet Simulator 99 user id used for
// contribution and rank when the profile and the --user-id flag are empty.
const DefaultUserID = "4092648666"

// ResolveUserID picks the flag, then the configured profile value, then DefaultUserID.
func ResolveUserID(flag, configured string) string {
	if s := strings.TrimSpace(flag); s != "" {
		return s
	}
	if s := strings.TrimSpace(configured); s != "" {
		return s
	}
	return DefaultUserID
}
