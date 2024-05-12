package main

import (
	"context"
	"os"
	"os/signal"
	"log"

	"torrentino/common"

	"torrentino/handlers/torrent_find"
	"torrentino/handlers/downloads"
	"torrentino/handlers/torrserver"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// https://github.com/go-telegram/bot/blob/main/examples/handler_match_func/main.go
	opts := []bot.Option{
		bot.WithSkipGetMe(),
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
			// {Command: "/find", Description: "find torrent"},
			{Command: "/downloads", Description: "list downloads"},
			{Command: "/torrserver", Description: "list torrserver"},
		},
	})

	b.Start(ctx)

}
