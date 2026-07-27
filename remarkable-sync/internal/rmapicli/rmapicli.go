// Package rmapicli wraps the rmapi CLI binary (github.com/ddvk/rmapi), which
// this add-on already builds into the image. Rather than maintaining our own
// client against reMarkable Cloud's private API — which has repeatedly
// broken as reMarkable moved hosts/protocols — we shell out to rmapi for
// authentication and document sync, the same way this add-on wraps
// obsidian-headless and obsidian-web-mcp instead of reimplementing them.
package rmapicli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const binary = "rmapi"

// configPath returns the path rmapi reads/writes its device+user tokens to.
// Mirrors rmapi's own resolution order (RMAPI_CONFIG env var, then ~/.rmapi)
// closely enough for an existence check — this add-on always sets
// RMAPI_CONFIG explicitly (build-env.sh), so the fallback rarely matters.
func configPath() string {
	if p := os.Getenv("RMAPI_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".rmapi")
}

// Register pipes a one-time pairing code to rmapi, completing device
// registration. rmapi reads the code as a single newline-terminated line on
// stdin — no TTY is required. On success rmapi persists its own device/user
// tokens to its config file (RMAPI_CONFIG env var, set in build-env.sh).
func Register(ctx context.Context, code string) error {
	cmd := exec.CommandContext(ctx, binary, "ls")
	cmd.Stdin = strings.NewReader(code + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rmapi registration: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// IsAuthenticated reports whether rmapi already has a usable device token.
//
// Checks for the config file first rather than always shelling out: with no
// device token, `rmapi ls` falls into rmapi's interactive one-time-code
// prompt, reading from stdin — which we deliberately don't supply here (it
// defaults to /dev/null) — and depending on how rmapi handles that EOF, the
// process can retry for the full context timeout before giving up. Calling
// this from an HTTP handler (as the registration UI does, on every page
// load) made the whole page hang for up to 30s while unregistered, which is
// indistinguishable from the server not listening at all.
func IsAuthenticated(ctx context.Context) bool {
	if _, err := os.Stat(configPath()); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "ls")
	return cmd.Run() == nil
}

// SyncTree recursively mirrors remotePath into outputDir as .rmdoc bundles,
// preserving reMarkable's folder hierarchy as real directories. Incremental:
// only files whose remote copy is newer than the local one are re-fetched.
func SyncTree(ctx context.Context, remotePath, outputDir string) error {
	cmd := exec.CommandContext(ctx, binary, "mget", "-i", "-o", outputDir, remotePath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rmapi mget: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
