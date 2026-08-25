package quickshell

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWidgetQMLEmbedded(t *testing.T) {
	if len(WidgetQML) == 0 {
		t.Fatal("WidgetQML is empty")
	}
	s := string(WidgetQML)
	// The bar re-checks the tooltip on a timer against these exact
	// property names; a rename would silently break hover tooltips.
	for _, needle := range []string{"tooltipHovered", "Process", "StdioCollector", "implicitWidth"} {
		if !strings.Contains(s, needle) {
			t.Fatalf("Widget.qml missing %q", needle)
		}
	}
}

func TestManifestJSON(t *testing.T) {
	var manifest struct {
		SchemaVersion int `json:"schemaVersion"`
		ID            string
		Kind          []string `json:"kinds"`
		EntryPoints   struct {
			BarWidget string `json:"barWidget"`
		} `json:"entryPoints"`
		BarWidget struct {
			DefaultSection string `json:"defaultSection"`
			Defaults       map[string]any
			Schema         []map[string]any
		} `json:"barWidget"`
	}
	if err := json.Unmarshal(ManifestJSON, &manifest); err != nil {
		t.Fatalf("manifest.json does not parse: %v", err)
	}
	if manifest.ID != "magudb.calendar" {
		t.Fatalf("id = %q", manifest.ID)
	}
	if manifest.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d", manifest.SchemaVersion)
	}
	found := false
	for _, k := range manifest.Kind {
		if k == "bar-widget" {
			found = true
		}
	}
	if !found {
		t.Fatalf("kinds = %v", manifest.Kind)
	}
	if manifest.EntryPoints.BarWidget != "quickshell/Widget.qml" {
		t.Fatalf("entryPoints.barWidget = %q", manifest.EntryPoints.BarWidget)
	}
	if manifest.BarWidget.DefaultSection != "right" {
		t.Fatalf("defaultSection = %q", manifest.BarWidget.DefaultSection)
	}
	for _, key := range []string{"interval", "binary", "onClick"} {
		have := false
		for _, field := range manifest.BarWidget.Schema {
			if field["key"] == key {
				have = true
			}
		}
		if !have {
			t.Fatalf("schema missing key %q", key)
		}
	}
}
