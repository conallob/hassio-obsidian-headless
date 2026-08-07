package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installFakeRmapi puts a no-op `rmapi` executable on PATH for the duration
// of the test, so rmapicli.SyncTree's exec.Command("rmapi", "mget", ...)
// succeeds without a real reMarkable account or network access. The mirror
// directory is expected to be pre-seeded with fixture .rmdoc files by the
// caller (as a real `rmapi mget` would have populated it) -- this stub only
// needs to exit 0 so runSync proceeds to process what's already there.
func installFakeRmapi(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake rmapi shim uses a POSIX shell script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "rmapi")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake rmapi: %v", err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
}

func seedRmdoc(t *testing.T, path, contentJSON, metadataJSON string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	blob := fakeRmdoc(t, contentJSON, metadataJSON)
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("write rmdoc: %v", err)
	}
}

// TestRunSyncEndToEnd exercises the real runSync pipeline end to end --
// rmapicli.SyncTree (against a stubbed rmapi), the mirror walk, per-document
// tag mapping, Markdown/frontmatter rendering, and the vault index -- against
// a pre-seeded mirror standing in for what a real rmapi mget would produce.
// This is the closest CI can get to a real reMarkable sync without a live
// account, and is what actually caught issues a purely unit-level test
// wouldn't (wrong relative paths, index omissions, incremental stamp bugs).
func TestRunSyncEndToEnd(t *testing.T) {
	installFakeRmapi(t)

	root := t.TempDir()
	mirrorDir := filepath.Join(root, "mirror")
	stampDir := filepath.Join(root, "stamps")
	destRoot := filepath.Join(root, "vault", "reMarkable")

	// Seed the mirror as if rmapi had already fetched two documents: one
	// nested in a folder with a native label, one at the root with none.
	seedRmdoc(t, filepath.Join(mirrorDir, "Work", "Notes.rmdoc"),
		`{"fileType":"notebook","pageCount":2,"tags":[{"name":"work"}]}`,
		`{"visibleName":"Notes","lastModified":"1700000000000","pinned":false}`)
	seedRmdoc(t, filepath.Join(mirrorDir, "Ideas.rmdoc"),
		`{"fileType":"notebook","pageCount":1,"tags":[]}`,
		`{"visibleName":"Ideas","lastModified":"1700000001000","pinned":true}`)

	if err := runSync(mirrorDir, stampDir, destRoot, nil, "rm:", []string{"remarkable"}); err != nil {
		t.Fatalf("runSync: %v", err)
	}

	note1, err := os.ReadFile(filepath.Join(destRoot, "Work", "Notes.md"))
	if err != nil {
		t.Fatalf("read Notes.md: %v", err)
	}
	if !strings.Contains(string(note1), "  - rm:work\n") {
		t.Errorf("Notes.md missing prefixed tag rm:work:\n%s", note1)
	}
	if !strings.Contains(string(note1), "  - remarkable\n") {
		t.Errorf("Notes.md missing extra tag remarkable:\n%s", note1)
	}

	note2, err := os.ReadFile(filepath.Join(destRoot, "Ideas.md"))
	if err != nil {
		t.Fatalf("read Ideas.md: %v", err)
	}
	if !strings.Contains(string(note2), "  - remarkable\n") {
		t.Errorf("Ideas.md (no native labels) missing extra tag:\n%s", note2)
	}
	if !strings.Contains(string(note2), "bookmarked: true") {
		t.Errorf("Ideas.md missing bookmarked: true from pinned metadata:\n%s", note2)
	}

	index, err := os.ReadFile(filepath.Join(destRoot, "index.md"))
	if err != nil {
		t.Fatalf("read index.md: %v", err)
	}
	if !strings.Contains(string(index), "Work/Notes") {
		t.Errorf("index.md missing Work/Notes entry:\n%s", index)
	}
	if !strings.Contains(string(index), "Ideas") {
		t.Errorf("index.md missing Ideas entry:\n%s", index)
	}

	// Second run against an unchanged mirror: mtimes haven't moved, so the
	// per-document stamp cache should treat everything as already synced.
	if err := runSync(mirrorDir, stampDir, destRoot, nil, "rm:", []string{"remarkable"}); err != nil {
		t.Fatalf("second runSync (no-op incremental pass): %v", err)
	}
}
