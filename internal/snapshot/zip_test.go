package snapshot

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
