package cmd

import (
	"calendar-widget/internal/calendar"
	"calendar-widget/internal/widget"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var clickCmd = &cobra.Command{
	Use:   "click",
	Short: "Handle calendar widget clicks intelligently",
	Long:  `Handle clicks on the calendar widget. If authentication is required, run reauth. Otherwise, open the current meeting.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runClick(); err != nil {
			fmt.Printf("Click handler failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func runClick() error {
	// First, check what's the current status by running waybar once
	_, err := widget.NewWidgetWithOptions(&widget.Config{
		RefreshInterval: 60,
		Compact:         true,
		Debug:           debug,
	}, false) // Start non-interactive
	if err != nil {
		fmt.Printf("Failed to create widget: %v\n", err)
		return runReauth()
	}

	// Capture output to check for "Auth Required"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Try to get upcoming events to see what the status is
	calendarService, err := calendar.NewCalendarServiceWithOptions(false)
	if err != nil {
		if isAuthError(err) {
			fmt.Println("Authentication required, forcing token refresh...")
			return runClickWithForceRefresh()
		}
		return nil
	}

	upcomingEvents, err := calendarService.GetUpcomingEvents(ctx)
	if err != nil {
		if isAuthError(err) {
			fmt.Println("Authentication required, forcing token refresh...")
			return runClickWithForceRefresh()
		}
		return nil
	}

	// Find the best event to open using the same prioritization as the widget
	bestEvent := selectBestEventForClick(upcomingEvents)
	if bestEvent != nil {
		status := bestEvent.GetStatus()
		if status == "current" || status == "urgent" {
			if bestEvent.IsTeams && bestEvent.TeamsLink != "" {
				return openMeetingLink(bestEvent.TeamsLink)
			} else if bestEvent.WebLink != "" {
				return openMeetingLink(bestEvent.WebLink)
			}
		}
	}

	// No current/urgent meetings, just run the regular widget
	return nil
}

func runClickWithForceRefresh() error {
	// Create widget with force refresh
	_, err := widget.NewWidgetWithOptions(&widget.Config{
		RefreshInterval: 60,
		Compact:         true,
		Debug:           debug,
	}, true) // Allow interactive for force refresh
	if err != nil {
		fmt.Printf("Failed to create widget with refresh: %v\n", err)
		return runReauth()
	}

	// Try with force refresh
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	calendarService, err := calendar.NewCalendarServiceWithOptions(true) // Allow interactive auth
	if err != nil {
		fmt.Printf("Force refresh failed: %v\n", err)
		return runReauth()
	}

	upcomingEvents, err := calendarService.GetUpcomingEvents(ctx)
	if err != nil {
		if isAuthError(err) {
			fmt.Printf("Force refresh still failed with auth error: %v\n", err)
			return runReauth()
		}
		fmt.Printf("Force refresh failed with error: %v\n", err)
		return nil
	}

	// Find the best event to open using the same prioritization as the widget
	bestEvent := selectBestEventForClick(upcomingEvents)
	if bestEvent != nil {
		status := bestEvent.GetStatus()
		if status == "current" || status == "urgent" {
			if bestEvent.IsTeams && bestEvent.TeamsLink != "" {
				return openMeetingLink(bestEvent.TeamsLink)
			} else if bestEvent.WebLink != "" {
				return openMeetingLink(bestEvent.WebLink)
			}
		}
	}

	// Successfully refreshed but no urgent meetings - just run widget
	return nil
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "token") ||
		strings.Contains(errStr, "login") ||
		strings.Contains(errStr, "unauthorized")
}

func openMeetingLink(url string) error {
	// Use the same logic as the widget's openMeeting function
	var cmd string
	switch {
	case strings.Contains(url, "teams.microsoft.com"):
		// Try to open in Teams app first, fallback to browser
		cmd = fmt.Sprintf(`sh -c 'xdg-open "msteams://" 2>/dev/null && sleep 1 && xdg-open "%s" || xdg-open "%s"'`, url, url)
	default:
		cmd = fmt.Sprintf(`xdg-open "%s"`, url)
	}

	return runBashCommand(cmd)
}

func runBashCommand(command string) error {
	// Execute the command using shell
	exec := exec.Command("sh", "-c", command)
	return exec.Run()
}

func selectBestEventForClick(events []calendar.Event) *calendar.Event {
	if len(events) == 0 {
		return nil
	}

	now := time.Now()
	statusPriority := []string{"current", "urgent", "soon", "upcoming"}

	// For each status level, first look for blocking events, then fall back to any event
	for _, targetStatus := range statusPriority {
		// First pass: find blocking events with this status
		for _, event := range events {
			status := event.GetStatus()
			if status == targetStatus && event.IsBlockingEvent() {
				if targetStatus == "upcoming" && !event.Start.After(now) {
					continue
				}
				return &event
			}
		}

		// Second pass: find any event with this status (fallback for all-day/long events)
		for _, event := range events {
			status := event.GetStatus()
			if status == targetStatus {
				if targetStatus == "upcoming" && !event.Start.After(now) {
					continue
				}
				return &event
			}
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(clickCmd)
}
