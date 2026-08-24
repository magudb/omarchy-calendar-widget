package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"calendar-widget/internal/omarchy"
	"calendar-widget/quickshell"

	"github.com/spf13/cobra"
)

var (
	omarchySection  string
	omarchyInterval int
	omarchyBinary   string
	omarchyAsPlugin bool
)

var omarchyCmd = &cobra.Command{
	Use:   "omarchy [install|uninstall]",
	Short: "Install or remove the widget for the Omarchy 4 (Quickshell) bar",
	Long: `Install or remove the calendar widget for Omarchy 4, whose status bar
is a Quickshell panel (omarchy-shell) instead of waybar.

Two install paths:

  (default) QML bar module - copies the widget QML to
          ~/.config/omarchy/bar/modules/calendar.qml and adds a
          {"id":"calendar","type":"qml"} entry to your shell.json bar
          layout (with a .bak backup of the previous file).

  --plugin  bar-widget plugin - writes the plugin (manifest.json +
          QML) to ~/.config/omarchy/plugins/magudb.calendar and tries
          to rescan/enable it via omarchy-shell and the omarchy CLI.

In both paths the widget polls '<binary> waybar' (this CLI's waybar
mode) on an interval and renders the result in the bar.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		action := "install"
		if len(args) > 0 {
			switch args[0] {
			case "install", "uninstall", "remove":
				action = args[0]
			default:
				fmt.Printf("Unknown action %q (expected install or uninstall)\n", args[0])
				os.Exit(1)
				return
			}
		}
		var err error
		if action == "install" {
			err = runOmarchyInstall()
		} else {
			err = runOmarchyUninstall()
		}
		if err != nil {
			fmt.Printf("Omarchy %s failed: %v\n", action, err)
			os.Exit(1)
		}
	},
}

// resolveBinary finds the executable the widget should poll. If the
// requested binary is not in PATH, it falls back to this process's own
// executable so the bar keeps working when the CLI was not installed
// on the user's PATH yet.
func resolveBinary(flag string) (path string, note string, err error) {
	if p, err := exec.LookPath(flag); err == nil {
		return p, "", nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("%q not found in PATH and the current executable cannot be determined: %w", flag, err)
	}
	return exe, fmt.Sprintf(
		"note: %q was not found in PATH; the widget will poll %q (this binary) directly\n", flag, exe), nil
}

// runBestEffort runs name args... and returns a descriptive error; the
// caller decides whether it is fatal.
func runBestEffort(timeout time.Duration, name string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s is not in PATH", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if err != nil {
		if s := strings.TrimSpace(string(out)); s != "" {
			return fmt.Errorf("%s %s: %v (output: %s)", name, strings.Join(args, " "), err, s)
		}
		return fmt.Errorf("%s %s: %v", name, strings.Join(args, " "), err)
	}
	return nil
}

func runOmarchyInstall() error {
	dir, err := omarchy.ConfigDir()
	if err != nil {
		return err
	}
	binary, note, err := resolveBinary(omarchyBinary)
	if err != nil {
		return err
	}
	if note != "" {
		fmt.Print(note)
	}

	if omarchyAsPlugin {
		root, err := omarchy.InstallPlugin(dir, quickshell.ManifestJSON, quickshell.WidgetQML)
		if err != nil {
			return err
		}
		fmt.Printf("Installed plugin %s at %s\n", omarchy.PluginID, root)
		if err := runBestEffort(15*time.Second, "omarchy-shell", "shell", "rescanPlugins"); err != nil {
			fmt.Printf("warning: %v\n", err)
			fmt.Printf("  re-run 'omarchy-shell shell rescanPlugins' once omarchy-shell is available\n")
		}
		if err := runBestEffort(30*time.Second, "omarchy", "plugin", "enable", omarchy.PluginID); err != nil {
			fmt.Printf("warning: %v\n", err)
			fmt.Printf("  enable the plugin with 'omarchy plugin enable %s'\n", omarchy.PluginID)
		}
		fmt.Println("Done. The widget now appears in the Omarchy bar via the plugin system.")
		return nil
	}

	modulePath, err := omarchy.InstallModule(dir, quickshell.WidgetQML)
	if err != nil {
		return err
	}

	shellPath := omarchy.ShellConfigPath(dir)
	cfg, existed, err := omarchy.LoadShellConfig(shellPath)
	if err != nil {
		return err
	}
	seeded := false
	if !existed {
		cfg, err = omarchy.DefaultShellConfig()
		if err != nil {
			return err
		}
		seeded = true
		fmt.Printf("Created %s from stock Omarchy defaults\n", shellPath)
	}

	entry := omarchy.ModuleEntry(omarchyInterval, binary)
	idx, err := omarchy.SetModuleEntry(cfg, omarchySection, entry)
	if err != nil {
		return err
	}
	if err := omarchy.SaveShellConfig(shellPath, cfg); err != nil {
		return err
	}
	if seeded {
		// The backup is a copy of our own seed, not user data.
		if err := os.Remove(shellPath + ".bak"); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	fmt.Printf("Installed %s\n", modulePath)
	if seeded {
		fmt.Printf("Updated %s:\n", shellPath)
	} else {
		fmt.Printf("Updated %s (previous file backed up to %s.bak):\n", shellPath, shellPath)
	}
	fmt.Printf("  bar.layout.%s[%d] = { id: %q, type: qml, interval: %d, binary: %q }\n",
		omarchySection, idx, omarchy.ModuleID, entry["interval"], binary)
	fmt.Println("The bar watches shell.json and should pick the widget up automatically.")
	fmt.Println("If it does not, run: omarchy-shell shell reloadConfig")
	fmt.Println("First click triggers an interactive browser sign-in if no Microsoft account token exists yet.")
	return nil
}

func runOmarchyUninstall() error {
	dir, err := omarchy.ConfigDir()
	if err != nil {
		return err
	}

	if omarchyAsPlugin {
		root := omarchy.PluginDir(dir)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			fmt.Printf("No plugin directory at %s; nothing to remove\n", root)
		} else if err := os.RemoveAll(root); err != nil {
			return err
		} else {
			fmt.Printf("Removed %s\n", root)
		}
		fmt.Printf("If the plugin was installed from git, remove it with 'omarchy plugin remove %s'\n", omarchy.PluginID)
		return nil
	}

	shellPath := omarchy.ShellConfigPath(dir)
	cfg, existed, err := omarchy.LoadShellConfig(shellPath)
	if err != nil {
		return err
	}
	var removed []string
	if existed {
		removed = omarchy.RemoveModuleEntry(cfg)
		if len(removed) > 0 {
			if err := omarchy.SaveShellConfig(shellPath, cfg); err != nil {
				return err
			}
			fmt.Printf("Updated %s (previous file backed up to %s.bak): removed calendar entry from %s\n",
				shellPath, shellPath, strings.Join(removed, ", "))
		} else {
			fmt.Printf("No calendar entry in the %s bar layout\n", shellPath)
		}
	} else {
		fmt.Printf("No %s found; skipping bar layout update\n", shellPath)
	}

	modulePath := omarchy.ModulePath(dir)
	if err := os.Remove(modulePath); err == nil {
		fmt.Printf("Removed %s\n", modulePath)
	} else if !os.IsNotExist(err) {
		return err
	} else {
		fmt.Printf("No module file at %s; nothing to remove\n", modulePath)
	}
	fmt.Println("Done.")
	return nil
}

func init() {
	omarchyCmd.Flags().StringVar(&omarchySection, "section", "right", "bar layout section (left, center, right)")
	omarchyCmd.Flags().IntVar(&omarchyInterval, "interval", 60, "poll interval in seconds")
	omarchyCmd.Flags().StringVar(&omarchyBinary, "binary", "calendar-widget", "executable the widget polls (falls back to this binary if not in PATH)")
	omarchyCmd.Flags().BoolVar(&omarchyAsPlugin, "plugin", false, "install as a bar-widget plugin instead of a QML bar module")
	rootCmd.AddCommand(omarchyCmd)
}
