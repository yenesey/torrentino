package web

import "testing"

func TestParseCommand(t *testing.T) {
	valid := map[string]webCommand{
		"select:0":               {verb: "select", index: 0},
		"select:42":              {verb: "select", index: 42},
		"deselect":               {verb: "deselect"},
		"page:next":              {verb: "page", action: "next"},
		"page:prev":              {verb: "page", action: "prev"},
		"controls:toggle":        {verb: "controls"},
		"sort:Size":              {verb: "sort", attribute: "Size"},
		"filter:TrackerId/rutor": {verb: "filter", attribute: "TrackerId", value: "rutor"},
		"filter:TrackerId/sub/g": {verb: "filter", attribute: "TrackerId", value: "sub/g"},
		"filter:clear":           {verb: "filter-clear"},
		"action:download":        {verb: "action", action: "download"},
		"action:download:movie":  {verb: "action", action: "download:movie"},
	}
	for input, want := range valid {
		got, err := parseCommand(input)
		if err != nil {
			t.Errorf("parseCommand(%q) error = %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("parseCommand(%q) = %+v, want %+v", input, got, want)
		}
	}
}

func TestParseCommandRejectsGarbage(t *testing.T) {
	inputs := []string{
		"", " ", "not-a-command", "select:", "select:-1", "select:+1",
		"select:12abc", "select:99999999999999999999", "sort:", "filter:", "filter:x",
		"filter:/v", "filter:a/", "action:", "page:", "%", "стоп",
	}
	for _, input := range inputs {
		if got, err := parseCommand(input); err == nil {
			t.Errorf("parseCommand(%q) = %+v, want an error", input, got)
		}
	}
}

func TestParseIndex(t *testing.T) {
	for _, input := range []string{"0", "1", "42"} {
		if _, ok := parseIndex(input); !ok {
			t.Errorf("parseIndex(%q) failed", input)
		}
	}
	for _, input := range []string{"", "-1", "+1", "1a", "99999999999999999999", "١"} {
		if _, ok := parseIndex(input); ok {
			t.Errorf("parseIndex(%q) succeeded", input)
		}
	}
}

func FuzzParseCommand(f *testing.F) {
	for _, seed := range []string{"select:1", "filter:a/b", "sort:Size", "action:delete", "", "filter:a"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = parseCommand(input) // must never panic
	})
}
