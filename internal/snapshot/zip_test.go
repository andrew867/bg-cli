package snapshot

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestZipPacksManifestMembersOnly(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("snap-inv.json", `{"ok":true}`)
	write("snap-manifest.json", `{"files":[]}`)
	write(".env", "TOKEN=super")
	write("config.json", `{"clientSecret":"nope"}`)
	write("credentials.json", "secret")
	write("notes.txt", "unrelated")
	write("token.txt", "access-token")

	members, missing, err := MembersFromManifest(dir, Manifest{
		ManifestFile: "snap-manifest.json",
		Files: []FileRecord{
			{Name: "snap-inv.json", RelPath: "snap-inv.json", OK: true},
			{Name: "missing-prof.json", RelPath: "missing-prof.json", OK: false, Error: "fetch failed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 || missing[0] != "missing-prof.json" {
		t.Fatalf("missing=%v", missing)
	}
	for _, m := range members {
		base := filepath.Base(m.Name)
		if strings.Contains(base, "env") || strings.Contains(base, "config") || strings.Contains(base, "credential") || strings.Contains(base, "token") || base == "notes.txt" {
			t.Fatalf("packed unexpected %s", m.Name)
		}
	}
	dest := filepath.Join(dir, "snap.zip")
	rep, err := Pack(dest, members, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Complete || rep.SHA256 == "" || rep.Bytes == 0 {
		t.Fatalf("report=%+v", rep)
	}
	r, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	got := map[string]bool{}
	for _, f := range r.File {
		got[f.Name] = true
		if strings.Contains(f.Name, "..") || strings.Contains(f.Name, "/") || strings.Contains(f.Name, `\`) {
			t.Fatalf("unsafe member %s", f.Name)
		}
	}
	if !got["snap-inv.json"] || !got["snap-manifest.json"] {
		t.Fatalf("members=%v", got)
	}
	if got["snap.zip"] || got[".env"] || got["config.json"] || got["credentials.json"] || got["notes.txt"] || got["token.txt"] || got["missing-prof.json"] {
		t.Fatalf("unexpected members=%v", got)
	}
	if len(got) != 2 {
		t.Fatalf("count=%d %v", len(got), got)
	}
}

func TestZipRejectsCredentialNamesAndTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := MembersFromManifest(dir, Manifest{
		Files: []FileRecord{{Name: ".env", RelPath: ".env"}},
	})
	if err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("err=%v", err)
	}
	_, _, err = MembersFromManifest(dir, Manifest{
		Files: []FileRecord{{Name: "../.env", RelPath: "../.env"}},
	})
	if err == nil {
		t.Fatal("traversal accepted")
	}
	_, _, err = MembersFromManifest(dir, Manifest{
		Files: []FileRecord{{Name: "/tmp/config.json", RelPath: "/tmp/config.json"}},
	})
	if err == nil {
		t.Fatal("absolute path accepted")
	}
}

func TestZipRenameFailureLeavesNoFinalArchive(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "snap-inv.json")
	if err := os.WriteFile(src, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "snap.zip")
	_, err := Pack(dest, []ZipMember{{Name: "snap-inv.json", Path: src}}, func(string, string) error {
		return errors.New("rename failed")
	})
	if err == nil || !strings.Contains(err.Error(), "rename failed") {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("final zip exists after failed rename")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if strings.HasPrefix(ent.Name(), ".bg-cli-zip-") || strings.HasSuffix(ent.Name(), ".tmp") {
			t.Fatalf("temp left behind: %s", ent.Name())
		}
	}
}

func TestZipBytesIgnoreSourceMetadata(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	bodyA := []byte(`{"eggs":42}`)
	bodyB := []byte("beta-bytes")
	if err := os.WriteFile(a, bodyA, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, bodyB, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Date(1999, 3, 4, 5, 6, 8, 0, time.UTC)
	if err := os.Chtimes(a, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(b, old.Add(48*time.Hour), old.Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	pack := func(name string) ([]byte, string) {
		t.Helper()
		dest := filepath.Join(dir, name)
		members := []ZipMember{
			{Name: "b.json", Path: b},
			{Name: "a.json", Path: a},
		}
		rep, err := Pack(dest, members, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(got)
		if hex.EncodeToString(sum[:]) != rep.SHA256 {
			t.Fatalf("report sha=%s file sha=%s", rep.SHA256, hex.EncodeToString(sum[:]))
		}
		if bytes.Contains(got, []byte(dir)) {
			t.Fatal("archive contains the source directory path")
		}
		assertZipMembers(t, dest, map[string][]byte{"a.json": bodyA, "b.json": bodyB})
		return got, rep.SHA256
	}

	first, sha1 := pack("one.zip")

	later := time.Date(2024, 12, 31, 23, 59, 58, 0, time.UTC)
	if err := os.Chtimes(a, later, later); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(b, later.Add(-72*time.Hour), later.Add(-72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(a, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(b, 0o640); err != nil {
		t.Fatal(err)
	}

	second, sha2 := pack("two.zip")
	if !bytes.Equal(first, second) {
		t.Fatalf("zip bytes differ (%d vs %d) after mtime and mode changes", len(first), len(second))
	}
	if sha1 != sha2 {
		t.Fatalf("sha256 %s vs %s", sha1, sha2)
	}
}

func TestZipRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "snap-inv.json")
	if err := os.WriteFile(src, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "existing.zip")
	const sentinel = "not-a-real-zip"
	if err := os.WriteFile(dest, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Pack(dest, []ZipMember{{Name: "snap-inv.json", Path: src}}, nil)
	if err == nil || !strings.Contains(err.Error(), "ZIP destination already exists:") {
		t.Fatalf("err=%v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Fatalf("destination changed: %q", got)
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if strings.HasPrefix(ent.Name(), ".bg-cli-zip-") || strings.HasSuffix(ent.Name(), ".tmp") {
			t.Fatalf("temp left behind: %s", ent.Name())
		}
	}
}

func assertZipMembers(t *testing.T, path string, want map[string][]byte) {
	t.Helper()
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(r.File) != len(want) {
		t.Fatalf("members=%d want %d", len(r.File), len(want))
	}
	for _, f := range r.File {
		body, ok := want[f.Name]
		if !ok {
			t.Fatalf("unexpected member %s", f.Name)
		}
		if !f.Modified.Equal(zipEntryTime) {
			t.Fatalf("member %s time %s", f.Name, f.Modified)
		}
		if f.Mode().Perm() != zipEntryMode {
			t.Fatalf("member %s mode %o", f.Name, f.Mode().Perm())
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(rc)
		closeErr := rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if !bytes.Equal(got, body) {
			t.Fatalf("member %s bytes %q", f.Name, got)
		}
	}
}
