// Tests for the canonical package. Each test works inside its own temp
// directory, so the suite is isolated and offline.
package canonical_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/djd39448/DevCore/internal/canonical"
)

// openTestStore returns a Store rooted at a fresh temp directory.
func openTestStore(t *testing.T) *canonical.Store {
	t.Helper()
	store, err := canonical.Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store
}

func TestOpenRejectsAFile(t *testing.T) {
	t.Parallel()
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
	if _, err := canonical.Open(file); err == nil {
		t.Fatal("Open accepted a regular file as the root, want an error")
	}
}

func TestWriteThenRead(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	const body = "# Architecture\n\nThe engine orchestrates agents.\n"
	if err := store.Write("architecture/engine.md", body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := store.Read("architecture/engine.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != body {
		t.Fatalf("Read returned %q, want %q", got, body)
	}
}

func TestWriteStampsLastUpdatedInFrontmatter(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	doc := "---\ntitle: Contract\nlast_updated: 2020-01-01\n---\n\nbody\n"
	if err := store.Write("contract/api.md", doc); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := store.Read("contract/api.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if strings.Contains(got, "2020-01-01") {
		t.Fatal("Write left the stale last_updated date in place")
	}
	if !strings.Contains(got, "last_updated: ") {
		t.Fatal("Write removed the last_updated field entirely")
	}
}

func TestWriteLeavesPlainContentUnchanged(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	const plain = "no frontmatter here\n"
	if err := store.Write("domain/notes.md", plain); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := store.Read("domain/notes.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != plain {
		t.Fatalf("Write changed frontmatter-less content: got %q", got)
	}
}

func TestListReturnsMarkdownFilesSorted(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	for _, p := range []string{"b/second.md", "a/first.md", "ignore.txt"} {
		if err := store.Write(p, "x"); err != nil {
			t.Fatalf("Write(%s): %v", p, err)
		}
	}
	docs, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"a/first.md", "b/second.md"}
	if len(docs) != len(want) || docs[0] != want[0] || docs[1] != want[1] {
		t.Fatalf("List = %v, want %v (only .md files, sorted)", docs, want)
	}
}

func TestPathTraversalIsRejected(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	for _, bad := range []string{"../escape.md", "../../etc/passwd", ""} {
		if _, err := store.Read(bad); err == nil {
			t.Errorf("Read(%q) was allowed, want a rejection", bad)
		}
		if err := store.Write(bad, "x"); err == nil {
			t.Errorf("Write(%q) was allowed, want a rejection", bad)
		}
	}
}

func TestWriteInsertsLastUpdatedWhenAbsent(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	doc := "---\ntitle: Spec\n---\n\nbody\n"
	if err := store.Write("domain/spec.md", doc); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := store.Read("domain/spec.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !strings.Contains(got, "last_updated: ") {
		t.Fatal("Write did not insert a last_updated field into frontmatter that lacked one")
	}
}

func TestWriteLeavesUnterminatedFrontmatterUnchanged(t *testing.T) {
	t.Parallel()
	store := openTestStore(t)

	// An opening --- with no closing --- is not valid frontmatter; leave it be.
	doc := "---\ntitle: Spec\nbody with no closing fence\n"
	if err := store.Write("domain/broken.md", doc); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := store.Read("domain/broken.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != doc {
		t.Fatalf("Write altered content that had unterminated frontmatter: got %q", got)
	}
}
