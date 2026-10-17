package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const settingsFileName = "settings.json"

// DefaultJackettTimeout is used when "jackett.timeout" is not set. Jackett
// queries its indexers one by one, so a search across several trackers can
// take much longer than a single HTTP request.
const DefaultJackettTimeout = 60 * time.Second

type hostPort struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type SettingsStruct struct {
	Jackett struct {
		hostPort `json:",inline"`
		APIKey   string   `json:"api-key"`
		Indexers []string `json:"indexers"`
		// Timeout is a Go duration string ("60s", "1m30s"). Empty means
		// DefaultJackettTimeout.
		Timeout string `json:"timeout"`
	} `json:"jackett"`

	Transmission hostPort `json:",inline"`
	Torrserver   hostPort `json:",inline"`

	TelegramAPIToken string  `json:"telegram-api-token"`
	UsersList        []int64 `json:"users-list"`

	Path struct {
		Default string `json:"default"`
		Movie   string `json:"movie"`
		Series  string `json:"series"`
	} `json:"path"`
}

var Settings SettingsStruct

// LoadSettings reads and validates the config from the default location.
//
// It is deliberately NOT called from init(): a package that kills the process
// while being imported cannot be tested, and test binaries run with the working
// directory set to their own package, where settings.json does not exist.
func LoadSettings() error {
	return loadSettings(settingsFileName)
}

// LoadSettingsFrom is LoadSettings for an arbitrary path. Tests use it to feed
// fixtures without having to touch the working directory.
func LoadSettingsFrom(fileName string) error {
	return loadSettings(fileName)
}

// MustLoadSettings is LoadSettings for the bot entry point, where a missing or
// broken config is not recoverable: report the problem and terminate.
func MustLoadSettings() {
	if err := LoadSettings(); err != nil {
		log.Fatalf("[Torrentino]: startup failed: %s", err)
	}
}

// JackettTimeout is the deadline for a Jackett API request. It is parsed on
// every call rather than cached at startup, because the config is loaded in
// main(); validate() has already rejected unparsable values.
func (s *SettingsStruct) JackettTimeout() time.Duration {
	value := strings.TrimSpace(s.Jackett.Timeout)
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return DefaultJackettTimeout
	}
	return d
}

func loadSettings(fileName string) error {
	// The path is relative to the working directory, so report it resolved to
	// make a "wrong cwd" mistake obvious.
	displayPath := fileName
	if absPath, err := filepath.Abs(fileName); err == nil {
		displayPath = absPath
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf(
				"config file %q not found (current directory: %s)\n"+
					"        the bot must be started from the directory that contains %s (see readme.md); "+
					"without it all service endpoints are built empty and requests go nowhere",
				displayPath, currentDir(), settingsFileName,
			)
		}
		return fmt.Errorf("cannot read config file %q: %w", displayPath, err)
	}

	if err = json.Unmarshal(data, &Settings); err != nil {
		return fmt.Errorf("cannot parse config file %q: %w", displayPath, err)
	}

	if err = Settings.validate(); err != nil {
		return fmt.Errorf("invalid config file %q:\n        %s", displayPath, err)
	}

	Settings.warn()
	return nil
}

func currentDir() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "unknown"
}

// problems collects every issue at once, so a misconfigured file can be fixed
// in a single pass instead of one error per restart.
func (hp hostPort) problems(field string) (problems []string) {
	if strings.TrimSpace(hp.Host) == "" {
		problems = append(problems, fmt.Sprintf(
			"%q is empty, expected a host name or an IP address", field+".host"))
	}
	if hp.Port < 1 || hp.Port > 65535 {
		problems = append(problems, fmt.Sprintf(
			"%q must be a port in 1..65535, got %d", field+".port", hp.Port))
	}
	return problems
}

func (s *SettingsStruct) validate() error {
	var problems []string

	if strings.TrimSpace(s.TelegramAPIToken) == "" {
		problems = append(problems, `"telegram-api-token" is empty, obtain one from @BotFather`)
	}

	problems = append(problems, s.Jackett.problems("jackett")...)
	if strings.TrimSpace(s.Jackett.APIKey) == "" {
		problems = append(problems, `"jackett.api-key" is empty, copy it from the Jackett dashboard`)
	}
	if value := strings.TrimSpace(s.Jackett.Timeout); value != "" {
		if d, err := time.ParseDuration(value); err != nil {
			problems = append(problems, fmt.Sprintf(
				`"jackett.timeout" is not a duration: %q, expected a value like "60s" or "1m30s"`, value))
		} else if d <= 0 {
			problems = append(problems, fmt.Sprintf(
				`"jackett.timeout" must be positive, got %q`, value))
		}
	}

	problems = append(problems, s.Transmission.problems("transmission")...)
	problems = append(problems, s.Torrserver.problems("torrserver")...)

	if len(problems) == 0 {
		return nil
	}
	return errors.New("missing or invalid fields:\n        - " + strings.Join(problems, "\n        - "))
}

// warn reports settings that are usable but most likely unintended.
func (s *SettingsStruct) warn() {
	if len(s.UsersList) == 0 {
		log.Print(`[Torrentino]: warning: "users-list" is empty, every incoming message will be rejected; ` +
			`write to the bot and your user id will appear in the log`)
	}
	if s.Path.Default == "" || s.Path.Movie == "" || s.Path.Series == "" {
		log.Print(`[Torrentino]: warning: "path.default/movie/series" are not fully set, ` +
			`downloads will be placed into the Transmission default directory`)
	}
}
