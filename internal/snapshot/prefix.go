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
	"currencies-csv",
}

// FileName builds "{stamp}-{prefix}-{suffix}" or "{stamp}-{suffixPart}-{prefix}-{suffix}" with collision.
func FileName(stamp, collision, prefix, suffix string) string {
	if collision == "" {
		return stamp + "-" + prefix + "-" + suffix
	}
	return stamp + "-" + collision + "-" + prefix + "-" + suffix
}

// AllocateStamp finds a local stamp (+ optional collision suffix) that does not
// collide with any existing completed payload or manifest in dir.
func AllocateStamp(dir, stamp, prefix string) (fullStamp string, collision string, err error) {
	if err := ValidatePrefix(prefix); err != nil {
		return "", "", err
	}
	for i := 0; i <= 9999; i++ {
		coll := ""
		if i > 0 {
			coll = fmt.Sprintf("%04d", i)
		}
		names := []string{
			FileName(stamp, coll, prefix, "inv.json"),
			FileName(stamp, coll, prefix, "prof.json"),
			FileName(stamp, coll, prefix, "extprof.json"),
			FileName(stamp, coll, prefix, "inventory.csv"),
			FileName(stamp, coll, prefix, "currencies.csv"),
			FileName(stamp, coll, prefix, "manifest.json"),
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
