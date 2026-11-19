package snapshot

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// zipEntryTime is the conventional DOS lower bound. Every member uses this
// instant so a source mtime cannot change the archive bytes.
var zipEntryTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// zipEntryMode is the portable regular-file mode stored for every member.
// Source permissions, including the executable bit, are not copied.
const zipEntryMode os.FileMode = 0o644

// ZipMember is one file the packager may put at the root of a snapshot ZIP.
type ZipMember struct {
	Name string
	Path string
}

// ZipReport is the verified ZIP. SHA256 is the full digest of the archive.
type ZipReport struct {
	Path     string
	Name     string
	Bytes    int64
	SHA256   string
	Members  []string
	Missing  []string
	Complete bool
}

// MembersFromManifest lists snapshot artifacts that exist. It reads the
// manifest file records and the manifest file itself. It does not scan the
// directory. Credential-like names, absolute paths, and ".." are rejected.
func MembersFromManifest(dir string, man Manifest) ([]ZipMember, []string, error) {
	type item struct {
		name string
	}
	var listed []item
	seen := map[string]struct{}{}
	add := func(name string) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		clean, err := safeZipName(name)
		if err != nil {
			return err
		}
		if _, ok := seen[clean]; ok {
			return fmt.Errorf("duplicate snapshot artifact %s", clean)
		}
		seen[clean] = struct{}{}
		listed = append(listed, item{name: clean})
		return nil
	}
	for _, f := range man.Files {
		name := f.RelPath
		if name == "" {
			name = f.Name
		}
		if err := add(name); err != nil {
			return nil, nil, err
		}
	}
	if err := add(man.ManifestFile); err != nil {
		return nil, nil, err
	}
	var members []ZipMember
	var missing []string
	for _, it := range listed {
		full, err := ResolveFilePath(dir, FileRecord{Name: it.name, RelPath: it.name})
		if err != nil {
			return nil, nil, err
		}
		info, err := os.Lstat(full)
		if err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, it.name)
				continue
			}
			return nil, nil, err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("refusing to pack %s", it.name)
		}
		members = append(members, ZipMember{Name: it.name, Path: full})
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	return members, missing, nil
}

func safeZipName(name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute paths are not allowed in a snapshot ZIP: %s", name)
	}
	clean := filepath.Clean(name)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("empty snapshot artifact name")
	}
	if strings.HasPrefix(clean, "..") || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal rejected: %s", name)
	}
	if strings.ContainsAny(clean, `/\`) {
		return "", fmt.Errorf("snapshot ZIP members must be file names, not paths: %s", name)
	}
	if deniedArtifactName(clean) {
		return "", fmt.Errorf("refusing to pack credential-like artifact %s", clean)
	}
	return clean, nil
}

func deniedArtifactName(name string) bool {
	low := strings.ToLower(filepath.Base(name))
	if strings.HasPrefix(low, ".") {
		return true
	}
	switch low {
	case "config.json", "credentials", "credentials.json", "cookies", "cookies.txt", "id_rsa":
		return true
	}
	if strings.HasSuffix(low, ".env") || strings.HasSuffix(low, ".pem") || strings.HasSuffix(low, ".key") || strings.HasSuffix(low, ".p12") || strings.HasSuffix(low, ".pfx") {
		return true
	}
	for _, frag := range []string{"token", "secret", "password", "credential", "cookie"} {
		if strings.Contains(low, frag) {
			return true
		}
	}
	return false
}

// ValidateZipPath rejects a destination whose file name looks like a secret.
func ValidateZipPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("--zip-path is empty")
	}
	if deniedArtifactName(filepath.Base(path)) {
		return fmt.Errorf("refusing ZIP name %s", filepath.Base(path))
	}
	return nil
}

// Pack writes members into dest atomically. The archive is verified before
// the final name is published. rename may be nil. A failed rename leaves no
// final file and removes the temporary archive.
//
// An existing dest is an error. Pack does not replace it.
// Member headers use a fixed timestamp and mode, so identical names and
// bytes produce identical archive bytes within this implementation.
func Pack(dest string, members []ZipMember, rename func(oldPath, newPath string) error) (ZipReport, error) {
	if len(members) == 0 {
		return ZipReport{}, fmt.Errorf("no snapshot artifacts to pack")
	}
	if rename == nil {
		rename = os.Rename
	}
	dest = filepath.Clean(dest)
	names := make([]string, 0, len(members))
	seen := map[string]struct{}{}
	for i := range members {
		clean, err := safeZipName(members[i].Name)
		if err != nil {
			return ZipReport{}, err
		}
		if _, ok := seen[clean]; ok {
			return ZipReport{}, fmt.Errorf("duplicate snapshot artifact %s", clean)
		}
		seen[clean] = struct{}{}
		if sameFile(members[i].Path, dest) {
			return ZipReport{}, fmt.Errorf("refusing to pack the ZIP into itself")
		}
		members[i].Name = clean
		names = append(names, clean)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	sort.Strings(names)

	if _, err := os.Lstat(dest); err == nil {
		return ZipReport{}, fmt.Errorf("ZIP destination already exists: %s", dest)
	} else if !os.IsNotExist(err) {
		return ZipReport{}, err
	}

	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".bg-cli-zip-*.tmp")
	if err != nil {
		return ZipReport{}, err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()
	if err := writeZip(tmp, members); err != nil {
		_ = tmp.Close()
		return ZipReport{}, err
	}
	if err := tmp.Close(); err != nil {
		return ZipReport{}, err
	}
	if err := verifyZip(tmpName, names); err != nil {
		return ZipReport{}, err
	}
	sum, size, err := hashFile(tmpName)
	if err != nil {
		return ZipReport{}, err
	}
	if err := rename(tmpName, dest); err != nil {
		return ZipReport{}, err
	}
	success = true
	info, err := os.Stat(dest)
	if err != nil {
		return ZipReport{}, err
	}
	return ZipReport{
		Path:     dest,
		Name:     filepath.Base(dest),
		Bytes:    info.Size(),
		SHA256:   sum,
		Members:  names,
		Complete: true,
	}, sizeCheck(size, info.Size())
}

func sizeCheck(hashed, actual int64) error {
	if hashed != actual {
		return fmt.Errorf("zip size changed after publish: hashed %d bytes, now %d", hashed, actual)
	}
	return nil
}

func writeZip(w io.Writer, members []ZipMember) error {
	zw := zip.NewWriter(w)
	for _, m := range members {
		info, err := os.Lstat(m.Path)
		if err != nil {
			_ = zw.Close()
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			_ = zw.Close()
			return fmt.Errorf("refusing to pack %s", m.Name)
		}
		hdr := &zip.FileHeader{
			Name:    m.Name,
			Method:  zip.Deflate,
			Comment: "",
		}
		hdr.SetModTime(zipEntryTime)
		hdr.SetMode(zipEntryMode)
		hdr.Extra = nil
		entry, err := zw.CreateHeader(hdr)
		if err != nil {
			_ = zw.Close()
			return err
		}
		f, err := os.Open(m.Path)
		if err != nil {
			_ = zw.Close()
			return err
		}
		_, copyErr := io.Copy(entry, f)
		closeErr := f.Close()
		if copyErr != nil {
			_ = zw.Close()
			return copyErr
		}
		if closeErr != nil {
			_ = zw.Close()
			return closeErr
		}
	}
	return zw.Close()
}

func verifyZip(path string, want []string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("reopen zip: %w", err)
	}
	defer r.Close()
	if len(r.File) != len(want) {
		return fmt.Errorf("zip member count %d, want %d", len(r.File), len(want))
	}
	got := make([]string, 0, len(r.File))
	for _, f := range r.File {
		got = append(got, f.Name)
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("zip member %s: %w", f.Name, err)
		}
		_, copyErr := io.Copy(io.Discard, rc)
		closeErr := rc.Close()
		if copyErr != nil {
			return fmt.Errorf("zip member %s: %w", f.Name, copyErr)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	sort.Strings(got)
	for i := range want {
		if got[i] != want[i] {
			return fmt.Errorf("zip members %v, want %v", got, want)
		}
	}
	return nil
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func sameFile(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return aa == bb
}
