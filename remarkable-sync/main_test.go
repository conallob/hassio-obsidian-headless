package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRmdoc builds a minimal in-memory .rmdoc bundle with the .content and
// .metadata files real reMarkable documents contain, so the extraction and
// tag-mapping logic can be exercised without a live reMarkable account.
func fakeRmdoc(t *testing.T, contentJSON, metadataJSON string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"doc.content":  contentJSON,
		"doc.metadata": metadataJSON,
	} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := f.Write([]byte(body)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestMapTags(t *testing.T) {
	cases := []struct {
		name      string
		tags      []string
		prefix    string
		extraTags []string
		want      []string
	}{
		{
			name: "no prefix overlays labels unchanged",
			tags: []string{"work", "urgent"},
			want: []string{"work", "urgent"},
		},
		{
			name:   "prefix applied to every label",
			tags:   []string{"mylabel"},
			prefix: "remarkable:",
			want:   []string{"remarkable:mylabel"},
		},
		{
			name:      "extra tags applied even with no native labels",
			tags:      nil,
			extraTags: []string{"remarkable"},
			want:      []string{"remarkable"},
		},
		{
			name:      "prefix and extra tags compose",
			tags:      []string{"work"},
			prefix:    "rm:",
			extraTags: []string{"remarkable"},
			want:      []string{"rm:work", "remarkable"},
		},
		{
			name:      "duplicates between mapped and extra tags are deduped",
			tags:      []string{"remarkable"},
			extraTags: []string{"remarkable"},
			want:      []string{"remarkable"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapTags(tc.tags, tc.prefix, tc.extraTags)
			if !equalSlices(got, tc.want) {
				t.Errorf("mapTags(%v, %q, %v) = %v, want %v", tc.tags, tc.prefix, tc.extraTags, got, tc.want)
			}
		})
	}
}

func TestSplitTags(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"", nil},
		{"remarkable", []string{"remarkable"}},
		{"a, b ,c", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
	}
	for _, tc := range cases {
		got := splitTags(tc.raw)
		if !equalSlices(got, tc.want) {
			t.Errorf("splitTags(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// TestExtractBlobMetadataAndWriteNote exercises the full pipeline a real
// synced document goes through: parsing a fake .rmdoc bundle, mapping tags,
// and rendering the Markdown note -- confirming the mapped tags actually
// land in the frontmatter, without needing a live reMarkable account.
func TestExtractBlobMetadataAndWriteNote(t *testing.T) {
	blob := fakeRmdoc(t,
		`{"fileType":"notebook","pageCount":3,"tags":[{"name":"work"},{"name":"urgent"}]}`,
		`{"visibleName":"My Notebook","lastModified":"1700000000000","pinned":true}`,
	)

	meta := extractBlobMetadata(blob)
	if meta.VisibleName != "My Notebook" {
		t.Fatalf("VisibleName = %q, want %q", meta.VisibleName, "My Notebook")
	}
	if !meta.Pinned {
		t.Fatal("Pinned = false, want true")
	}
	if !equalSlices(meta.Tags, []string{"work", "urgent"}) {
		t.Fatalf("Tags = %v, want [work urgent]", meta.Tags)
	}

	meta.Tags = mapTags(meta.Tags, "rm:", []string{"remarkable"})
	want := []string{"rm:work", "rm:urgent", "remarkable"}
	if !equalSlices(meta.Tags, want) {
		t.Fatalf("mapped Tags = %v, want %v", meta.Tags, want)
	}

	dir := t.TempDir()
	notePath := filepath.Join(dir, "note.md")
	if err := writeNote(notePath, meta.VisibleName, "folder/My Notebook", meta.Modified, meta, ""); err != nil {
		t.Fatalf("writeNote: %v", err)
	}
	out, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	body := string(out)
	for _, tag := range want {
		if !strings.Contains(body, "  - "+tag+"\n") {
			t.Errorf("note frontmatter missing tag %q; body:\n%s", tag, body)
		}
	}
	if !strings.Contains(body, "bookmarked: true") {
		t.Errorf("note frontmatter missing bookmarked: true; body:\n%s", body)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
