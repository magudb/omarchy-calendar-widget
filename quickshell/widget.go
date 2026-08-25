// Package quickshell embeds the Omarchy 4 (Quickshell) bar widget so the
// calendar-widget binary can install it into the user's shell config
// without requiring a checkout of this repository.
package quickshell

import _ "embed"

// WidgetQML is the bar widget component (canonical copy: quickshell/Widget.qml).
//
//go:embed Widget.qml
var WidgetQML []byte

// ManifestJSON is the plugin manifest used for the `omarchy plugin`
// install path (canonical copy: quickshell/manifest.json). It is mirrored
// to the repository root so `omarchy plugin add <repo-url>` works against
// a plain git clone; the root copy is checked in CI for drift.
//
//go:embed manifest.json
var ManifestJSON []byte
