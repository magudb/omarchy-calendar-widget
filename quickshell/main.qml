import QtQuick
import Quickshell

// Standalone test harness for the calendar bar widget - no omarchy-shell
// or waybar needed. Run from the repository root:
//
//   go build -o calendar-widget .
//   quickshell -p quickshell
//
// It renders Widget.qml with a mock "bar" object that stands in for the
// Omarchy bar host (the widget's documented contract): run() logs the
// click command, showTooltip() logs the hover tooltip. Point the
// `binary` setting at a built calendar-widget to see live data:
//
//   quickshell -p quickshell   # widget stays hidden without auth/data
//
// The widget polls `calendar-widget waybar`, so an authenticated setup
// (`calendar-widget setup`) produces the real colored label.

ShellRoot {
  visible: true
  width: 720
  height: 60
  color: "#1c1c1e"

  // Mock of the bar host's public interface (see the Omarchy bar README,
  // "Bar properties available to widgets").
  property var mockBar: ({
    foreground: "#e5e5ea",
    fontFamily: "monospace",
    barSize: 48,
    vertical: false,
    run: function(command) { console.log("[mock bar] run:", command) },
    showTooltip: function(target, text) {
      console.log("[mock bar] tooltip:\n" + String(text).replace(/\n/g, "\n    "))
    },
    hideTooltip: function(target) {}
  })

  Item {
    anchors.left: parent.left
    anchors.leftMargin: 16
    anchors.verticalCenter: parent.verticalCenter
    width: 560
    height: parent.height

    // The widget under test, same directory -> usable as a type.
    Widget {
      id: calendar
      anchors.left: parent.left
      anchors.verticalCenter: parent.verticalCenter
      width: implicitWidth
      height: implicitHeight
      bar: mockBar
      settings: ({
        interval: 30,
        // Default is the bare name "calendar-widget" resolved via PATH.
        // Use an absolute path here if it is not installed, e.g.
        // binary: "/absolute/path/to/calendar-widget"
      })
    }

    Text {
      anchors.right: parent.right
      anchors.verticalCenter: parent.verticalCenter
      text: "calendar-widget · quickshell test harness"
      color: "#6e6e73"
      font.pixelSize: 11
    }
  }
}
