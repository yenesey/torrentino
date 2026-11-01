// Package logging renders application log records as
//
//	YYYY-MM-DD HH:MM:SS [LEVEL] [SUBSYSTEM] message
//
// The timestamp is added by the writer installed in main; this package only
// supplies the level and the interface tag. Records that belong to both
// interfaces (configuration, core) intentionally have no subsystem tag.
package logging

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// Tag identifies the interface that produced a record.
type Tag string

const (
	TagNone  Tag = ""
	TagWeb   Tag = "WEB"
	TagTGBot Tag = "TGBOT"
)

type level string

const (
	levelError level = "ERR"
	levelWarn  level = "WRN"
	levelInfo  level = "INF"
)

// Info logs an informational record.
func Info(tag Tag, format string, args ...any) {
	write(levelInfo, tag, format, args...)
}

// Warn logs a warning: the program keeps running, but the situation most
// likely needs attention.
func Warn(tag Tag, format string, args ...any) {
	write(levelWarn, tag, format, args...)
}

// Error logs an error that was handled (or returned to the caller).
func Error(tag Tag, format string, args ...any) {
	write(levelError, tag, format, args...)
}

// Fatalf logs an error and exits with status 1, like log.Fatalf.
func Fatalf(tag Tag, format string, args ...any) {
	write(levelError, tag, format, args...)
	os.Exit(1)
}

func write(lvl level, tag Tag, format string, args ...any) {
	prefix := "[" + string(lvl) + "] "
	if tag != TagNone {
		prefix += "[" + string(tag) + "] "
	}

	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	message = strings.TrimSuffix(message, "\n")
	// Log every physical line separately: the timestamp writer in main adds
	// the date to each log call, so multiline messages keep the full
	// "date [LEVEL] [TAG]" prefix on every line.
	for _, line := range strings.Split(message, "\n") {
		log.Print(prefix + line)
	}
}
