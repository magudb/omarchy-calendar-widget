import QtQuick
import Quickshell.Io

// Bar widget for calendar-widget (Microsoft 365 calendar), for the
// Omarchy 4 Quickshell bar (omarchy-shell).
//
// This file is the single canonical widget. It ships two ways:
//
//  1. Custom QML bar module (the default install path,
//     `calendar-widget omarchy`): a `~/.config/omarchy/shell.json` bar
//     layout entry
//
//         { "id": "calendar", "type": "qml", "interval": 60 }
//
//     makes the bar load this file from
//     ~/.config/omarchy/bar/modules/calendar.qml.
//
//  2. bar-widget plugin entry point (see manifest.json next to this
//     file): `omarchy plugin add <repo-url>` clones a checkout of this
//     repo and loads quickshell/Widget.qml from there.
//
// The widget polls `calendar-widget waybar` (one JSON line per run with
// text / tooltip / class / alt - the same output the old waybar module
// consumed) and mirrors the waybar color scheme:
//
//   current   green  meeting in progress
//   urgent    red    starts within 5 minutes
//   soon      yellow starts within 15 minutes
//   upcoming  blue   starts later
//   past      gray   ended
//   error     red    auth/calendar failure (click to sign in / retry)
//
// Left click runs `calendar-widget click` (smart click: opens the
// current/urgent meeting link, or kicks off a re-auth). Right click
// forces an immediate refresh. Hover shows today's full schedule.

Item {
  id: root

  // ---------------------------------------------------------------------
  // Properties the bar host injects into every module (see the Omarchy
  // bar README, "Bar properties available to widgets"). Declared so the
  // host's injectProps() can set them; all have safe standalone
  // fallbacks for out-of-bar testing (see main.qml).
  // ---------------------------------------------------------------------
  property var bar: null
  property string moduleName: "calendar"
  property var settings: ({})

  // Read a value from this widget's inline shell.json layout entry.
  function setting(name, fallback) {
    var value = settings ? settings[name] : undefined
    return value === undefined || value === null ? fallback : value
  }

  readonly property int intervalSeconds: Math.max(1, Number(setting("interval", 60)))
  readonly property string binary: String(setting("binary", "calendar-widget"))
  readonly property string clickCommand: String(setting("onClick", "calendar-widget click"))
  readonly property bool vertical: bar ? bar.vertical : false

  // ---------------------------------------------------------------------
  // State from the last `calendar-widget waybar` run.
  // ---------------------------------------------------------------------
  property string outputText: ""
  property string outputTooltip: ""
  property string statusClass: ""

  // The bar re-checks target.tooltipHovered on its 400 ms timer before
  // actually showing the tooltip; first-party buttons expose the same
  // property, so mirror that contract.
  readonly property bool tooltipHovered: visible && mouseArea.containsMouse

  readonly property color statusColor: {
    switch (statusClass) {
      case "current":  return "#3ddc84"
      case "urgent":   return "#ff453a"
      case "soon":     return "#ffcc00"
      case "upcoming": return "#4488ff"
      case "past":     return "#8e8e93"
      case "error":    return "#ff453a"
      default:
        return bar ? bar.foreground : "#ffffff"
    }
  }

  // Compact glyph for vertical bars (text widgets fall back to
  // icon-only forms there, like the first-party widgets do).
  readonly property string verticalGlyph: {
    switch (statusClass) {
      case "current":  return "🟢"
      case "urgent":   return "🔴"
      case "soon":     return "🟡"
      case "upcoming": return "🔵"
      case "past":     return "⚫"
      case "error":    return "⚠️"
      default:         return "📅"
    }
  }

  // ---------------------------------------------------------------------
  // Polling: one process run per interval, the same pattern as the
  // bar's built-in command modules. The process is skipped while a
  // previous run is still in flight (e.g. slow first-time auth).
  // ---------------------------------------------------------------------
  Process {
    id: proc
    command: [root.binary, "waybar"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.update(text)
    }
  }

  function runProcess() {
    if (!proc.running)
      proc.running = true
  }

  Timer {
    interval: root.intervalSeconds * 1000
    running: root.binary !== ""
    repeat: true
    triggeredOnStart: true
    onTriggered: root.runProcess()
  }

  // Parse one run's output. Mirrors the bar's own module parser: the
  // last non-empty line must be JSON with text / tooltip / class;
  // anything else is rendered as plain text without status styling.
  function update(raw) {
    var lines = String(raw || "").split("\n")
    var last = ""
    for (var i = lines.length - 1; i >= 0; i--) {
      if (lines[i].trim() !== "") {
        last = lines[i]
        break
      }
    }
    if (last === "")
      return

    try {
      var data = JSON.parse(last)
      outputText = data.text !== undefined && data.text !== null ? String(data.text) : last
      outputTooltip = data.tooltip !== undefined && data.tooltip !== null ? String(data.tooltip) : ""
      statusClass = String(data.class !== undefined && data.class !== null ? data.class : (data.alt || ""))
    } catch (e) {
      outputText = last
      outputTooltip = ""
      statusClass = ""
    }
  }

  // ---------------------------------------------------------------------
  // Geometry. The bar slot sizes itself from implicitWidth/implicitHeight
  // and collapses to zero width while the item is invisible, so the bar
  // shows no dead gap until the first data arrives.
  // ---------------------------------------------------------------------
  readonly property bool hasContent: outputText !== ""
  visible: hasContent

  implicitWidth: vertical
    ? (bar ? bar.barSize : 28)
    : Math.max(12, label.implicitWidth + 16)
  implicitHeight: vertical
    ? Math.max(12, label.implicitHeight + 12)
    : (bar ? bar.barSize : 26)

  Text {
    id: label
    anchors.centerIn: parent
    visible: root.visible
    text: root.vertical ? root.verticalGlyph : root.outputText
    color: root.statusColor
    opacity: 1
    font.family: bar ? bar.fontFamily : "monospace"
    font.pixelSize: 12
    horizontalAlignment: Text.AlignHCenter
    verticalAlignment: Text.AlignVCenter
  }

  // Parity with the waybar CSS pulse animation for urgent/current.
  NumberAnimation {
    target: label
    property: "opacity"
    from: 1.0
    to: 0.65
    duration: 500
    loops: Animation.Infinite
    running: root.visible && (root.statusClass === "urgent" || root.statusClass === "current")
  }

  onStatusClassChanged: label.opacity = 1

  MouseArea {
    id: mouseArea
    anchors.fill: parent
    hoverEnabled: true
    acceptedButtons: Qt.LeftButton | Qt.RightButton
    cursorShape: root.visible ? Qt.PointingHandCursor : Qt.ArrowCursor

    onEntered: if (root.bar && root.outputTooltip !== "")
      root.bar.showTooltip(root, root.outputTooltip)
    onExited: if (root.bar)
      root.bar.hideTooltip(root)
    onClicked: function(mouse) {
      if (!root.bar)
        return
      if (mouse.button === Qt.LeftButton)
        root.bar.run(root.clickCommand)
      else if (mouse.button === Qt.RightButton)
        root.runProcess()
    }
  }
}
