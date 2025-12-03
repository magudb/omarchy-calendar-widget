package cmd

import (
	"calendar-widget/internal/auth"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate Azure AD configuration",
	Long:  `Validate that your Azure AD application is properly configured for the calendar widget.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runValidate(); err != nil {
			fmt.Printf("Validation failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func runValidate() error {
	fmt.Println("Validating Azure AD Configuration")
	fmt.Println("=================================")
	fmt.Println()

	// Check if config exists
	config, err := auth.LoadConfig()
	if err != nil {
		fmt.Println("❌ Configuration not found. Run 'calendar-widget setup' first.")
		return err
	}

	fmt.Printf("✅ Configuration found\n")
	fmt.Printf("   Client ID: %s\n", config.ClientID)
	fmt.Printf("   Tenant ID: %s\n", config.TenantID)
	fmt.Println()

	// Test authentication
	fmt.Println("Testing authentication...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, err = auth.GetAccessToken(ctx, true) // Allow interactive for validation
	if err != nil {
		fmt.Println("❌ Authentication failed:")
		fmt.Printf("   Error: %v\n", err)
		fmt.Println()

		// Provide specific guidance based on error type
		errorStr := err.Error()
		if strings.Contains(errorStr, "client_assertion") || strings.Contains(errorStr, "client_secret") || strings.Contains(errorStr, "AADSTS7000218") {
			fmt.Println("🔧 SOLUTION: Your Azure AD app is configured as a confidential client.")
			fmt.Println("   You need to enable public client flows:")
			fmt.Println()
			fmt.Println("   1. Go to Azure Portal → Azure AD → App registrations")
			fmt.Println("   2. Select your 'Calendar Widget' app")
			fmt.Println("   3. Go to 'Authentication' tab")
			fmt.Println("   4. Scroll to 'Advanced settings'")
			fmt.Println("   5. Set 'Allow public client flows' to YES")
			fmt.Println("   6. Click 'Save'")
			fmt.Println()
		} else if strings.Contains(errorStr, "insufficient_privileges") || strings.Contains(errorStr, "need admin approval") {
			fmt.Println("🔧 SOLUTION: Missing permissions or admin consent required.")
			fmt.Println("   1. Go to your Azure AD app → 'API permissions' tab")
			fmt.Println("   2. Ensure 'Calendars.Read' and 'User.Read' are added")
			fmt.Println("   3. Click 'Grant admin consent for [organization]'")
			fmt.Println()
		} else if strings.Contains(errorStr, "invalid_client") || strings.Contains(errorStr, "Application not found") {
			fmt.Println("🔧 SOLUTION: Invalid Client ID or Tenant ID.")
			fmt.Println("   1. Go to your Azure AD app → 'Overview' tab")
			fmt.Println("   2. Copy the correct Application (client) ID")
			fmt.Println("   3. Copy the correct Directory (tenant) ID")
			fmt.Println("   4. Run 'calendar-widget setup' again")
			fmt.Println()
		} else {
			fmt.Println("🔧 Common solutions:")
			fmt.Println("   1. Make sure 'Allow public client flows' is enabled in Azure AD app")
			fmt.Println("   2. Ensure the app has 'Calendars.Read' and 'User.Read' permissions")
			fmt.Println("   3. Check if admin consent is required and granted")
			fmt.Println("   4. Verify the Client ID and Tenant ID are correct")
			fmt.Println()
		}

		fmt.Println("📖 For detailed troubleshooting, see: TROUBLESHOOTING.md")
		return err
	}

	fmt.Println("✅ Authentication successful!")
	fmt.Println()
	fmt.Println("Your Azure AD configuration is working correctly.")

	return nil
}

func init() {
	rootCmd.AddCommand(validateCmd)
}
