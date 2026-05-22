package calendar

import (
	"testing"
	"time"
)

func TestExtractTeamsLink_AcceptsCanonicalTeamsURLs(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "teams.microsoft.com meetup-join",
			body: "Join here: https://teams.microsoft.com/l/meetup-join/19%3aMEETING/0",
			want: "https://teams.microsoft.com/l/meetup-join/19%3aMEETING/0",
		},
		{
			name: "teams.live.com",
			body: "Personal meet: https://teams.live.com/meet/abc123def",
			want: "https://teams.live.com/meet/abc123def",
		},
		{
			name: "tenant subdomain",
			body: "Click https://tenant1.teams.microsoft.com/l/meetup-join/xyz",
			want: "https://tenant1.teams.microsoft.com/l/meetup-join/xyz",
		},
		{
			name: "trailing punctuation stripped",
			body: "See https://teams.microsoft.com/l/meetup-join/19%3aABC.",
			want: "https://teams.microsoft.com/l/meetup-join/19%3aABC",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, isTeams := extractTeamsLink(tc.body, "")
			if !isTeams {
				t.Errorf("expected IsTeams=true")
			}
			if gotURL != tc.want {
				t.Errorf("URL = %q, want %q", gotURL, tc.want)
			}
		})
	}
}

// TestExtractTeamsLink_DoesNotReturnArbitraryURL is the regression test for
// the security finding: if the event body says "Microsoft Teams Meeting" but
// the URL inside is to some random domain, we must NOT label it a Teams link.
func TestExtractTeamsLink_DoesNotReturnArbitraryURL(t *testing.T) {
	body := "Microsoft Teams Meeting\nhttps://evil.example.com/phish?as=teams"
	gotURL, isTeams := extractTeamsLink(body, "")

	if !isTeams {
		t.Error("expected IsTeams=true because the indicator string is present")
	}
	if gotURL != "" {
		t.Errorf("URL = %q, want empty — non-Teams-domain URLs must not be returned even with a Teams indicator", gotURL)
	}
}

func TestExtractTeamsLink_DanishIndicator(t *testing.T) {
	body := "Microsoft Teams-møde\nLink: https://teams.microsoft.com/l/meetup-join/19%3aDK"
	gotURL, isTeams := extractTeamsLink(body, "")
	if !isTeams || gotURL == "" {
		t.Errorf("Danish indicator failed: got URL=%q isTeams=%v", gotURL, isTeams)
	}
}

func TestExtractTeamsLink_NoTeamsHint(t *testing.T) {
	body := "Standup at the whiteboard, see you there"
	gotURL, isTeams := extractTeamsLink(body, "")
	if isTeams || gotURL != "" {
		t.Errorf("expected no Teams detection, got URL=%q isTeams=%v", gotURL, isTeams)
	}
}

func TestEventStatusEmoji(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name  string
		start time.Time
		end   time.Time
		want  string
	}{
		{"past", now.Add(-2 * time.Hour), now.Add(-time.Hour), "⚫"},
		{"current", now.Add(-10 * time.Minute), now.Add(10 * time.Minute), "🟢"},
		{"urgent", now.Add(2 * time.Minute), now.Add(32 * time.Minute), "🔴"},
		{"soon", now.Add(10 * time.Minute), now.Add(40 * time.Minute), "🟡"},
		{"upcoming", now.Add(2 * time.Hour), now.Add(3 * time.Hour), "🔵"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := Event{Start: tc.start, End: tc.end}
			got := e.StatusEmoji()
			if got != tc.want {
				t.Errorf("StatusEmoji() = %q, want %q", got, tc.want)
			}
		})
	}
}
