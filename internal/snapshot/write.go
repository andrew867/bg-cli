package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteResult is the outcome of an atomic payload write.
type WriteResult struct {
	Bytes  int64
	SHA256 string
	Path   string
}

// AtomicWrite writes via temp file then rename. Computes SHA-256 of content.
// On Windows, removes a pre-existing destination before rename.
func AtomicWrite(path string, write func(w io.Writer) error) (WriteResult, error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".bg-cli-snap-*.tmp")
	if err != nil {
		return WriteResult{}, err
	}
	tmpName := tmp.Name()
	h := sha256.New()
	mw := io.MultiWriter(tmp, h)
	var wr WriteResult
	if err := write(mw); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return WriteResult{}, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return WriteResult{}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return WriteResult{}, err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return WriteResult{}, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return WriteResult{}, err
	}
	wr.Bytes = fi.Size()
	wr.SHA256 = hex.EncodeToString(h.Sum(nil))
	wr.Path = path
	return wr, nil
}

// VerifyFile checks size and optional SHA-256 against a FileRecord.
func VerifyFile(path string, rec FileRecord) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if rec.Bytes > 0 && fi.Size() != rec.Bytes {
		return fmt.Errorf("%s: size mismatch recorded=%d actual=%d", rec.Name, rec.Bytes, fi.Size())
	}
	if rec.SHA256 == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if sum != rec.SHA256 {
		return fmt.Errorf("%s: sha256 mismatch", rec.Name)
	}
	return nil
}
