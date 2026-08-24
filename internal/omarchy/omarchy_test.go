package omarchy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// minimalShell returns a minimal but structurally complete shell config.
func minimalShell() map[string]any {
	return map[string]any{
		"version": float64(1),
		"bar": map[string]any{
			"layout": map[string]any{
				"left":   []any{map[string]any{"id": "omarchy.menu"}},
				"center": []any{map[string]any{"id": "omarchy.clock"}},
				"right":  []any{map[string]any{"id": "omarchy.power"}},
			},
		},
		"plugins": []any{},
	}
}

func entryAt(t *testing.T, cfg map[string]any, section string, idx int) map[string]any {
	t.Helper()
	entries, _ := layoutMap(cfg)[section].([]any)
	if idx >= len(entries) {
		t.Fatalf("section %q has no entry %d", section, idx)
	}
	e, ok := entries[idx].(map[string]any)
	if !ok {
		t.Fatalf("section %q entry %d is not an object", section, idx)
	}
	return e
}

func countOurs(cfg map[string]any) int {
	n := 0
	layout := layoutMap(cfg)
	for _, section := range ValidSections {
		entries, _ := layout[section].([]any)
		for _, e := range entries {
			if isOurs(e) {
				n++
			}
		}
	}
	return n
}

func TestModuleEntryDefaults(t *testing.T) {
	entry := ModuleEntry(0, "")
	if entry["id"] != ModuleID || entry["type"] != "qml" {
		t.Fatalf("unexpected entry: %v", entry)
	}
	if entry["interval"] != defaultIntervalSeconds || entry["binary"] != defaultBinaryName {
		t.Fatalf("defaults not applied: %v", entry)
	}
	entry = ModuleEntry(30, "/opt/bin/calendar-widget")
	if entry["interval"] != 30 || entry["binary"] != "/opt/bin/calendar-widget" {
		t.Fatalf("overrides not applied: %v", entry)
	}
}

func TestSetModuleEntryAppends(t *testing.T) {
	cfg := minimalShell()
	idx, err := SetModuleEntry(cfg, "right", ModuleEntry(60, ""))
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 { // omarchy.power is 0
		t.Fatalf("idx = %d, want 1", idx)
	}
	if got := entryAt(t, cfg, "right", idx); got["id"] != ModuleID {
		t.Fatalf("entry = %v", got)
	}
	// Other sections untouched.
	if got := entryAt(t, cfg, "left", 0); got["id"] != "omarchy.menu" {
		t.Fatalf("left changed: %v", got)
	}
}

func TestSetModuleEntryReplacesSameSection(t *testing.T) {
	cfg := minimalShell()
	if _, err := SetModuleEntry(cfg, "right", ModuleEntry(60, "old")); err != nil {
		t.Fatal(err)
	}
	idx, err := SetModuleEntry(cfg, "right", ModuleEntry(30, "new"))
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 {
		t.Fatalf("idx = %d, want 1 (in-place replace)", idx)
	}
	if countOurs(cfg) != 1 {
		t.Fatalf("duplicate entries: %d", countOurs(cfg))
	}
	if got := entryAt(t, cfg, "right", idx); got["binary"] != "new" || got["interval"] != 30 {
		t.Fatalf("entry = %v", got)
	}
}

func TestSetModuleEntryMovesSectionOnReinstall(t *testing.T) {
	cfg := minimalShell()
	if _, err := SetModuleEntry(cfg, "left", ModuleEntry(60, "")); err != nil {
		t.Fatal(err)
	}
	idx, err := SetModuleEntry(cfg, "center", ModuleEntry(60, ""))
	if err != nil {
		t.Fatal(err)
	}
	if idx != 1 { // after omarchy.clock
		t.Fatalf("idx = %d, want 1", idx)
	}
	if countOurs(cfg) != 1 {
		t.Fatalf("expected exactly one entry after move, got %d", countOurs(cfg))
	}
	// No longer in left.
	for _, e := range layoutMap(cfg)["left"].([]any) {
		if isOurs(e) {
			t.Fatal("entry left in left section")
		}
	}
}

func TestSetModuleEntryInvalidSection(t *testing.T) {
	cfg := minimalShell()
	for _, section := range []string{"", "top", "Left", "right "} {
		if _, err := SetModuleEntry(cfg, section, ModuleEntry(60, "")); err == nil {
			t.Fatalf("section %q: expected error", section)
		}
	}
	if countOurs(cfg) != 0 {
		t.Fatal("entry was added for invalid section")
	}
}

func TestSetModuleEntryRefusesForeignSource(t *testing.T) {
	cfg := minimalShell()
	// User-owned "calendar" module: carries a source override.
	userEntry := map[string]any{"id": ModuleID, "type": "qml", "source": "/home/me/foo.qml"}
	layoutMap(cfg)["right"] = append(layoutMap(cfg)["right"].([]any), userEntry)
	if _, err := SetModuleEntry(cfg, "right", ModuleEntry(60, "")); err == nil {
		t.Fatal("expected error for user-owned source entry")
	}
	if countOurs(cfg) != 0 {
		t.Fatal("user entry was clobbered")
	}
	// Same check when the user entry is in another section: the id is
	// globally ours, so any foreign claim on it blocks the install.
	cfg = minimalShell()
	layoutMap(cfg)["center"] = append(layoutMap(cfg)["center"].([]any), userEntry)
	if _, err := SetModuleEntry(cfg, "right", ModuleEntry(60, "")); err == nil {
		t.Fatal("expected error for user-owned source entry in another section")
	}
}

func TestRemoveModuleEntry(t *testing.T) {
	cfg := minimalShell()
	layout := layoutMap(cfg)
	// Two of our entries in different sections (hand-seeded; the
	// installer itself can only ever leave one behind).
	layout["left"] = append(layout["left"].([]any), ModuleEntry(60, ""))
	layout["right"] = append(layout["right"].([]any), ModuleEntry(30, ""))
	// A user-owned "calendar" module (foreign source) in a third section.
	userEntry := map[string]any{"id": ModuleID, "type": "qml", "source": "/home/me/foo.qml"}
	layout["center"] = append(layout["center"].([]any), userEntry)

	removed := RemoveModuleEntry(cfg)
	if len(removed) != 2 {
		t.Fatalf("removed = %v, want 2 sections", removed)
	}
	if countOurs(cfg) != 0 {
		t.Fatal("our entries still present")
	}
	// User's foreign-source entry survives, in center.
	found := false
	for _, e := range layout["center"].([]any) {
		if m, _ := e.(map[string]any); m["source"] == "/home/me/foo.qml" {
			found = true
		}
	}
	if !found {
		t.Fatal("foreign-source calendar entry was removed")
	}
	// Stock entries survive.
	if got := entryAt(t, cfg, "left", 0); got["id"] != "omarchy.menu" {
		t.Fatalf("left changed: %v", got)
	}
	// Idempotent.
	if removed := RemoveModuleEntry(cfg); len(removed) != 0 {
		t.Fatalf("second remove = %v, want none", removed)
	}
}

func TestRemoveModuleEntryEmptiedSliceNotNil(t *testing.T) {
	cfg := minimalShell()
	// Our entry is the only one in left, so removing it must leave
	// [] (not null) so the saved JSON stays an array.
	layout := layoutMap(cfg)
	layout["left"] = []any{ModuleEntry(60, "")}
	RemoveModuleEntry(cfg)
	raw, err := json.Marshal(layoutMap(cfg)["left"])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("emptied section marshals as %s, want []", raw)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := ShellConfigPath(dir)

	cfg, existed, err := LoadShellConfig(path)
	if err != nil || existed {
		t.Fatalf("missing file: cfg=%v existed=%v err=%v", cfg, existed, err)
	}

	cfg = minimalShell()
	if err := SaveShellConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, existed, err := LoadShellConfig(path)
	if err != nil || !existed {
		t.Fatalf("load after save: existed=%v err=%v", existed, err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Fatalf("round trip mismatch:\n%v\n%v", got, cfg)
	}

	// Second save backs up the first.
	cfg["version"] = float64(999)
	if err := SaveShellConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + backupSuffix)
	if err != nil {
		t.Fatalf("no backup written: %v", err)
	}
	var backed map[string]any
	if err := json.Unmarshal(backup, &backed); err != nil {
		t.Fatalf("backup unparseable: %v", err)
	}
	if backed["version"] != float64(1) {
		t.Fatalf("backup does not hold previous content: %v", backed["version"])
	}
	if strings.Contains(string(backup), "999") {
		t.Fatal("backup overwritten with new content")
	}
}

func TestLoadShellConfigRejectsNonJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), shellConfigName)
	if err := os.WriteFile(path, []byte("{ version: 1, }"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadShellConfig(path); err == nil {
		t.Fatal("expected error for non-JSON config")
	}
}

func TestLoadShellConfigEmptyObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), shellConfigName)
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, existed, err := LoadShellConfig(path)
	if err != nil || !existed || cfg == nil {
		t.Fatalf("cfg=%v existed=%v err=%v", cfg, existed, err)
	}
	if _, err := SetModuleEntry(cfg, "right", ModuleEntry(60, "")); err != nil {
		t.Fatal(err)
	}
	if got := entryAt(t, cfg, "right", 0); got["id"] != ModuleID {
		t.Fatalf("entry = %v", got)
	}
}

func TestDefaultShellConfig(t *testing.T) {
	cfg, err := DefaultShellConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg["version"] != float64(1) {
		t.Fatalf("version = %v", cfg["version"])
	}
	layout := layoutMap(cfg)
	for _, section := range ValidSections {
		if layout[section] == nil {
			t.Fatalf("default has no %q section", section)
		}
	}
	if _, err := SetModuleEntry(cfg, "right", ModuleEntry(60, "")); err != nil {
		t.Fatal(err)
	}
	if idx := countOurs(cfg); idx != 1 {
		t.Fatalf("count = %d", idx)
	}
}

func TestInstallModule(t *testing.T) {
	dir := t.TempDir()
	path, err := InstallModule(dir, []byte("// qml"))
	if err != nil {
		t.Fatal(err)
	}
	want := ModulePath(dir)
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	if filepath.Base(filepath.Dir(path)) != "modules" {
		t.Fatalf("unexpected dir: %s", filepath.Dir(path))
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "// qml" {
		t.Fatalf("content = %q err=%v", data, err)
	}
}

func TestInstallPlugin(t *testing.T) {
	dir := t.TempDir()
	root, err := InstallPlugin(dir, []byte(`{"id":"x"}`), []byte("// qml"))
	if err != nil {
		t.Fatal(err)
	}
	if root != PluginDir(dir) {
		t.Fatalf("root = %q", root)
	}
	manifest, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil || string(manifest) != `{"id":"x"}` {
		t.Fatalf("manifest = %q err=%v", manifest, err)
	}
	widget, err := os.ReadFile(filepath.Join(root, "quickshell", "Widget.qml"))
	if err != nil || string(widget) != "// qml" {
		t.Fatalf("widget = %q err=%v", widget, err)
	}
}

func TestInstallPluginReinstallOverwrites(t *testing.T) {
	dir := t.TempDir()
	if _, err := InstallPlugin(dir, []byte(`{"v":1}`), []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallPlugin(dir, []byte(`{"v":2}`), []byte("v2")); err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
	manifest, _ := os.ReadFile(filepath.Join(PluginDir(dir), "manifest.json"))
	if string(manifest) != `{"v":2}` {
		t.Fatalf("manifest not overwritten: %q", manifest)
	}
}
