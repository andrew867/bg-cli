package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var prefixRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// ValidatePrefix rejects empty, path-like, or otherwise unsafe filename prefixes.
func ValidatePrefix(prefix string) error {
	p := strings.TrimSpace(prefix)
	if p == "" {
		return fmt.Errorf("--prefix must not be empty")
	}
	if p != prefix {
		return fmt.Errorf("--prefix must not have leading or trailing whitespace")
	}
	if strings.ContainsAny(p, `/\`) || strings.Contains(p, "..") {
		return fmt.Errorf("--prefix must not contain path separators or traversal sequences")
	}
	if !prefixRE.MatchString(p) {
		return fmt.Errorf("--prefix %q invalid: use 1-64 chars of [A-Za-z0-9_-], starting alphanumeric", p)
	}
	return nil
}

// PayloadKinds are the stable file kinds written by account snapshot.
var PayloadKinds = []string{
	"inventory-json",
	"profile-json",
	"extended-profile-json",
	"inventory-csv",
	"inventory-diff",
	"currencies-csv",
	"clan-json",
	"clan-battle-json",
	"snapshot-summary",
}

// FileName builds "{stamp}-{prefix}-{suffix}" or "{stamp}-{collision}-{prefix}-{suffix}".
func FileName(stamp, collision, prefix, suffix string) string {
	return ProfileFileName(stamp, collision, "", prefix, suffix)
}

// ProfileFileName builds the snapshot file name. An empty profile keeps the
// single-profile shape from FileName. A profile inserts that alias before the
// prefix: "{stamp}-{profile}-{prefix}-{suffix}", with a collision suffix
// before the profile when one was allocated.
func ProfileFileName(stamp, collision, profile, prefix, suffix string) string {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		if collision == "" {
			return stamp + "-" + prefix + "-" + suffix
		}
		return stamp + "-" + collision + "-" + prefix + "-" + suffix
	}
	if collision == "" {
		return stamp + "-" + profile + "-" + prefix + "-" + suffix
	}
	return stamp + "-" + collision + "-" + profile + "-" + prefix + "-" + suffix
}

// AllocateStamp finds a local stamp (+ optional collision suffix) that does not
// collide with any existing completed payload or manifest in dir.
func AllocateStamp(dir, stamp, prefix string) (fullStamp string, collision string, err error) {
	return AllocateNamedStamp(dir, stamp, prefix, "")
}

// AllocateNamedStamp is AllocateStamp for the multi-profile file name.
// profile is empty when the config has a single profile.
func AllocateNamedStamp(dir, stamp, prefix, profile string) (fullStamp string, collision string, err error) {
	if err := ValidatePrefix(prefix); err != nil {
		return "", "", err
	}
	profile = strings.TrimSpace(profile)
	if profile != "" {
		if strings.ContainsAny(profile, `/\`) || strings.Contains(profile, "..") || !prefixRE.MatchString(profile) {
			return "", "", fmt.Errorf("profile %q cannot be used in a file name", profile)
		}
	}
	for i := 0; i <= 9999; i++ {
		coll := ""
		if i > 0 {
			coll = fmt.Sprintf("%04d", i)
		}
		manifestName := ProfileFileName(stamp, coll, profile, prefix, "manifest.json")
		names := []string{
			ProfileFileName(stamp, coll, profile, prefix, "inv.json"),
			ProfileFileName(stamp, coll, profile, prefix, "prof.json"),
			ProfileFileName(stamp, coll, profile, prefix, "extprof.json"),
			ProfileFileName(stamp, coll, profile, prefix, "inventory.csv"),
			ProfileFileName(stamp, coll, profile, prefix, "currencies.csv"),
			ProfileFileName(stamp, coll, profile, prefix, "clan.json"),
			ProfileFileName(stamp, coll, profile, prefix, "clanbattle.json"),
			ProfileFileName(stamp, coll, profile, prefix, "summary.json"),
			ProfileFileName(stamp, coll, profile, prefix, "inventory-diff.json"),
			manifestName,
			strings.TrimSuffix(manifestName, "-manifest.json") + ".zip",
		}
		conflict := false
		for _, n := range names {
			if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
				conflict = true
				break
			} else if err != nil && !os.IsNotExist(err) {
				return "", "", err
			}
		}
		if !conflict {
			return stamp, coll, nil
		}
	}
	return "", "", fmt.Errorf("could not allocate non-colliding snapshot name under stamp %s", stamp)
}
