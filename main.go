package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"slices"

	"torrentino/common"
	"torrentino/handlers/downloads"
	"torrentino/handlers/search"
	"torrentino/handlers/torrserver"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {
	log.Println("[Torrentino]: startup")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	opts := []bot.Option{
		bot.WithSkipGetMe(),
		bot.WithMiddlewares(func(next bot.HandlerFunc) bot.HandlerFunc {
			return func(ctx context.Context, b *bot.Bot, update *models.Update) {
				if (update != nil) && (update.Message != nil) {
					if slices.Index(common.Settings.Users_list, update.Message.From.ID) == -1 {
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

	b, err := bot.New(common.Settings.Telegram_api_token, opts...)
	if nil != err {
		log.Fatal(err)
	}

	b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "/downloads", Description: "Downloads"},
			{Command: "/torrserver", Description: "Torrserver"},
		},
	})

	b.Start(ctx)
}
