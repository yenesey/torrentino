package logging

import (
	"bytes"
	"log"
	"testing"
)

func capture(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	oldWriter, oldFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})
	fn()
	return buf.String()
}

func TestLevelsAndTags(t *testing.T) {
	got := capture(t, func() {
		Info(TagWeb, "hello %s", "world")
		Warn(TagTGBot, "careful")
		Error(TagNone, "boom")
	})
	want := "[INF] [WEB] hello world\n[WRN] [TGBOT] careful\n[ERR] boom\n"
	if got != want {
		t.Errorf("unexpected log output:\n got: %q\nwant: %q", got, want)
	}
}

func TestMultilineKeepsPrefixOnEveryLine(t *testing.T) {
	got := capture(t, func() {
		Error(TagNone, "first line\nsecond line")
	})
	want := "[ERR] first line\n[ERR] second line\n"
	if got != want {
		t.Errorf("unexpected multiline output:\n got: %q\nwant: %q", got, want)
	}
}

func TestMessageWithoutPlaceholders(t *testing.T) {
	got := capture(t, func() {
		Warn(TagWeb, "no placeholders here")
	})
	want := "[WRN] [WEB] no placeholders here\n"
	if got != want {
		t.Errorf("unexpected warning output:\n got: %q\nwant: %q", got, want)
	}
}
