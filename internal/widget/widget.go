package widget

import (
	"calendar-widget/internal/calendar"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Config struct {
	RefreshInterval int
	Compact         bool
	Debug           bool
}

type Widget struct {
	config          *Config
	calendarService *calendar.CalendarService
}

type model struct {
	nextMeeting *calendar.Event
	events      []calendar.Event
	lastUpdate  time.Time
	err         error
	config      *Config
	service     *calendar.CalendarService
}

type tickMsg time.Time
type eventsMsg []calendar.Event
type meetingMsg *calendar.Event
type errMsg error

func NewWidget(config *Config) (*Widget, error) {
	return NewWidgetWithOptions(config, true)
}

func NewWidgetWithOptions(config *Config, allowInteractive bool) (*Widget, error) {
	calendarService, err := calendar.NewCalendarServiceWithOptions(allowInteractive)
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	return &Widget{
		config:          config,
		calendarService: calendarService,
	}, nil
}

func (w *Widget) GetCalendarService() *calendar.CalendarService {
	return w.calendarService
}

func (w *Widget) Run() error {
	p := tea.NewProgram(initialModel(w.config, w.calendarService), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (w *Widget) ShowTooltip() error {
	ctx := context.Background()

	// Get both today's events and upcoming events
	todaysEvents, err := w.calendarService.GetTodaysEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get today's events: %w", err)
	}

	upcomingEvents, err := w.calendarService.GetUpcomingEvents(ctx)
	if err != nil {
		return fmt.Errorf("failed to get upcoming events: %w", err)
	}

	fmt.Print(renderExtendedTooltip(todaysEvents, upcomingEvents))
	return nil
}

func (w *Widget) RunWaybar() error {
	return w.RunWaybarWithRefresh(false)
}

func (w *Widget) RunWaybarWithRefresh(forceRefresh bool) error {
	// For waybar mode, run once and exit instead of looping
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use service with interactive auth if force refresh requested
	service := w.calendarService
	if forceRefresh {
		// Create a new service with interactive auth enabled
		refreshService, err := calendar.NewCalendarServiceWithOptions(true)
		if err != nil {
			output := WaybarOutput{
				Text:    "Auth Error",
				Class:   "error",
				Alt:     "auth-error",
				Tooltip: "Failed to create calendar service",
			}
			jsonBytes, _ := json.Marshal(output)
			fmt.Println(string(jsonBytes))
			return nil
		}
		service = refreshService
	}

	// Get upcoming events for main display
	upcomingEvents, err := service.GetUpcomingEvents(ctx)
	if err != nil {
		// Check if this is an authentication error
		if strings.Contains(err.Error(), "authentication") ||
			strings.Contains(err.Error(), "token") ||
			strings.Contains(err.Error(), "login") {
			output := WaybarOutput{
				Text:    "Auth Required",
				Class:   "error",
				Alt:     "auth-required",
				Tooltip: "Click to authenticate",
			}
			jsonBytes, _ := json.Marshal(output)
			fmt.Println(string(jsonBytes))
		} else {
			output := WaybarOutput{
				Text:    "Calendar Error",
				Class:   "error",
				Alt:     "error",
				Tooltip: err.Error(),
			}
			jsonBytes, _ := json.Marshal(output)
			fmt.Println(string(jsonBytes))
		}
		return nil
	}

	// Get today's events for tooltip
	todaysEvents, _ := service.GetTodaysEvents(ctx)

	// Find the most relevant upcoming meeting to display with blocking priority
	displayEvent := selectBestEvent(upcomingEvents)

	if displayEvent == nil {
		output := WaybarOutput{
			Text:    "No upcoming meetings",
			Class:   "no-meeting",
			Alt:     "no-meeting",
			Tooltip: generateTooltipForSchedule(todaysEvents),
		}
		jsonBytes, _ := json.Marshal(output)
		fmt.Println(string(jsonBytes))
		return nil
	}

	output := generateWaybarOutputForSchedule(displayEvent, todaysEvents)
	jsonBytes, _ := json.Marshal(output)
	fmt.Println(string(jsonBytes))

	return nil
}

func initialModel(config *Config, service *calendar.CalendarService) model {
	return model{
		config:  config,
		service: service,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		fetchEventsCmd(m.service),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "enter", " ":
			if m.nextMeeting != nil {
				return m, openMeetingCmd(*m.nextMeeting)
			}
		case "r":
			return m, fetchEventsCmd(m.service)
		}

	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonLeft && m.nextMeeting != nil {
			return m, openMeetingCmd(*m.nextMeeting)
		}

	case tickMsg:
		return m, tea.Batch(
			tickCmd(),
			fetchEventsCmd(m.service),
		)

	case eventsMsg:
		m.events = []calendar.Event(msg)
		m.lastUpdate = time.Now()

		ctx := context.Background()
		nextMeeting, _ := m.service.GetNextMeeting(ctx)
		m.nextMeeting = nextMeeting

		return m, nil

	case meetingMsg:
		m.nextMeeting = (*calendar.Event)(msg)
		return m, nil

	case errMsg:
		m.err = error(msg)
		return m, nil
	}

	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	if m.nextMeeting == nil {
		return noMeetingStyle.Render("No upcoming meetings")
	}

	return renderMeeting(*m.nextMeeting, m.config.Compact)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Duration(60)*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func fetchEventsCmd(service *calendar.CalendarService) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		events, err := service.GetTodaysEvents(ctx)
		if err != nil {
			return errMsg(err)
		}

		return eventsMsg(events)
	}
}

func openMeetingCmd(event calendar.Event) tea.Cmd {
	return func() tea.Msg {
		if err := openMeeting(event); err != nil {
			return errMsg(err)
		}
		return nil
	}
}

func openMeeting(event calendar.Event) error {
	var url string
	if event.IsTeams && event.TeamsLink != "" {
		url = event.TeamsLink
	} else if event.WebLink != "" {
		url = event.WebLink
	} else {
		return fmt.Errorf("no link available for meeting")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return fmt.Errorf("unsupported platform")
	}

	return cmd.Start()
}

var (
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true)

	noMeetingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Italic(true)

	urgentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#FF0000")).
			Bold(true).
			Padding(0, 1)

	soonStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Background(lipgloss.Color("#FFA500")).
			Bold(true).
			Padding(0, 1)

	upcomingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#0080FF")).
			Padding(0, 1)

	currentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#00FF00")).
			Bold(true).
			Padding(0, 1)

	pastStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666")).
			Strikethrough(true)

	timeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			MarginRight(1)

	titleStyle = lipgloss.NewStyle().
			Bold(true)

	teamsIndicatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0078D4")).
				Bold(true)
)

func renderMeeting(event calendar.Event, compact bool) string {
	status := event.GetStatus()
	timeUntil := event.GetTimeUntil()

	var statusIndicator string
	var style lipgloss.Style

	switch status {
	case "urgent":
		style = urgentStyle
		statusIndicator = "🔴"
	case "soon":
		style = soonStyle
		statusIndicator = "🟡"
	case "current":
		style = currentStyle
		statusIndicator = "🟢"
	case "upcoming":
		style = upcomingStyle
		statusIndicator = "🔵"
	case "past":
		style = pastStyle
		statusIndicator = "⚫"
	}

	title := event.Subject
	if len(title) > 30 && compact {
		title = title[:27] + "..."
	}

	timeStr := event.Start.Format("15:04")
	if status == "current" {
		endTime := event.End.Format("15:04")
		timeStr = fmt.Sprintf("%s-%s", timeStr, endTime)
	} else if status == "upcoming" || status == "soon" || status == "urgent" {
		if timeUntil < time.Hour {
			timeStr = fmt.Sprintf("in %dm", int(timeUntil.Minutes()))
		} else {
			timeStr = fmt.Sprintf("in %dh%dm", int(timeUntil.Hours()), int(timeUntil.Minutes())%60)
		}
	}

	var parts []string
	parts = append(parts, statusIndicator)

	if event.IsTeams {
		parts = append(parts, teamsIndicatorStyle.Render("Teams"))
	}

	parts = append(parts, timeStyle.Render(timeStr))
	parts = append(parts, titleStyle.Render(title))

	content := strings.Join(parts, " ")

	if compact {
		return style.Render(content)
	}

	return style.Render(content)
}

type WaybarOutput struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip,omitempty"`
	Class   string `json:"class,omitempty"`
	Alt     string `json:"alt,omitempty"`
}

func generateWaybarOutput(meeting *calendar.Event) WaybarOutput {
	if meeting == nil {
		return WaybarOutput{
			Text:  "No meetings",
			Class: "no-meeting",
			Alt:   "no-meeting",
		}
	}

	status := meeting.GetStatus()
	timeUntil := meeting.GetTimeUntil()

	var text, class, alt string

	subject := escapePangoMarkup(meeting.Subject)

	switch status {
	case "urgent":
		text = fmt.Sprintf("🔴 %s", subject)
		if len(text) > 50 {
			text = fmt.Sprintf("🔴 %s...", subject[:45])
		}
		class = "urgent"
		alt = "urgent"
	case "soon":
		text = fmt.Sprintf("🟡 %s", subject)
		if len(text) > 50 {
			text = fmt.Sprintf("🟡 %s...", subject[:45])
		}
		class = "soon"
		alt = "soon"
	case "current":
		text = fmt.Sprintf("🟢 %s", subject)
		if len(text) > 50 {
			text = fmt.Sprintf("🟢 %s...", subject[:45])
		}
		class = "current"
		alt = "current"
	case "upcoming":
		if timeUntil < time.Hour {
			text = fmt.Sprintf("🔵 %s (in %dm)", subject, int(timeUntil.Minutes()))
		} else {
			text = fmt.Sprintf("🔵 %s (in %dh%dm)", subject, int(timeUntil.Hours()), int(timeUntil.Minutes())%60)
		}
		if len(text) > 50 {
			text = fmt.Sprintf("🔵 %s...", subject[:40])
		}
		class = "upcoming"
		alt = "upcoming"
	case "past":
		text = fmt.Sprintf("⚫ %s", subject)
		if len(text) > 50 {
			text = fmt.Sprintf("⚫ %s...", subject[:45])
		}
		class = "past"
		alt = "past"
	}

	if meeting.IsTeams {
		text = "[T] " + text
	}

	return WaybarOutput{
		Text:  text,
		Class: class,
		Alt:   alt,
	}
}

func escapePangoMarkup(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func generateWaybarOutputForSchedule(displayEvent *calendar.Event, allEvents []calendar.Event) WaybarOutput {
	if displayEvent == nil {
		return WaybarOutput{
			Text:    "No meetings today",
			Class:   "no-meeting",
			Alt:     "no-meeting",
			Tooltip: "No meetings scheduled for today",
		}
	}

	// Generate the main display text
	baseOutput := generateWaybarOutput(displayEvent)

	// Generate tooltip with full day schedule
	var tooltipLines []string
	tooltipLines = append(tooltipLines, "📅 Today's Schedule:")
	tooltipLines = append(tooltipLines, "")

	if len(allEvents) == 0 {
		tooltipLines = append(tooltipLines, "No meetings today")
	} else {
		for _, event := range allEvents {
			timeStr := fmt.Sprintf("%s-%s",
				event.Start.Format("15:04"),
				event.End.Format("15:04"))

			status := event.GetStatus()
			var indicator string
			switch status {
			case "current":
				indicator = "🟢"
			case "urgent":
				indicator = "🔴"
			case "soon":
				indicator = "🟡"
			case "upcoming":
				indicator = "🔵"
			case "past":
				indicator = "⚫"
			default:
				indicator = "📅"
			}

			title := escapePangoMarkup(event.Subject)
			if event.IsTeams {
				title = title + " (Teams)"
			}

			if event.Location != "" && !event.IsTeams {
				title = title + " @ " + escapePangoMarkup(event.Location)
			}

			line := fmt.Sprintf("%s %s %s", indicator, timeStr, title)
			tooltipLines = append(tooltipLines, line)
		}

		tooltipLines = append(tooltipLines, "")
		tooltipLines = append(tooltipLines, "💡 Click to open meeting link")
		if displayEvent.IsTeams {
			tooltipLines = append(tooltipLines, "🔗 Teams meeting - will open directly in Teams")
		} else {
			tooltipLines = append(tooltipLines, "🌐 Will open in browser")
		}
	}

	baseOutput.Tooltip = strings.Join(tooltipLines, "\n")
	return baseOutput
}

func generateTooltipForSchedule(todaysEvents []calendar.Event) string {
	var tooltipLines []string
	tooltipLines = append(tooltipLines, "📅 Today's Schedule:")
	tooltipLines = append(tooltipLines, "")

	if len(todaysEvents) == 0 {
		tooltipLines = append(tooltipLines, "No meetings today")
	} else {
		for _, event := range todaysEvents {
			timeStr := fmt.Sprintf("%s-%s",
				event.Start.Format("15:04"),
				event.End.Format("15:04"))

			status := event.GetStatus()
			var indicator string
			switch status {
			case "current":
				indicator = "🟢"
			case "urgent":
				indicator = "🔴"
			case "soon":
				indicator = "🟡"
			case "upcoming":
				indicator = "🔵"
			case "past":
				indicator = "⚫"
			default:
				indicator = "📅"
			}

			title := escapePangoMarkup(event.Subject)
			if event.IsTeams {
				title = title + " (Teams)"
			}

			if event.Location != "" && !event.IsTeams {
				title = title + " @ " + escapePangoMarkup(event.Location)
			}

			line := fmt.Sprintf("%s %s %s", indicator, timeStr, title)
			tooltipLines = append(tooltipLines, line)
		}
	}

	return strings.Join(tooltipLines, "\n")
}

func selectBestEvent(events []calendar.Event) *calendar.Event {
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

func renderExtendedTooltip(todaysEvents []calendar.Event, upcomingEvents []calendar.Event) string {
	var lines []string

	// Today's events
	lines = append(lines, titleStyle.Render("📅 Today's Schedule"))
	lines = append(lines, "")

	if len(todaysEvents) == 0 {
		lines = append(lines, "No meetings today")
	} else {
		for _, event := range todaysEvents {
			timeStr := fmt.Sprintf("%s-%s",
				event.Start.Format("15:04"),
				event.End.Format("15:04"))

			status := event.GetStatus()
			var indicator string
			switch status {
			case "current":
				indicator = "🟢"
			case "urgent":
				indicator = "🔴"
			case "soon":
				indicator = "🟡"
			case "upcoming":
				indicator = "🔵"
			case "past":
				indicator = "⚫"
			default:
				indicator = "📅"
			}

			title := event.Subject
			if event.IsTeams {
				title = title + " (Teams)"
			}

			if event.Location != "" && !event.IsTeams {
				title = title + " @ " + event.Location
			}

			line := fmt.Sprintf("%s %s %s", indicator, timeStyle.Render(timeStr), title)
			lines = append(lines, line)
		}
	}

	// Upcoming events (next 7 days)
	lines = append(lines, "")
	lines = append(lines, titleStyle.Render("🔮 Upcoming Events"))
	lines = append(lines, "")

	if len(upcomingEvents) == 0 {
		lines = append(lines, "No upcoming meetings")
	} else {
		now := time.Now()
		for i, event := range upcomingEvents {
			// Show only next 5 events to keep tooltip manageable
			if i >= 5 {
				lines = append(lines, fmt.Sprintf("... and %d more events", len(upcomingEvents)-5))
				break
			}

			// Format date and time
			var dateTimeStr string
			if event.Start.Format("2006-01-02") == now.Format("2006-01-02") {
				// Today - just show time
				dateTimeStr = event.Start.Format("15:04")
			} else if event.Start.Format("2006-01-02") == now.AddDate(0, 0, 1).Format("2006-01-02") {
				// Tomorrow - show "Tomorrow 15:04"
				dateTimeStr = "Tomorrow " + event.Start.Format("15:04")
			} else {
				// Other days - show "Mon 24/9 15:04"
				dateTimeStr = event.Start.Format("Mon 2/1 15:04")
			}

			status := event.GetStatus()
			var indicator string
			switch status {
			case "current":
				indicator = "🟢"
			case "urgent":
				indicator = "🔴"
			case "soon":
				indicator = "🟡"
			case "upcoming":
				indicator = "🔵"
			case "past":
				indicator = "⚫"
			default:
				indicator = "📅"
			}

			title := event.Subject
			if event.IsTeams {
				title = title + " (Teams)"
			}

			if event.Location != "" && !event.IsTeams {
				title = title + " @ " + event.Location
			}

			line := fmt.Sprintf("%s %s %s", indicator, timeStyle.Render(dateTimeStr), title)
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}
