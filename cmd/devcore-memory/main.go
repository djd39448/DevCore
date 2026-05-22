// Command devcore-memory serves DevCore's Tier-1 and Tier-2 memory as an MCP
// server over stdio. It is launched as a subprocess by Claude Code (via the
// project's .mcp.json) and, later, by the DevCore engine.
//
// Depends on: internal/episodic, internal/canonical, internal/embed, and
// internal/memoryserver.
// Depended on by: nothing in Go — this is a process entrypoint, launched over
// the MCP stdio transport.
// Why it exists: agents need a running memory service; main wires the two
// stores and the embedder together and serves them.
//
// Usage:
//
//	devcore-memory [flags]
//	devcore-memory --help
//
// Flags default to the standard in-repo paths and a local Ollama, so the
// server runs with no arguments from the DevCore repo root.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/djd39448/DevCore/internal/canonical"
	"github.com/djd39448/DevCore/internal/embed"
	"github.com/djd39448/DevCore/internal/episodic"
	"github.com/djd39448/DevCore/internal/memoryserver"
)

// dirPerm is the permission new state directories are created with.
const dirPerm = 0o750

func main() {
	episodicDB := flag.String("episodic-db", ".devcore/state/episodic.sqlite",
		"path to the episodic SQLite database")
	canonicalDir := flag.String("canonical-dir", ".devcore/memory",
		"path to the Tier-1 canonical memory directory")
	ollamaEndpoint := flag.String("ollama-endpoint", "http://localhost:11434",
		"base URL of the Ollama server used for embeddings")
	ollamaModel := flag.String("ollama-model", "nomic-embed-text",
		"Ollama model used to produce embeddings")
	flag.Parse()

	if err := run(*episodicDB, *canonicalDir, *ollamaEndpoint, *ollamaModel); err != nil {
		fmt.Fprintf(os.Stderr, "devcore-memory: %v\n", err)
		os.Exit(1)
	}
}

// run wires the memory subsystems together and serves them over stdio until an
// interrupt signal arrives or the MCP client disconnects.
func run(episodicDB, canonicalDir, ollamaEndpoint, ollamaModel string) error {
	if err := os.MkdirAll(filepath.Dir(episodicDB), dirPerm); err != nil {
		return fmt.Errorf("creating the episodic database directory: %w", err)
	}

	episodicStore, err := episodic.Open(episodicDB)
	if err != nil {
		return err
	}
	defer func() { _ = episodicStore.Close() }()

	canonicalStore, err := canonical.Open(canonicalDir)
	if err != nil {
		return err
	}

	embedder := embed.NewClient(ollamaEndpoint, ollamaModel)

	server, err := memoryserver.New(episodicStore, canonicalStore, embedder)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Run(ctx)
}
