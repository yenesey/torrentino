package utils

import (
	"testing"

	"torrentino/common/logging"
)

func TestLogTag(t *testing.T) {
	cases := map[string]logging.Tag{
		"web":              logging.TagWeb,
		"web/handlers":     logging.TagWeb,
		"telegram":         logging.TagTGBot,
		"telegram/search":  logging.TagTGBot,
		"common/paginator": logging.TagTGBot,
		"core":             logging.TagNone,
		"common/list":      logging.TagNone,
		"main":             logging.TagNone,
	}
	for pkg, want := range cases {
		if got := logTag(pkg); got != want {
			t.Errorf("logTag(%q) = %q, want %q", pkg, got, want)
		}
	}
}
