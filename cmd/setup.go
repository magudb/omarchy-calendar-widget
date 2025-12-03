package cmd

import (
	"bufio"
	"calendar-widget/internal/auth"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Setup Microsoft 365 authentication",
	Long: `Setup authentication for Microsoft 365 calendar access.
Requires a Microsoft Entra app registration with the following permissions:
- Calendars.Read
- User.Read`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runSetup(); err != nil {
			fmt.Printf("Setup failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func runSetup() error {
	fmt.Println("Calendar Widget Setup")
	fmt.Println("=====================")
	fmt.Println()
	fmt.Println("This setup requires a Microsoft Entra app registration.")
	fmt.Println()
	fmt.Println("If you haven't created one yet:")
	fmt.Println("1. Go to https://entra.microsoft.com")
	fmt.Println("2. App registrations → New registration")
	fmt.Println("3. Name: Calendar Widget")
	fmt.Println("4. Supported account types: Choose based on your needs")
	fmt.Println("5. Redirect URI: Mobile and desktop applications → http://localhost:12345/auth/callback")
	fmt.Println("6. Add API permissions: Calendars.Read, User.Read")
	fmt.Println("7. Authentication → Allow public client flows → Yes")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter your Application (client) ID: ")
	clientID, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read client ID: %w", err)
	}
	clientID = strings.TrimSpace(clientID)

	if clientID == "" {
		return fmt.Errorf("client ID cannot be empty")
	}

	fmt.Print("Enter your Directory (tenant) ID (or 'common' for multi-tenant): ")
	tenantID, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read tenant ID: %w", err)
	}
	tenantID = strings.TrimSpace(tenantID)

	if tenantID == "" {
		tenantID = "common"
	}

	config := &auth.Config{
		ClientID:    clientID,
		TenantID:    tenantID,
		RedirectURI: auth.RedirectURI,
	}

	if err := auth.SaveConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Println("Starting authentication process...")
	fmt.Println("Your default browser will open for Microsoft login.")
	fmt.Println("Please complete the authentication in your browser.")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	_, err = auth.GetAccessToken(ctx, true)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	fmt.Println()
	fmt.Println("Authentication successful!")
	fmt.Println("Credentials cached for future use.")
	fmt.Println()
	fmt.Println("Setup complete! You can now use the calendar widget.")
	fmt.Println("Try running: calendar-widget waybar")

	return nil
}

func init() {
	rootCmd.AddCommand(setupCmd)
}
