package widget

import (
	"calendar-widget/internal/calendar"
	"strings"
	"testing"
	"time"
)

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"empty", "", 10, ""},
		{"fits", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"ascii_truncate", "abcdefghij", 5, "abcd…"},
		{"danish_multibyte_no_corruption", "Møde med ledelsen om implementering", 10, "Møde med …"},
		{"emoji_multibyte_no_corruption", "🎉🎉🎉🎉🎉", 3, "🎉🎉…"},
		{"max_zero", "abc", 0, ""},
		{"max_one", "abc", 1, "…"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRunes(tc.in, tc.max)
			if got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
			}
			if !isValidUTF8(got) {
				t.Errorf("truncateRunes(%q, %d) produced invalid UTF-8: %q", tc.in, tc.max, got)
			}
		})
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == 0xFFFD && !strings.ContainsRune(s, 0xFFFD) {
			return false
		}
	}
	return true
}

// TestSelectBestEvent_AllDayDoesNotOutrankUrgent is the regression test for
// the priority bug: an all-day "current" event (e.g. OOO) must not beat a
// blocking meeting that's about to start.
func TestSelectBestEvent_AllDayDoesNotOutrankUrgent(t *testing.T) {
	now := time.Now()
	allDayOOO := calendar.Event{
		Subject:  "Out of Office",
		Start:    now.Add(-4 * time.Hour),
		End:      now.Add(20 * time.Hour),
		IsAllDay: true,
	}
	urgent := calendar.Event{
		Subject: "Real meeting",
		Start:   now.Add(3 * time.Minute),
		End:     now.Add(33 * time.Minute),
	}

	got := selectBestEvent([]calendar.Event{allDayOOO, urgent})
	if got == nil {
		t.Fatal("selectBestEvent returned nil, expected the urgent meeting")
	}
	if got.Subject != "Real meeting" {
		t.Errorf("selectBestEvent picked %q, want %q (all-day events must not outrank blocking ones)", got.Subject, "Real meeting")
	}
}

func TestSelectBestEvent_PicksCurrentBlockingOverUpcoming(t *testing.T) {
	now := time.Now()
	events := []calendar.Event{
		{Subject: "In progress", Start: now.Add(-10 * time.Minute), End: now.Add(20 * time.Minute)},
		{Subject: "Later today", Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
	}
	got := selectBestEvent(events)
	if got == nil || got.Subject != "In progress" {
		t.Fatalf("got %+v, want In progress", got)
	}
}

func TestSelectBestEvent_FallsBackToNonBlockingWhenNoBlocking(t *testing.T) {
	now := time.Now()
	allDay := calendar.Event{
		Subject:  "All-Day Focus",
		Start:    now.Add(-1 * time.Hour),
		End:      now.Add(10 * time.Hour),
		IsAllDay: true,
	}
	got := selectBestEvent([]calendar.Event{allDay})
	if got == nil || got.Subject != "All-Day Focus" {
		t.Fatalf("got %+v, want All-Day Focus (fallback)", got)
	}
}

func TestSelectBestEvent_Empty(t *testing.T) {
	if selectBestEvent(nil) != nil {
		t.Error("expected nil for empty input")
	}
}

func TestSplitTodayAndUpcoming(t *testing.T) {
	now := time.Date(2026, 5, 22, 14, 0, 0, 0, time.UTC)
	endOfToday := time.Date(2026, 5, 23, 0, 0, 0, 0, time.UTC)
	_ = endOfToday

	events := []calendar.Event{
		{Subject: "Earlier today (ended)", Start: now.Add(-2 * time.Hour), End: now.Add(-1 * time.Hour)},
		{Subject: "In progress", Start: now.Add(-30 * time.Minute), End: now.Add(30 * time.Minute)},
		{Subject: "Later today", Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
		{Subject: "Tomorrow", Start: now.Add(24 * time.Hour), End: now.Add(25 * time.Hour)},
	}

	todays, upcoming := splitTodayAndUpcoming(events, now)

	if len(todays) != 3 {
		t.Errorf("todays: got %d events, want 3 (earlier, in-progress, later)", len(todays))
	}
	if len(upcoming) != 3 {
		t.Errorf("upcoming: got %d events, want 3 (in-progress, later, tomorrow)", len(upcoming))
	}

	// The already-ended event must NOT appear in upcoming.
	for _, e := range upcoming {
		if e.Subject == "Earlier today (ended)" {
			t.Error("ended event leaked into upcoming")
		}
	}
	// The tomorrow event must NOT appear in todays.
	for _, e := range todays {
		if e.Subject == "Tomorrow" {
			t.Error("tomorrow event leaked into todays")
		}
	}
}

func TestGenerateWaybarOutput_UpcomingPreservesCountdown(t *testing.T) {
	// Long Danish subject that would have triggered the old byte-slicing bug
	// AND have its countdown stripped by the old truncation path.
	longSubject := "Ekstraordinært statusmøde med ledelsen om implementering af nye processer i Q3"
	ev := &calendar.Event{
		Subject: longSubject,
		Start:   time.Now().Add(45 * time.Minute),
		End:     time.Now().Add(90 * time.Minute),
	}

	out := generateWaybarOutput(ev)

	if !strings.Contains(out.Text, "in 4") { // "in 44m" or "in 45m" depending on rounding
		t.Errorf("text missing countdown suffix: %q", out.Text)
	}
	if !isValidUTF8(out.Text) {
		t.Errorf("text contains invalid UTF-8: %q", out.Text)
	}
}

func TestEscapePangoMarkup(t *testing.T) {
	tests := map[string]string{
		"A & B":          "A &amp; B",
		"<script>":       "&lt;script&gt;",
		"already &amp;":  "already &amp;amp;", // we don't try to detect pre-escaped — that's by design
		"":               "",
		"plain":          "plain",
		"both < and > &": "both &lt; and &gt; &amp;",
	}
	for in, want := range tests {
		got := escapePangoMarkup(in)
		if want == "both &lt; and &gt; &amp;" {
			// Order of replacement matters: & first, then <, >.
			// "both < and > &" → "both < and > &amp;" → "both &lt; and &gt; &amp;"
			if got != want {
				t.Errorf("escapePangoMarkup(%q) = %q, want %q", in, got, want)
			}
			continue
		}
		if got != want {
			t.Errorf("escapePangoMarkup(%q) = %q, want %q", in, got, want)
		}
	}
}
