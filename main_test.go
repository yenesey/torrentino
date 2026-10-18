package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
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
