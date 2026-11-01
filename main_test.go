package main

import (
	"bytes"
	"log"
	"regexp"
	"strings"
	"testing"

	"torrentino/common/logging"
)

func TestRedactingWriterRemovesBotToken(t *testing.T) {
	var out bytes.Buffer
	writer := redactingWriter{
		out:     &out,
		pattern: regexp.MustCompile(`bot[0-9]+:[A-Za-z0-9_-]+`),
	}
	line := []byte(`[TGBOT] Post "https://api.telegram.org/bot123456:AAH-secret_token/getUpdates": refused`)
	if _, err := writer.Write(line); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "123456:AAH-secret_token") {
		t.Errorf("token leaked: %q", out.String())
	}
	if !strings.Contains(out.String(), "bot<redacted>/getUpdates") {
		t.Errorf("redaction marker missing: %q", out.String())
	}
}

func TestRedactingWriterLeavesOtherLinesAlone(t *testing.T) {
	var out bytes.Buffer
	writer := redactingWriter{out: &out, pattern: regexp.MustCompile(`bot[0-9]+:[A-Za-z0-9_-]+`)}
	if _, err := writer.Write([]byte("plain web log line\n")); err != nil {
		t.Fatal(err)
	}
	if out.String() != "plain web log line\n" {
		t.Errorf("line changed: %q", out.String())
	}
}

func TestTimestampWriterUsesDashDate(t *testing.T) {
	var out bytes.Buffer
	writer := timestampWriter{out: &out}
	if _, err := writer.Write([]byte("startup\n")); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} startup\n$`).MatchString(out.String()) {
		t.Errorf("unexpected timestamp layout: %q", out.String())
	}
}

func TestLogLineHasTimestampLevelAndTag(t *testing.T) {
	var out bytes.Buffer
	oldWriter, oldFlags := log.Writer(), log.Flags()
	log.SetFlags(0)
	log.SetOutput(redactingWriter{
		out:     timestampWriter{out: &out},
		pattern: regexp.MustCompile(`bot[0-9]+:[A-Za-z0-9_-]+`),
	})
	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
	})

	logging.Info(logging.TagWeb, "hello %s", "world")
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \[INF\] \[WEB\] hello world\n$`).
		MatchString(out.String()) {
		t.Errorf("unexpected log line: %q", out.String())
	}
}
