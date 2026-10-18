package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"torrentino/common"
	"torrentino/common/password"
	"torrentino/core"
	"torrentino/telegram/downloads"
	"torrentino/telegram/search"
	"torrentino/telegram/torrserver"
	"torrentino/web"
)

func main() {
	hashPassword := flag.Bool("hash-password", false, "read a password from stdin and print a web.users password-hash")
	flag.Parse()
	if *hashPassword {
		if err := printPasswordHash(); err != nil {
			log.Fatalf("[Torrentino]: %s", err)
		}
		return
	}

	common.MustLoadSettings()

	// The Telegram client library logs the request URL on network errors, and
	// that URL contains the bot token. Wrap the standard logger so no token
	// reaches /var/log/torrentino.log.
	log.SetOutput(redactingWriter{
		out:     os.Stderr,
		pattern: regexp.MustCompile(`bot[0-9]+:[A-Za-z0-9_-]+`),
	})

	log.Println("[Torrentino]: startup")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var webServer *web.Server
	if common.Settings.Web.Enabled {
		var err error
		webServer, err = web.NewServer(core.Default)
		if err != nil {
			log.Fatalf("[Torrentino]: web startup failed: %s", err)
		}
		// The web server starts before Telegram: even if the bot token or the
		// Telegram API is broken, the UI is already reachable.
		if err := webServer.Start(); err != nil {
			log.Fatalf("[Torrentino]: web startup failed: %s", err)
		}
	}

	opts := []bot.Option{
		bot.WithSkipGetMe(),
		bot.WithMiddlewares(func(next bot.HandlerFunc) bot.HandlerFunc {
			return func(ctx context.Context, b *bot.Bot, update *models.Update) {
				if (update != nil) && (update.Message != nil) {
					if slices.Index(common.Settings.UsersList, update.Message.From.ID) == -1 {
						log.Printf("%d (%s) say: %s", update.Message.From.ID, update.Message.From.Username, update.Message.Text)
						return
					}
				}
				next(ctx, b, update)
			}
		}),
		bot.WithDefaultHandler(search.Handler),
		bot.WithMessageTextHandler("/downloads", bot.MatchTypeExact, downloads.Handler),
		bot.WithMessageTextHandler("/torrserver", bot.MatchTypeExact, torrserver.Handler),
	}

	b, botErr := bot.New(common.Settings.TelegramAPIToken, opts...)
	if botErr != nil {
		if !common.Settings.Web.Enabled {
			log.Fatal(botErr)
		}
		log.Printf("[Torrentino]: Telegram is unavailable (%v); running web-only", botErr)
		<-ctx.Done()
	} else {
		b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
			Commands: []models.BotCommand{
				{Command: "/downloads", Description: "Downloads"},
				{Command: "/torrserver", Description: "Torrserver"},
			},
		})
		botDone := make(chan struct{})
		go func() {
			defer close(botDone)
			b.Start(ctx)
		}()
		if common.Settings.Web.Enabled {
			select {
			case <-ctx.Done():
			case <-botDone:
				if ctx.Err() == nil {
					log.Print("[Torrentino]: Telegram bot stopped; keeping the web interface running")
					<-ctx.Done()
				}
			}
		} else {
			<-botDone
		}
	}

	if webServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := webServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("[Torrentino]: web shutdown: %v", err)
		}
	}
}

// printPasswordHash implements -hash-password. The password is read from
// stdin, never from argv, so it does not end up in shell history or ps.
func printPasswordHash() error {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	value := strings.TrimRight(string(data), "\r\n")
	if value == "" {
		return fmt.Errorf("empty password")
	}
	hash, err := password.Hash(value, password.DefaultIterations)
	if err != nil {
		return err
	}
	fmt.Println(hash)
	return nil
}

// redactingWriter removes bot tokens from log lines before they reach stderr
// (which systemd appends to /var/log/torrentino.log).
type redactingWriter struct {
	out     io.Writer
	pattern *regexp.Regexp
}

func (w redactingWriter) Write(p []byte) (int, error) {
	redacted := p
	if w.pattern != nil {
		redacted = w.pattern.ReplaceAll(p, []byte("bot<redacted>"))
	}
	if _, err := w.out.Write(redacted); err != nil {
		return 0, err
	}
	return len(p), nil
}
