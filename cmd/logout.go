package cmd

import (
	"calendar-widget/internal/auth"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout and clear stored authentication tokens",
	Long:  `Logout from Microsoft 365 and clear any stored authentication tokens and configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runLogout(); err != nil {
			fmt.Printf("Logout failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func runLogout() error {
	fmt.Println("Logging out...")

	// Remove config file
	configPath := auth.GetConfigPath()
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove config file: %w", err)
	}

	// Try to clear the keyring cache (Linux-specific)
	// This may fail silently if secret-tool is not available
	clearKeyring()

	fmt.Println("Successfully logged out!")
	fmt.Println("Configuration has been cleared.")
	fmt.Println()
	fmt.Println("To use the calendar widget again, run: calendar-widget setup")

	return nil
}

func clearKeyring() {
	// Try to clear the keyring entries for calendar-widget
	// This uses secret-tool which is available on most Linux systems with GNOME keyring
	cmd := exec.Command("secret-tool", "clear", "service", "calendar-widget")
	_ = cmd.Run() // Ignore errors - keyring may not be available or already cleared
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
