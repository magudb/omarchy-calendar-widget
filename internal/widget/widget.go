package widget

import (
	"calendar-widget/internal/auth"
	"calendar-widget/internal/calendar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	// Max runes of subject text we show on the waybar bar before truncating.
	waybarMaxSubjectRunes = 45
	// Max runes for the compact TUI title.
	compactTitleMaxRunes = 30
	// Single timeout for the run-once waybar fetch.
	waybarFetchTimeout = 30 * time.Second
	// How many upcoming events to enumerate in the extended tooltip.
	extendedTooltipMaxEvents = 5
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
	// Waybar invokes this binary on its own `interval` and reads one JSON line
	// per invocation, so we deliberately run once and exit.
	ctx, cancel := context.WithTimeout(context.Background(), waybarFetchTimeout)
	defer cancel()

	// The calendar service constructed in cmd/waybar.go already received
	// allowInteractive=forceRefresh, so we don't rebuild it here.
	events, err := w.calendarService.GetEventsFromTodayThroughWeek(ctx)
	if err != nil {
		emitWaybar(errorOutput(err, forceRefresh))
		return nil
	}

	todaysEvents, upcomingEvents := splitTodayAndUpcoming(events, time.Now())

	displayEvent := selectBestEvent(upcomingEvents)
	if displayEvent == nil {
		emitWaybar(WaybarOutput{
			Text:    "No upcoming meetings",
			Class:   "no-meeting",
			Alt:     "no-meeting",
			Tooltip: buildScheduleTooltip(todaysEvents, nil),
		})
		return nil
	}

	emitWaybar(generateWaybarOutputForSchedule(displayEvent, todaysEvents))
	return nil
}

// emitWaybar writes one JSON line to stdout. If encoding ever fails we log to
// stderr so waybar at least shows its previous text instead of a silent blank.
func emitWaybar(out WaybarOutput) {
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(out); err != nil {
		log.Printf("waybar: failed to encode output: %v", err)
	}
}

// errorOutput maps a calendar error to a user-friendly waybar payload.
// Auth errors are detected via the typed sentinel from the auth package so we
// don't depend on locale-specific error text.
func errorOutput(err error, forceRefreshTried bool) WaybarOutput {
	if errors.Is(err, auth.ErrLoginRequired) {
		text := "Auth Required"
		tip := "Click to sign in to your Microsoft account"
		if forceRefreshTried {
			text = "Auth Failed"
			tip = "Sign-in attempt did not complete — click to retry"
		}
		return WaybarOutput{Text: text, Class: "error", Alt: "auth-required", Tooltip: tip}
	}
	return WaybarOutput{
		Text:    "Calendar Error",
		Class:   "error",
		Alt:     "error",
		Tooltip: "Couldn't load your calendar. Run `calendar-widget debug` for details.",
	}
}

// splitTodayAndUpcoming partitions a single events slice (start-of-today → +7d)
// into the two views the waybar UI needs, avoiding a second Graph round-trip.
//
//   - todays:   events whose start falls inside [start-of-today, end-of-today).
//   - upcoming: events that haven't ended yet (current or future).
func splitTodayAndUpcoming(events []calendar.Event, now time.Time) (todays, upcoming []calendar.Event) {
	endOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(24 * time.Hour)
	for _, e := range events {
		if e.Start.Before(endOfToday) {
			todays = append(todays, e)
		}
		if e.End.After(now) {
			upcoming = append(upcoming, e)
		}
	}
	return todays, upcoming
}

func initialModel(config *Config, service *calendar.CalendarService) model {
	return model{
		config:  config,
		service: service,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(m.refreshInterval()),
		fetchEventsCmd(m.service),
	)
}

// refreshInterval reads --refresh from config, defaulting to 60s when unset.
func (m model) refreshInterval() time.Duration {
	if m.config == nil || m.config.RefreshInterval <= 0 {
		return 60 * time.Second
	}
	return time.Duration(m.config.RefreshInterval) * time.Second
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
			tickCmd(m.refreshInterval()),
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

func tickCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return tea.Tick(interval, func(t time.Time) tea.Msg {
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

// statusStyle returns the lipgloss style used for the given event status in
// the TUI. Returns the zero Style for unknown statuses (renders unstyled).
func statusStyle(status string) lipgloss.Style {
	switch status {
	case "urgent":
		return urgentStyle
	case "soon":
		return soonStyle
	case "current":
		return currentStyle
	case "upcoming":
		return upcomingStyle
	case "past":
		return pastStyle
	}
	return lipgloss.Style{}
}

func renderMeeting(event calendar.Event, compact bool) string {
	status := event.GetStatus()
	timeUntil := event.GetTimeUntil()
	style := statusStyle(status)

	title := event.Subject
	if compact {
		title = truncateRunes(title, compactTitleMaxRunes)
	}

	timeStr := event.Start.Format("15:04")
	switch status {
	case "current":
		timeStr = fmt.Sprintf("%s-%s", timeStr, event.End.Format("15:04"))
	case "upcoming", "soon", "urgent":
		timeStr = humanCountdown(timeUntil)
	}

	parts := []string{event.StatusEmoji()}
	if event.IsTeams {
		parts = append(parts, teamsIndicatorStyle.Render("Teams"))
	}
	parts = append(parts, timeStyle.Render(timeStr), titleStyle.Render(title))

	return style.Render(strings.Join(parts, " "))
}

// humanCountdown formats a positive duration as "in 12m" or "in 1h05m".
func humanCountdown(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	}
	return fmt.Sprintf("in %dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// truncateRunes shortens s to at most max user-perceived characters, appending
// "…" when truncated. Operates on runes so multibyte UTF-8 sequences (e.g.
// Danish ø/æ/å, emoji) are never sliced mid-codepoint.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(r[:max-1]) + "…"
}

type WaybarOutput struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip,omitempty"`
	Class   string `json:"class,omitempty"`
	Alt     string `json:"alt,omitempty"`
}

func generateWaybarOutput(meeting *calendar.Event) WaybarOutput {
	if meeting == nil {
		return WaybarOutput{Text: "No meetings", Class: "no-meeting", Alt: "no-meeting"}
	}

	status := meeting.GetStatus()
	subject := escapePangoMarkup(meeting.Subject)

	// Add the countdown suffix first (only for genuinely future events), then
	// truncate the subject so the time information is never lost.
	suffix := ""
	if status == "upcoming" {
		suffix = " (" + humanCountdown(meeting.GetTimeUntil()) + ")"
	}

	text := fmt.Sprintf("%s %s%s", meeting.StatusEmoji(), truncateRunes(subject, waybarMaxSubjectRunes), suffix)
	if meeting.IsTeams {
		text = "[T] " + text
	}

	return WaybarOutput{Text: text, Class: status, Alt: status}
}

func escapePangoMarkup(s string) string {
	// `&` must be replaced first so we don't double-escape the entities we add.
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// scheduleFooter is appended to the tooltip when we have a chosen display
// event — it tells the user what clicking the widget will do. nil for the
// no-meeting tooltip.
type scheduleFooter struct {
	isTeams bool
}

// buildScheduleTooltip formats today's schedule as a Pango-safe tooltip body.
// When footer is non-nil, a "click to open" hint is appended below the list.
func buildScheduleTooltip(events []calendar.Event, footer *scheduleFooter) string {
	lines := []string{"📅 Today's Schedule:", ""}

	if len(events) == 0 {
		lines = append(lines, "No meetings today")
		return strings.Join(lines, "\n")
	}

	for _, ev := range events {
		timeStr := fmt.Sprintf("%s-%s", ev.Start.Format("15:04"), ev.End.Format("15:04"))
		title := escapePangoMarkup(ev.Subject)
		if ev.IsTeams {
			title += " (Teams)"
		} else if ev.Location != "" {
			title += " @ " + escapePangoMarkup(ev.Location)
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", ev.StatusEmoji(), timeStr, title))
	}

	if footer != nil {
		lines = append(lines, "", "💡 Click to open meeting link")
		if footer.isTeams {
			lines = append(lines, "🔗 Teams meeting - will open directly in Teams")
		} else {
			lines = append(lines, "🌐 Will open in browser")
		}
	}

	return strings.Join(lines, "\n")
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
	out := generateWaybarOutput(displayEvent)
	out.Tooltip = buildScheduleTooltip(allEvents, &scheduleFooter{isTeams: displayEvent.IsTeams})
	return out
}

// selectBestEvent picks the meeting to surface on the waybar bar. It prefers
// "blocking" events (not all-day, not multi-hour blocks) across all status
// tiers before considering non-blocking ones — so a meeting starting in 3
// minutes wins over an all-day "Out of Office" event currently in progress.
func selectBestEvent(events []calendar.Event) *calendar.Event {
	if len(events) == 0 {
		return nil
	}
	now := time.Now()
	statusPriority := []string{"current", "urgent", "soon", "upcoming"}

	pick := func(blockingOnly bool) *calendar.Event {
		for _, want := range statusPriority {
			for i := range events {
				ev := events[i]
				if ev.GetStatus() != want {
					continue
				}
				if blockingOnly && !ev.IsBlockingEvent() {
					continue
				}
				if want == "upcoming" && !ev.Start.After(now) {
					continue
				}
				return &ev
			}
		}
		return nil
	}

	if ev := pick(true); ev != nil {
		return ev
	}
	return pick(false)
}

func renderExtendedTooltip(todaysEvents []calendar.Event, upcomingEvents []calendar.Event) string {
	lines := []string{titleStyle.Render("📅 Today's Schedule"), ""}

	if len(todaysEvents) == 0 {
		lines = append(lines, "No meetings today")
	} else {
		for _, ev := range todaysEvents {
			timeStr := fmt.Sprintf("%s-%s", ev.Start.Format("15:04"), ev.End.Format("15:04"))
			lines = append(lines, fmt.Sprintf("%s %s %s",
				ev.StatusEmoji(), timeStyle.Render(timeStr), formatTitleWithDecorations(ev)))
		}
	}

	lines = append(lines, "", titleStyle.Render("🔮 Upcoming Events"), "")

	if len(upcomingEvents) == 0 {
		lines = append(lines, "No upcoming meetings")
		return strings.Join(lines, "\n")
	}

	now := time.Now()
	for i, ev := range upcomingEvents {
		if i >= extendedTooltipMaxEvents {
			lines = append(lines, fmt.Sprintf("... and %d more events", len(upcomingEvents)-extendedTooltipMaxEvents))
			break
		}
		lines = append(lines, fmt.Sprintf("%s %s %s",
			ev.StatusEmoji(), timeStyle.Render(formatUpcomingWhen(ev.Start, now)), formatTitleWithDecorations(ev)))
	}

	return strings.Join(lines, "\n")
}

// formatTitleWithDecorations appends "(Teams)" or "@ location" to the subject
// for the TUI tooltip. Not Pango-escaped because the TUI renders plain text.
func formatTitleWithDecorations(ev calendar.Event) string {
	title := ev.Subject
	if ev.IsTeams {
		return title + " (Teams)"
	}
	if ev.Location != "" {
		return title + " @ " + ev.Location
	}
	return title
}

// formatUpcomingWhen formats an event start time relative to now: bare time
// for today, "Tomorrow HH:MM" for the next day, otherwise weekday + date.
func formatUpcomingWhen(start, now time.Time) string {
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	switch start.Format("2006-01-02") {
	case today:
		return start.Format("15:04")
	case tomorrow:
		return "Tomorrow " + start.Format("15:04")
	default:
		return start.Format("Mon 2/1 15:04")
	}
}
