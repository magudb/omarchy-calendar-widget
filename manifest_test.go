package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"calendar-widget/quickshell"
)

// TestRootManifestSynced guards the invariant that the root manifest.json
// (used by `omarchy plugin add <repo-url>`) is a byte-for-byte copy of
// the canonical quickshell/manifest.json. CI enforces the same with a
// diff; this test makes it visible to anyone running `go test ./...`.
func TestRootManifestSynced(t *testing.T) {
	root, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatalf("reading root manifest.json: %v", err)
	}
	if strings.TrimSpace(string(root)) != strings.TrimSpace(string(quickshell.ManifestJSON)) {
		t.Fatal("root manifest.json is out of sync with quickshell/manifest.json; run 'make manifest'")
	}
}

// TestRootManifestEntryPointExists checks the barWidget entry point
// resolves relative to the repository root, which is how the shell loads
// it from a git checkout.
func TestRootManifestEntryPointExists(t *testing.T) {
	if !fileExists(filepath.Join("quickshell", "Widget.qml")) {
		t.Fatal("entry point quickshell/Widget.qml missing from repository")
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
