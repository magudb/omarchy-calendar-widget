package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configFile string
	debug      bool
)

var rootCmd = &cobra.Command{
	Use:   "calendar-widget",
	Short: "A calendar widget for waybar and the Omarchy 4 (Quickshell) bar",
	Long: `A calendar widget that shows your next Microsoft 365 meeting with
visual indicators for urgency and click-to-join functionality for
Teams meetings.

Supported bars:
  - waybar: use the 'waybar' mode (JSON line output)
  - Omarchy 4 (Quickshell): run 'omarchy install' to add the widget to
    the omarchy-shell bar
`,
	Run: func(cmd *cobra.Command, args []string) {
		// Run the widget by default
		widgetCmd.Run(cmd, args)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file (default is $HOME/.config/calendar-widget/config.json)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug mode")

	rootCmd.AddCommand(widgetCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(tooltipCmd)
}
