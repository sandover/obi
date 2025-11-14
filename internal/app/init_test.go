package app

import "testing"

func TestSanitizeKey(t *testing.T) {
	if got := sanitizeKey("abc-def"); got != "abc_def" {
		t.Fatalf("unexpected key: %s", got)
	}
}

func TestNormalizeSingleLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "trims whitespace", in: "  hello  ", want: "hello"},
		{name: "collapses newlines", in: "line one\nline two", want: "line one line two"},
		{name: "keeps apostrophes", in: "it's good", want: "it's good"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeSingleLine(tc.in); got != tc.want {
				t.Fatalf("normalizeSingleLine(%q)=%q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestFilterActiveEpicsIncludesEligibleOpen(t *testing.T) {
	t.Parallel()
	epics := []bdEpic{
		{
			Epic: struct {
				ID          string "json:\"id\""
				Title       string "json:\"title\""
				Description string "json:\"description\""
				Status      string "json:\"status\""
			}{
				ID:     "automatic-octo-barnacle-eh2",
				Title:  "Obi epic",
				Status: "open",
			},
			EligibleForClose: true,
			TotalChildren:    10,
			ClosedChildren:   10,
		},
	}
	got := filterActiveEpics(epics)
	if len(got) != 1 {
		t.Fatalf("expected open epic even if eligible_for_close; got %d", len(got))
	}
}
