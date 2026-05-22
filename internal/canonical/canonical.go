// Package canonical is devcore-memory's Tier-1 store: the git-versioned
// markdown/YAML files under .devcore/memory that hold DevCore's durable
// knowledge.
//
// Depends on: the Go standard library only (os, path/filepath).
// Depended on by: internal/memoryserver, which exposes Read, Write, and List
// as MCP tools.
// Why it exists: Tier-1 memory is plain files so it stays diffable and
// reviewable. This package gives controlled, path-safe access to that file
// tree and keeps the last_updated frontmatter stamp current on every write.
package canonical

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// dirPerm and filePerm are the permissions new directories and files are
// created with. Canonical memory is single-user, version-controlled content.
const (
	dirPerm  = 0o750
	filePerm = 0o600
)

// Store is path-safe access to a Tier-1 canonical memory directory.
type Store struct {
	root string
}

// Open returns a Store rooted at dir, which must be an existing directory.
func Open(dir string) (*Store, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("opening canonical memory root %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("canonical memory root %s is not a directory", dir)
	}
	return &Store{root: dir}, nil
}

// Read returns the contents of the canonical file at relPath, which is
// interpreted relative to the memory root.
func (s *Store) Read(relPath string) (string, error) {
	full, err := s.resolve(relPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full) //nolint:gosec // relPath is confined to the root by resolve
	if err != nil {
		return "", fmt.Errorf("reading canonical file %s: %w", relPath, err)
	}
	return string(data), nil
}

// Write replaces the canonical file at relPath with content, creating parent
// directories as needed. If content carries YAML frontmatter, its last_updated
// field is set to today's date.
func (s *Store) Write(relPath, content string) error {
	full, err := s.resolve(relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), dirPerm); err != nil {
		return fmt.Errorf("creating directory for %s: %w", relPath, err)
	}
	stamped := stampLastUpdated(content, time.Now().UTC().Format("2006-01-02"))
	if err := os.WriteFile(full, []byte(stamped), filePerm); err != nil {
		return fmt.Errorf("writing canonical file %s: %w", relPath, err)
	}
	return nil
}

// List returns the relative slash-separated paths of every .md file under the
// memory root, sorted.
func (s *Store) List() ([]string, error) {
	var paths []string
	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	}
	if err := filepath.WalkDir(s.root, walk); err != nil {
		return nil, fmt.Errorf("listing canonical memory: %w", err)
	}
	sort.Strings(paths)
	return paths, nil
}

// resolve joins relPath onto the root and confirms the result stays inside it,
// rejecting any path that would escape via "..".
func (s *Store) resolve(relPath string) (string, error) {
	if strings.TrimSpace(relPath) == "" {
		return "", fmt.Errorf("canonical path is empty")
	}
	full := filepath.Join(s.root, filepath.FromSlash(relPath))
	rel, err := filepath.Rel(s.root, full)
	if err != nil {
		return "", fmt.Errorf("resolving canonical path %s: %w", relPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("canonical path %s escapes the memory root", relPath)
	}
	return full, nil
}

// stampLastUpdated sets the last_updated field in content's YAML frontmatter to
// today. Content with no frontmatter, or an unterminated one, is returned
// unchanged.
func stampLastUpdated(content, today string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content
	}
	closing := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closing = i
			break
		}
	}
	if closing < 0 {
		return content
	}

	stamp := "last_updated: " + today
	for i := 1; i < closing; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "last_updated:") {
			lines[i] = stamp
			return strings.Join(lines, "\n")
		}
	}
	// No last_updated field present — insert one just before the closing ---.
	updated := append([]string{}, lines[:closing]...)
	updated = append(updated, stamp)
	updated = append(updated, lines[closing:]...)
	return strings.Join(updated, "\n")
}
