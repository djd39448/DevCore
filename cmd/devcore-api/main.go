// Command devcore-api serves DevCore's Tier-1 and Tier-2 memory as a small
// read-only HTTP API for the desktop shell.
//
// Depends on: internal/episodic, internal/canonical, and internal/apiserver.
// Depended on by: nothing in Go — this is a process entrypoint, launched as
// a subprocess by the DevCore desktop app (desktop/Shell/main.swift).
// Why it exists: the desktop shell is a WKWebView whose page origin is
// devcore://. To render real data it needs a real HTTP origin to fetch from;
// devcore-api is that origin, bound to a localhost-only TCP port.
//
// Usage:
//
//	devcore-api [flags]
//	devcore-api --help
//
// The default flags resolve to the standard in-repo paths and bind 127.0.0.1
// on a kernel-chosen port (port 0). The chosen port is printed to stdout as
// `LISTENING:<port>\n` so the parent process can read it without parsing logs.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/djd39448/DevCore/internal/apiserver"
	"github.com/djd39448/DevCore/internal/canonical"
	"github.com/djd39448/DevCore/internal/episodic"
)

// dirPerm is the permission new state directories are created with.
const dirPerm = 0o750

func main() {
	episodicDB := flag.String("episodic-db", ".devcore/state/episodic.sqlite",
		"path to the episodic SQLite database")
	canonicalDir := flag.String("canonical-dir", ".devcore/memory",
		"path to the Tier-1 canonical memory directory")
	addr := flag.String("addr", "127.0.0.1:0",
		"bind address; the default uses a kernel-chosen ephemeral port")
	flag.Parse()

	if err := run(*episodicDB, *canonicalDir, *addr); err != nil {
		fmt.Fprintf(os.Stderr, "devcore-api: %v\n", err)
		os.Exit(1)
	}
}

// run wires the stores together, opens the listener, prints the chosen port,
// and serves until an interrupt arrives.
func run(episodicDB, canonicalDir, addr string) error {
	if err := os.MkdirAll(filepath.Dir(episodicDB), dirPerm); err != nil {
		return fmt.Errorf("creating the episodic database directory: %w", err)
	}

	episodicStore, err := episodic.Open(episodicDB)
	if err != nil {
		return fmt.Errorf("opening the episodic store: %w", err)
	}
	defer func() { _ = episodicStore.Close() }()

	canonicalStore, err := canonical.Open(canonicalDir)
	if err != nil {
		return fmt.Errorf("opening the canonical store: %w", err)
	}

	server, err := apiserver.New(episodicStore, canonicalStore)
	if err != nil {
		return fmt.Errorf("wiring the api server: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("binding %s: %w", addr, err)
	}
	// Tell the parent process where to talk to us. A dedicated line — not
	// mixed into log noise — keeps the contract trivial to parse.
	port := ln.Addr().(*net.TCPAddr).Port
	if _, err := fmt.Fprintf(os.Stdout, "LISTENING:%d\n", port); err != nil {
		return fmt.Errorf("announcing listen port: %w", err)
	}

	return server.Serve(ctx, ln)
}
