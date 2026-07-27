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
	"os/exec"
	"strings"
	"time"
)

const binary = "rmapi"

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

// IsAuthenticated reports whether rmapi already has a usable device token by
// running a lightweight, read-only command and checking its exit status.
func IsAuthenticated(ctx context.Context) bool {
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
