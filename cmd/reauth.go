package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var reauthCmd = &cobra.Command{
	Use:   "reauth",
	Short: "Clear tokens and re-authenticate",
	Long:  `Clear stored tokens and re-authenticate with Microsoft 365.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runReauth(); err != nil {
			fmt.Printf("Re-authentication failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func runReauth() error {
	// Try to clear the keyring cache (Linux-specific)
	cmd := exec.Command("secret-tool", "clear", "service", "calendar-widget")
	_ = cmd.Run() // Ignore errors - keyring may not be available

	fmt.Println("Re-authenticating...")
	fmt.Println("Starting fresh authentication process...")

	// Run setup again
	return runSetup()
}

func init() {
	rootCmd.AddCommand(reauthCmd)
}
