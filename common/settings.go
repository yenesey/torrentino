package common

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"torrentino/common/password"
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

// WebUser is one account allowed to log in to the web interface.
type WebUser struct {
	Name         string `json:"name"`
	PasswordHash string `json:"password-hash"`
}

type WebSession struct {
	IdleTimeout string `json:"idle-timeout"`
	MaxAge      string `json:"max-age"`
}

type WebTLS struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// WebStruct configures the optional HTTP interface. When Enabled is false the
// bot behaves exactly as before and none of these fields are validated.
type WebStruct struct {
	Enabled        bool       `json:"enabled"`
	Listen         string     `json:"listen"`
	BasePath       string     `json:"base-path"`
	PageSize       int        `json:"page-size"`
	Session        WebSession `json:"session"`
	SecureCookies  bool       `json:"secure-cookies"`
	TLS            WebTLS     `json:"tls"`
	TrustedProxies []string   `json:"trusted-proxies"`
	Users          []WebUser  `json:"users"`
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

	Web WebStruct `json:"web"`
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
// WebListen returns the configured listen address or the loopback default.
func (s *SettingsStruct) WebListen() string {
	if strings.TrimSpace(s.Web.Listen) == "" {
		return "127.0.0.1:8088"
	}
	return strings.TrimSpace(s.Web.Listen)
}

// WebBasePath returns the normalized external prefix; "/" means no prefix.
func (s *SettingsStruct) WebBasePath() string {
	if strings.TrimSpace(s.Web.BasePath) == "" {
		return "/"
	}
	value, err := NormalizeBasePath(s.Web.BasePath)
	if err != nil {
		return "/"
	}
	return value
}

// WebPageSize returns the number of items per page, defaulting to 20.
func (s *SettingsStruct) WebPageSize() int {
	if s.Web.PageSize == 0 {
		return 20
	}
	return s.Web.PageSize
}

// WebSessionIdle returns the idle timeout, defaulting to 12h.
func (s *SettingsStruct) WebSessionIdle() time.Duration {
	return durationOr(s.Web.Session.IdleTimeout, 12*time.Hour)
}

// WebSessionMaxAge returns the absolute session lifetime, defaulting to 168h.
func (s *SettingsStruct) WebSessionMaxAge() time.Duration {
	return durationOr(s.Web.Session.MaxAge, 168*time.Hour)
}

func durationOr(value string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

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

	// Decode into a fresh value: json.Unmarshal into the global struct would
	// keep fields that are absent from the new file (for example a timeout
	// left over from a previous load in tests), and a failed validation would
	// leave the process with a half-applied config.
	var loaded SettingsStruct
	if err = json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("cannot parse config file %q: %w", displayPath, err)
	}

	if err = loaded.validate(); err != nil {
		return fmt.Errorf("invalid config file %q:\n        %s", displayPath, err)
	}

	Settings = loaded
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

	if s.Web.Enabled {
		problems = append(problems, s.Web.problems()...)
	}

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
	if s.Web.Enabled {
		s.Web.warn()
	}
}

// problems validates every enabled web setting at once, in the same style as
// the rest of the config: one restart shows all mistakes.
func (w *WebStruct) problems() (problems []string) {
	listen := strings.TrimSpace(w.Listen)
	if listen == "" {
		listen = "127.0.0.1:8088"
	}
	_, portText, err := net.SplitHostPort(listen)
	if err != nil {
		problems = append(problems, fmt.Sprintf(`"web.listen" is not host:port: %q`, listen))
	} else {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			problems = append(problems, fmt.Sprintf(`"web.listen" must use a port in 1..65535, got %q`, portText))
		}
	}

	if _, err := NormalizeBasePath(w.BasePath); err != nil {
		problems = append(problems, fmt.Sprintf(`"web.base-path" is invalid: %s`, err))
	}

	if w.PageSize != 0 && (w.PageSize < 5 || w.PageSize > 100) {
		problems = append(problems, fmt.Sprintf(`"web.page-size" must be in 5..100, got %d`, w.PageSize))
	}

	idle := w.Session.IdleTimeout
	maxAge := w.Session.MaxAge
	idleDuration, err := parsePositiveDuration(idle)
	if err != nil {
		problems = append(problems, fmt.Sprintf(`"web.session.idle-timeout" is invalid: %s`, err))
	}
	maxAgeDuration, err := parsePositiveDuration(maxAge)
	if err != nil {
		problems = append(problems, fmt.Sprintf(`"web.session.max-age" is invalid: %s`, err))
	}
	if idleDuration > 0 && maxAgeDuration > 0 && idleDuration >= maxAgeDuration {
		problems = append(problems, `"web.session.idle-timeout" must be less than "web.session.max-age"`)
	}

	if (strings.TrimSpace(w.TLS.Cert) == "") != (strings.TrimSpace(w.TLS.Key) == "") {
		problems = append(problems, `"web.tls.cert" and "web.tls.key" must be set together or both empty`)
	}

	for _, proxy := range w.TrustedProxies {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(proxy)); err != nil {
			problems = append(problems, fmt.Sprintf(`"web.trusted-proxies" contains an invalid CIDR: %q`, proxy))
		}
	}

	if len(w.Users) == 0 {
		problems = append(problems, `"web.users" must contain at least one account when web.enabled is true`)
	}
	seen := make(map[string]bool)
	for i := range w.Users {
		name := strings.TrimSpace(w.Users[i].Name)
		if name == "" {
			problems = append(problems, fmt.Sprintf(`"web.users[%d].name" is empty`, i))
		} else if len([]rune(name)) > 64 {
			problems = append(problems, fmt.Sprintf(`"web.users[%d].name" is longer than 64 characters`, i))
		} else if seen[strings.ToLower(name)] {
			problems = append(problems, fmt.Sprintf(`"web.users[%d].name" duplicates %q (names are case-insensitive)`, i, name))
		}
		seen[strings.ToLower(name)] = true
		if err := password.Validate(w.Users[i].PasswordHash); err != nil {
			problems = append(problems, fmt.Sprintf(`"web.users[%d].password-hash" is invalid: %s`, i, err))
		}
	}

	return problems
}

func parsePositiveDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf(`not a duration: %q, expected a value like "12h"`, value)
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive, got %q", value)
	}
	return d, nil
}

func (w *WebStruct) warn() {
	listen := strings.TrimSpace(w.Listen)
	if listen == "" {
		listen = "127.0.0.1:8088"
	}
	host, _, err := net.SplitHostPort(listen)
	if err == nil && w.TLS.Cert == "" {
		loopback := host == "" || host == "localhost"
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			loopback = true
		}
		if !loopback {
			log.Print(`[Torrentino]: warning: "web.listen" is not loopback and TLS is not configured; ` +
				`the web interface will be reachable over plain HTTP`)
		}
	}
	if w.BasePath != "" && w.BasePath != "/" && len(w.TrustedProxies) == 0 {
		log.Print(`[Torrentino]: warning: "web.base-path" is set but "web.trusted-proxies" is empty; ` +
			`X-Forwarded-* headers will be ignored`)
	}
}
