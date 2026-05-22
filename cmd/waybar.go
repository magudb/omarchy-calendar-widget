package cmd

import (
	"calendar-widget/internal/widget"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var forceRefresh bool

var waybarCmd = &cobra.Command{
	Use:   "waybar",
	Short: "Run in waybar mode with JSON output",
	Long:  `Run the calendar widget in waybar mode, outputting JSON format suitable for waybar modules.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runWaybar(); err != nil {
			fmt.Printf("Waybar mode failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func runWaybar() error {
	// Waybar polls on its own configured interval and we exit after one print,
	// so RefreshInterval is intentionally unused here.
	w, err := widget.NewWidgetWithOptions(&widget.Config{
		Compact: true,
		Debug:   debug,
	}, forceRefresh) // forceRefresh enables interactive browser auth for this run
	if err != nil {
		return fmt.Errorf("failed to create widget: %w", err)
	}

	return w.RunWaybarWithRefresh(forceRefresh)
}

func init() {
	waybarCmd.Flags().BoolVar(&forceRefresh, "force-refresh", false, "allow interactive browser sign-in on this run")
	rootCmd.AddCommand(waybarCmd)
}
