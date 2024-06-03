package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"torrentino/common"

	"torrentino/handlers/downloads"
	"torrentino/handlers/torrent_find"
	"torrentino/handlers/torrserver"

	"slices"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// https://github.com/go-telegram/bot/blob/main/examples/handler_match_func/main.go
	opts := []bot.Option{
		bot.WithSkipGetMe(),
		bot.WithMiddlewares(securityMiddleware),
		bot.WithDefaultHandler(torrent_find.Handler),
		bot.WithMessageTextHandler("/downloads", bot.MatchTypeExact, downloads.Handler),
		bot.WithMessageTextHandler("/torrserver", bot.MatchTypeExact, torrserver.Handler),
	}

	b, err := bot.New(common.Settings.Telegram_api_token, opts...)
	if nil != err {
		log.Fatal(err)
	}

	b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "/downloads", Description: "list downloads"},
			{Command: "/torrserver", Description: "list torrserver"},
		},
	})

	b.Start(ctx)
}

func securityMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if (update != nil) && (update.Message != nil) {
			if slices.Index(common.Settings.Users_list, update.Message.From.ID) == -1 {
				log.Printf("%d (%s) say: %s", update.Message.From.ID, update.Message.From.Username, update.Message.Text)
				return
			}
		}
		next(ctx, b, update)
	}
}
