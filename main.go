package main

import (
	"context"
	"os"
	"os/signal"

	//"torrentino/api"
	"torrentino/common"
	// "torrentino/handlers"
	"torrentino/handlers/torrent_find"
	"torrentino/handlers/torrent_list"

	// "torrentino/api/torrserver"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// https://github.com/go-telegram/bot/blob/main/examples/handler_match_func/main.go
	opts := []bot.Option{
		bot.WithDefaultHandler(torrent_find.Handler),
		//bot.WithSkipGetMe(),
		// bot.WithCallbackQueryDataHandler("", bot.MatchTypePrefix, handlers.DefaultHandler),
		//bot.WithMessageTextHandler("/find", bot.MatchTypeExact, torrent_find.FindHandler),
		bot.WithMessageTextHandler("/list", bot.MatchTypeExact, torrent_list.Handler),
	}

	b, err := bot.New(common.Settings.Telegram_api_token, opts...)
	if nil != err {
		// panics for the sake of simplicity.
		// you should handle this error properly in your code.
		panic(err)
	}

	b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			// {Command: "/find", Description: "find torrent"},
			{Command: "/list", Description: "list torrents"},
		},
	})

	b.Start(ctx)

}

/*
func callbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	// answering callback query first to let Telegram know that we received the callback query,
	// and we're handling it. Otherwise, Telegram might retry sending the update repetitively
	// as it thinks the callback query doesn't reach to our application. learn more by
	// reading the footnote of the https://core.telegram.org/bots/api#callbackquery type.
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.CallbackQuery.Message.Chat.ID,
		Text:   "You selected the button: " + update.CallbackQuery.Data,
	})
}

func helloHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      "Hello, *" + bot.EscapeMarkdown(update.Message.From.FirstName) + "*",
		ParseMode: models.ParseModeMarkdown,
	})
}



*/
