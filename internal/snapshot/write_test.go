package snapshot

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteLeavesNoPartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "summary.json")
	if _, err := AtomicWrite(path, func(w io.Writer) error {
		return errors.New("boom")
	}); err == nil {
		t.Fatal("expected write error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("partial summary was left behind")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Fatalf("temp files left: %v", ents)
	}
	wr, err := AtomicWrite(path, func(w io.Writer) error {
		_, err := w.Write([]byte("{}\n"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if wr.Bytes != 3 || wr.SHA256 == "" {
		t.Fatalf("result=%+v", wr)
	}
}
