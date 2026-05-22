package calendar

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"calendar-widget/internal/auth"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	"github.com/microsoftgraph/msgraph-sdk-go-core/authentication"
	"github.com/microsoftgraph/msgraph-sdk-go/users"
)

type Event struct {
	Subject   string
	Start     time.Time
	End       time.Time
	Location  string
	WebLink   string
	TeamsLink string
	IsTeams   bool
	IsAllDay  bool
	Organizer string
	Attendees []string
	Body      string
}

type CalendarService struct {
	client *msgraphsdk.GraphServiceClient
}

func NewCalendarService() (*CalendarService, error) {
	return NewCalendarServiceWithOptions(true)
}

func NewCalendarServiceWithOptions(allowInteractive bool) (*CalendarService, error) {
	// Create a custom credential that respects interactive mode
	credential := &nonInteractiveCredential{
		allowInteractive: allowInteractive,
	}

	authProvider, err := authentication.NewAzureIdentityAuthenticationProviderWithScopes(credential, []string{
		"https://graph.microsoft.com/Calendars.Read",
		"https://graph.microsoft.com/User.Read",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create auth provider: %w", err)
	}

	adapter, err := msgraphsdk.NewGraphRequestAdapter(authProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to create adapter: %w", err)
	}

	client := msgraphsdk.NewGraphServiceClient(adapter)

	return &CalendarService{client: client}, nil
}

// nonInteractiveCredential wraps the authentication to control interactive behavior
type nonInteractiveCredential struct {
	allowInteractive bool
}

func (nic *nonInteractiveCredential) GetToken(ctx context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return auth.GetAccessToken(ctx, nic.allowInteractive)
}

const graphTimeFormat = "2006-01-02T15:04:05.000Z"

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func (cs *CalendarService) GetTodaysEvents(ctx context.Context) ([]Event, error) {
	start := startOfDay(time.Now())
	end := start.Add(24 * time.Hour)
	return cs.getEventsWithCalendarView(ctx, start.UTC().Format(graphTimeFormat), end.UTC().Format(graphTimeFormat))
}

func (cs *CalendarService) GetUpcomingEvents(ctx context.Context) ([]Event, error) {
	now := time.Now()
	end := now.Add(7 * 24 * time.Hour)
	return cs.getEventsWithCalendarView(ctx, now.UTC().Format(graphTimeFormat), end.UTC().Format(graphTimeFormat))
}

// GetEventsFromTodayThroughWeek returns every event from start-of-today to 7 days
// from now in a single Graph API call. Callers can partition the result locally
// into "today" and "upcoming" buckets to avoid two round trips.
func (cs *CalendarService) GetEventsFromTodayThroughWeek(ctx context.Context) ([]Event, error) {
	now := time.Now()
	start := startOfDay(now)
	end := now.Add(7 * 24 * time.Hour)
	return cs.getEventsWithCalendarView(ctx, start.UTC().Format(graphTimeFormat), end.UTC().Format(graphTimeFormat))
}

func (cs *CalendarService) getEventsWithCalendarView(ctx context.Context, startDateTime, endDateTime string) ([]Event, error) {
	requestConfiguration := &users.ItemCalendarViewRequestBuilderGetRequestConfiguration{
		QueryParameters: &users.ItemCalendarViewRequestBuilderGetQueryParameters{
			StartDateTime: &startDateTime,
			EndDateTime:   &endDateTime,
			Orderby:       []string{"start/dateTime"},
			Select:        []string{"subject", "start", "end", "location", "webLink", "body", "organizer", "attendees", "onlineMeeting", "isAllDay"},
			Top:           intPtr(50),
		},
	}

	events, err := cs.client.Me().CalendarView().Get(ctx, requestConfiguration)
	if err != nil {
		return nil, fmt.Errorf("failed to get calendar view: %w", err)
	}

	var result []Event
	for _, event := range events.GetValue() {
		e := Event{
			Subject:  getStringValue(event.GetSubject()),
			Location: getStringValue(event.GetLocation().GetDisplayName()),
			WebLink:  getStringValue(event.GetWebLink()),
			Body:     getStringValue(event.GetBody().GetContent()),
			IsAllDay: getBoolValue(event.GetIsAllDay()),
		}

		if event.GetStart() != nil && event.GetStart().GetDateTime() != nil {
			startStr := getStringValue(event.GetStart().GetDateTime())
			e.Start = parseMicrosoftDateTime(startStr)
		}
		if event.GetEnd() != nil && event.GetEnd().GetDateTime() != nil {
			endStr := getStringValue(event.GetEnd().GetDateTime())
			e.End = parseMicrosoftDateTime(endStr)
		}

		if event.GetOrganizer() != nil && event.GetOrganizer().GetEmailAddress() != nil {
			e.Organizer = getStringValue(event.GetOrganizer().GetEmailAddress().GetName())
		}

		for _, attendee := range event.GetAttendees() {
			if attendee.GetEmailAddress() != nil {
				e.Attendees = append(e.Attendees, getStringValue(attendee.GetEmailAddress().GetName()))
			}
		}

		// Use onlineMeeting field for Teams meetings
		if event.GetOnlineMeeting() != nil {
			e.IsTeams = true
			if event.GetOnlineMeeting().GetJoinUrl() != nil {
				e.TeamsLink = getStringValue(event.GetOnlineMeeting().GetJoinUrl())
			}
		} else {
			// Fallback to body/location parsing for non-standard meeting links
			e.TeamsLink, e.IsTeams = extractTeamsLink(e.Body, e.Location)
		}

		result = append(result, e)
	}

	return result, nil
}

func (cs *CalendarService) GetNextMeeting(ctx context.Context) (*Event, error) {
	events, err := cs.GetUpcomingEvents(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	for _, event := range events {
		if event.Start.After(now) || (event.Start.Before(now) && event.End.After(now)) {
			return &event, nil
		}
	}

	return nil, nil
}

var (
	teamsURLRegexes = []*regexp.Regexp{
		regexp.MustCompile(`https://teams\.microsoft\.com/l/meetup-join/[^\s<>"']+`),
		regexp.MustCompile(`https://teams\.live\.com/meet/[^\s<>"']+`),
		regexp.MustCompile(`https://[a-zA-Z0-9-]+\.teams\.microsoft\.com/[^\s<>"']+`),
	}

	// Lower-cased Teams meeting indicators (English + Danish).
	teamsIndicatorsLower = []string{
		"microsoft teams meeting",
		"teams meeting",
		"join microsoft teams meeting",
		"microsoft teams-møde",
		"teams-møde",
	}
)

func extractTeamsLink(body, location string) (string, bool) {
	content := body + " " + location

	for _, re := range teamsURLRegexes {
		if match := re.FindString(content); match != "" {
			return strings.TrimRight(match, ".,:;!?"), true
		}
	}

	// A Teams indicator phrase with no recognizable Teams URL: flag as Teams
	// but don't return an arbitrary URL from the body — it could point anywhere
	// (event bodies can be attacker-controlled via meeting invites).
	contentLower := strings.ToLower(content)
	for _, indicator := range teamsIndicatorsLower {
		if strings.Contains(contentLower, indicator) {
			return "", true
		}
	}

	return "", false
}

func getStringValue(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func getBoolValue(ptr *bool) bool {
	if ptr == nil {
		return false
	}
	return *ptr
}

func intPtr(i int32) *int32 {
	return &i
}

func parseMicrosoftDateTime(dateTimeStr string) time.Time {
	if dateTimeStr == "" {
		return time.Time{}
	}

	// Microsoft Graph datetime formats to try
	formats := []string{
		"2006-01-02T15:04:05.0000000", // Microsoft's .NET format
		"2006-01-02T15:04:05",         // ISO without fractional seconds
		time.RFC3339,                  // Standard RFC3339
		"2006-01-02T15:04:05Z",        // UTC format
		"2006-01-02T15:04:05.000Z",    // UTC with milliseconds
	}

	for _, format := range formats {
		if parsedTime, err := time.Parse(format, dateTimeStr); err == nil {
			// Microsoft Graph returns times in the user's timezone, but without timezone info
			// We'll assume local timezone if no timezone is specified
			if parsedTime.Location() == time.UTC && !strings.HasSuffix(dateTimeStr, "Z") {
				// Convert to local timezone
				return parsedTime.In(time.Local)
			}
			return parsedTime
		}
	}

	// If all formats fail, return zero time
	return time.Time{}
}

func (e *Event) GetTimeUntil() time.Duration {
	return time.Until(e.Start)
}

func (e *Event) GetStatus() string {
	now := time.Now()
	if now.After(e.End) {
		return "past"
	}
	if now.After(e.Start) && now.Before(e.End) {
		return "current"
	}

	timeUntil := time.Until(e.Start)
	if timeUntil <= 5*time.Minute {
		return "urgent"
	}
	if timeUntil <= 15*time.Minute {
		return "soon"
	}
	return "upcoming"
}

// StatusEmoji returns the indicator emoji matching the event's current status.
// Kept on Event so callers across the widget package agree on the mapping.
func (e *Event) StatusEmoji() string {
	switch e.GetStatus() {
	case "current":
		return "🟢"
	case "urgent":
		return "🔴"
	case "soon":
		return "🟡"
	case "upcoming":
		return "🔵"
	case "past":
		return "⚫"
	default:
		return "📅"
	}
}

func (e *Event) GetDuration() time.Duration {
	return e.End.Sub(e.Start)
}

func (e *Event) IsLongEvent() bool {
	return e.GetDuration() > 4*time.Hour
}

func (e *Event) IsBlockingEvent() bool {
	return !e.IsAllDay && !e.IsLongEvent()
}
