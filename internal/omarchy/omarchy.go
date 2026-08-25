// Package omarchy implements the Omarchy 4 (Quickshell) integration.
//
// Omarchy 4 drops waybar: the status bar is a Quickshell panel
// (omarchy-shell) that loads third-party widgets as either
//
//   - a custom QML bar module: a `~/.config/omarchy/shell.json` bar layout
//     entry { "id": "<name>", "type": "qml" } makes the bar load
//     ~/.config/omarchy/bar/modules/<name>.qml; or
//   - a bar-widget plugin: a directory with a manifest.json (schema
//     documented in the Omarchy shell README) under
//     ~/.config/omarchy/plugins/<plugin-id>/, enabled with
//     `omarchy plugin enable <id>`.
//
// This package owns the file layout for both paths and the shell.json
// bar layout surgery (insert/replace/remove of the module entry with a
// .bak backup). The widget QML and manifest bytes themselves live in the
// calendar-widget/quickshell package (embedded).
package omarchy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// PluginID is the id of the bar-widget plugin (manifest "id").
	PluginID = "magudb.calendar"

	// ModuleID is the shell.json bar layout entry id for the QML module
	// install path; the bar loads bar/modules/<ModuleID>.qml.
	ModuleID = "calendar"

	// ModuleFileName is the QML file name for the QML module install.
	ModuleFileName = ModuleID + ".qml"

	// defaultBinaryName is the executable the widget polls when the
	// caller did not override it.
	defaultBinaryName = "calendar-widget"

	// defaultIntervalSeconds is the poll interval used when the caller
	// did not override it (matches the old waybar config).
	defaultIntervalSeconds = 60

	shellConfigName = "shell.json"
	backupSuffix    = ".bak"
)

// Section names the bar layout supports.
var ValidSections = []string{"left", "center", "right"}

// IsValidSection reports whether section is a valid bar layout section.
func IsValidSection(section string) bool {
	for _, s := range ValidSections {
		if s == section {
			return true
		}
	}
	return false
}

// ConfigDir returns the user's Omarchy config directory
// (~/.config/omarchy). It is created by the installer as needed.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "omarchy"), nil
}

// ShellConfigPath returns the user's shell.json path inside dir.
func ShellConfigPath(dir string) string {
	return filepath.Join(dir, shellConfigName)
}

// ModulePath returns the QML module file path inside dir.
func ModulePath(dir string) string {
	return filepath.Join(dir, "bar", "modules", ModuleFileName)
}

// PluginDir returns the plugin checkout root inside dir
// (~/.config/omarchy/plugins/<PluginID>).
func PluginDir(dir string) string {
	return filepath.Join(dir, "plugins", PluginID)
}

// DefaultShellConfig returns a parseable copy of the stock Omarchy 4
// shell config (from the Omarchy repo's config/omarchy/shell.json on the
// quattro branch), used to seed the user file when none exists yet. A
// user file is canonical once it exists (the shell does not deep-merge),
// so seeding the full stock shape - not just the bar subtree - keeps a
// fresh install behaving exactly like stock Omarchy plus the widget.
func DefaultShellConfig() (map[string]any, error) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(DefaultShellJSON), &cfg); err != nil {
		return nil, fmt.Errorf("builtin default shell.json is invalid: %w", err)
	}
	return cfg, nil
}

// DefaultShellJSON is the stock Omarchy 4 shell config (see above).
const DefaultShellJSON = `{
  "version": 1,
  "idle": {
    "screensaver": 150,
    "lock": 300
  },
  "bar": {
    "position": "top",
    "transparent": false,
    "centerAnchor": "omarchy.clock",
    "layout": {
      "left": [
        { "id": "omarchy.menu" },
        { "id": "omarchy.workspaces" }
      ],
      "center": [
        { "id": "omarchy.indicators" },
        { "id": "omarchy.clock", "format": "dddd HH:mm", "formatAlt": "d MMMM 'W'ww yyyy", "verticalFormat": "HH\n\u2014\nmm" },
        { "id": "omarchy.keyboard-layout" },
        { "id": "omarchy.weather" },
        { "id": "omarchy.system-update" }
      ],
      "right": [
        { "id": "omarchy.tray" },
        { "id": "omarchy.agents" },
        { "id": "omarchy.bluetooth" },
        { "id": "omarchy.network" },
        { "id": "omarchy.audio" },
        { "id": "omarchy.monitor" },
        { "id": "omarchy.power" }
      ]
    }
  },
  "plugins": []
}`

// LoadShellConfig reads and parses shell.json. existed is false when the
// file is missing. The file must be plain JSON (no comments or trailing
// commas) - the same constraint the shell itself has.
func LoadShellConfig(path string) (cfg map[string]any, existed bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, true, fmt.Errorf("%s must be plain JSON (no comments or trailing commas): %w", path, err)
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	return cfg, true, nil
}

// SaveShellConfig writes cfg to path with 2-space indentation, backing
// up the previous file to <path>.bak first (when one exists) and writing
// through a same-directory temp file so a torn write can never corrupt
// the user's config.
func SaveShellConfig(path string, cfg map[string]any) error {
	old, err := os.ReadFile(path)
	if err == nil {
		if err := os.WriteFile(path+backupSuffix, old, 0644); err != nil {
			return fmt.Errorf("backing up %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding shell.json: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".shell.json.*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("setting mode on %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// ModuleEntry builds the shell.json bar layout entry for the widget.
// interval <= 0 and empty binary fall back to the waybar-era defaults.
func ModuleEntry(interval int, binary string) map[string]any {
	if interval <= 0 {
		interval = defaultIntervalSeconds
	}
	if binary == "" {
		binary = defaultBinaryName
	}
	return map[string]any{
		"id":       ModuleID,
		"type":     "qml",
		"interval": interval,
		"binary":   binary,
	}
}

// layoutMap returns (creating if needed) the bar.layout map.
func layoutMap(cfg map[string]any) map[string]any {
	bar, _ := cfg["bar"].(map[string]any)
	if bar == nil {
		bar = map[string]any{}
		cfg["bar"] = bar
	}
	layout, _ := bar["layout"].(map[string]any)
	if layout == nil {
		layout = map[string]any{}
		bar["layout"] = layout
	}
	return layout
}

// isOurs reports whether an entry with the module id is one we wrote
// (no custom "source" override, which would mean the user points the id
// at their own QML file).
func isOurs(entry any) bool {
	m, ok := entry.(map[string]any)
	if !ok || m["id"] != ModuleID {
		return false
	}
	return m["source"] == nil
}

// SetModuleEntry adds or replaces the widget entry in the given bar
// layout section and returns the entry's final index. An existing
// id "calendar" entry that we own (no "source" override) is replaced
// in place when it is already in the target section, or moved out of
// its current section when the install target differs - installing is
// always idempotent and leaves exactly one entry. An existing
// "calendar" entry with a "source" override (a user-owned module with
// this id, anywhere in the layout) is refused with an error so we
// never claim an id the user has taken.
func SetModuleEntry(cfg map[string]any, section string, entry map[string]any) (int, error) {
	if !IsValidSection(section) {
		return -1, fmt.Errorf("section must be one of %v (got %q)", ValidSections, section)
	}
	layout := layoutMap(cfg)

	oursIdx := map[string]int{}
	for _, s := range ValidSections {
		entries, _ := layout[s].([]any)
		for i, e := range entries {
			m, ok := e.(map[string]any)
			if !ok || m["id"] != ModuleID {
				continue
			}
			if m["source"] != nil {
				return -1, fmt.Errorf(
					"shell.json already has a %q module with a custom source in the %q section; "+
						"remove or rename it and try again", ModuleID, s)
			}
			oursIdx[s] = i
		}
	}

	if i, ok := oursIdx[section]; ok {
		entries := layout[section].([]any)
		entries[i] = entry
		return i, nil
	}

	for s, i := range oursIdx {
		entries := layout[s].([]any)
		layout[s] = append(entries[:i], entries[i+1:]...)
	}
	entries, _ := layout[section].([]any)
	layout[section] = append(entries, entry)
	return len(layout[section].([]any)) - 1, nil
}

// RemoveModuleEntry removes our widget entry from every bar layout
// section and returns the sections it was removed from. User-owned
// "calendar" entries (with a "source" override) are left in place.
func RemoveModuleEntry(cfg map[string]any) []string {
	var removed []string
	layout := layoutMap(cfg)
	for _, section := range ValidSections {
		entries, _ := layout[section].([]any)
		if len(entries) == 0 {
			continue
		}
		next := make([]any, 0, len(entries))
		dropped := false
		for _, e := range entries {
			if isOurs(e) {
				dropped = true
				continue
			}
			next = append(next, e)
		}
		if dropped {
			layout[section] = next
			removed = append(removed, section)
		}
	}
	return removed
}

// InstallModule writes widgetQML to dir/bar/modules/calendar.qml and
// returns the file path.
func InstallModule(dir string, widgetQML []byte) (string, error) {
	path := ModulePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, widgetQML, 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", path, err)
	}
	return path, nil
}

// InstallPlugin writes the plugin (manifest.json at the plugin root plus
// quickshell/Widget.qml, mirroring this repository's layout) into
// dir/plugins/<PluginID> and returns the plugin root.
func InstallPlugin(dir string, manifest, widgetQML []byte) (string, error) {
	root := PluginDir(dir)
	quickshellDir := filepath.Join(root, "quickshell")
	for _, d := range []string{root, quickshellDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", fmt.Errorf("creating %s: %w", d, err)
		}
	}
	manifestPath := filepath.Join(root, "manifest.json")
	if err := os.WriteFile(manifestPath, manifest, 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", manifestPath, err)
	}
	widgetPath := filepath.Join(quickshellDir, "Widget.qml")
	if err := os.WriteFile(widgetPath, widgetQML, 0644); err != nil {
		return "", fmt.Errorf("writing %s: %w", widgetPath, err)
	}
	return root, nil
}
