package paginator

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"torrentino/common/utils"
)

const (
	CB_ORDER_BY       = "#order_by#"
	CB_FILTER_BY      = "#filterby#"
	CB_ACTION         = "#action__#"
	CB_NEXT_PAGE      = "next_page"
	CB_PREV_PAGE      = "prev_page"
	CB_TOGGLE_FILTERS = "toggle_filters"
	CB_STUB           = "stub"
)

var Handlers map[string]string = make(map[string]string)

// ----------------------------------------
type Builder interface {
	Header() string
	Footer() string
	Line(i int) string
}

type Actor interface {
	Actions(i int) []string
	Execute(i int, action string) (unselect bool)
}

type Paginator struct {
	List

	Builder
	Actor

	bot     *bot.Bot
	ctx     context.Context
	message *models.Message
	update  *models.Update

	extControls  bool
	activePage   int
	itemsPerPage int
	selectedItem int

	prefix   string
	text     string
	keyboard models.InlineKeyboardMarkup
}

func New(
	ctx context.Context, b *bot.Bot, update *models.Update,
	prefix string, itemsPerPage int, builder Builder, actor Actor, evaluator Evaluator,
) *Paginator {
	p := &Paginator{
		ctx:          ctx,
		bot:          b,
		update:       update,
		itemsPerPage: itemsPerPage,
		prefix:       prefix,
		selectedItem: -1,
	}
	p.Builder = builder
	p.Actor = actor
	p.List.Evaluator = evaluator
	return p
}

// ----------"Builder" interface----------------
func (p *Paginator) Header() string {
	var fromIndex, toIndex = p.pageBounds()
	if fromIndex < toIndex {
		return "<b>results: " + strconv.Itoa(fromIndex+1) + "-" + strconv.Itoa(toIndex) + " of " + strconv.Itoa(p.Len()) + "</b>"
	} else {
		return "<b>the list is empty</b>"
	}
}

func (p *Paginator) Footer() string {
	return ""
}

func (p *Paginator) Line(item int) string {
	return ""
}

// ----------END "Builder" interface----------------

// ----------"Actor" interface----------------
func (p *Paginator) Actions(i int) []string {
	return nil
}

func (p *Paginator) Execute(i int, actionKey string) (unselectItem bool) {
	return true
}

// ----------END "Actor" interface----------------

func (p *Paginator) ReplyMessage(text string) {
	var chatId int64
	if p.message != nil {
		chatId = p.message.Chat.ID
	} else {
		chatId = p.update.Message.Chat.ID
	}

	_, err := p.bot.SendMessage(p.ctx, &bot.SendMessageParams{
		ChatID:      chatId,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: nil,
	})
	if err != nil {
		utils.LogError(err)
	}
}

func (p *Paginator) ReplyDocument(doc *models.InputFileUpload) {
	_, err := p.bot.SendDocument(p.ctx, &bot.SendDocumentParams{
		ChatID:      p.message.Chat.ID,
		Document:    doc,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: nil,
	})
	if err != nil {
		utils.LogError(err)
	}
}

func (p *Paginator) pageBounds() (int, int) {
	var maxItems int = p.Len()
	var fromIndex = p.activePage * p.itemsPerPage
	var toIndex = fromIndex + p.itemsPerPage
	if toIndex > maxItems {
		toIndex = maxItems
	}
	return fromIndex, toIndex
}

func (p *Paginator) buildText() string {

	var text string
	hr := "\n<b>⸻⸻⸻⸻⸻</b>\n"
	br := "\n\n"

	text = text + p.Builder.Header() + hr
	fromIndex, toIndex := p.pageBounds()
	for i := fromIndex; i < toIndex; i++ {
		text = text + "<b>" + strconv.Itoa(i+1) + ".</b> " +
			(func() string {
				if p.selectedItem == i {
					return "<u>" + p.Builder.Line(i) + "</u>"
				} else {
					return p.Builder.Line(i)
				}
			})()
		if i < toIndex-1 {
			text = text + br
		}
	}
	footer := p.Builder.Footer()
	if len(footer) > 0 {
		text = text + hr + "<b>" + footer + "</b>"
	}
	return text
}

func (p *Paginator) buildKeyboard() [][]models.InlineKeyboardButton {

	type buttonData struct {
		Text string
		Data string
	}
	sortChars := [3]string{"", "▼", "▲"}

	btoi := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}

	chooseButton := func(predicate bool, bdata [2]buttonData) models.InlineKeyboardButton {
		var i = btoi(!predicate)
		return models.InlineKeyboardButton{Text: bdata[i].Text, CallbackData: bdata[i].Data}
	}

	var keyboard [][]models.InlineKeyboardButton
	var row []models.InlineKeyboardButton

	var fromIndex, toIndex = p.pageBounds()
	for i := fromIndex; i < toIndex; i++ {
		btnCap := strconv.Itoa(i + 1)
		if i == p.selectedItem {
			btnCap = "(" + btnCap + ")"
		}
		row = append(row, models.InlineKeyboardButton{Text: btnCap, CallbackData: p.prefix + strconv.Itoa(i)})
	}
	if len(row) > 0 {
		keyboard = append(keyboard, row)
	}

	if p.extControls {
		row = []models.InlineKeyboardButton{}
		for _, attr := range p.sorting.attributes.Iter() {
			row = append(row, models.InlineKeyboardButton{
				Text:         attr.Alias + sortChars[int(attr.Order)],
				CallbackData: p.prefix + CB_ORDER_BY + attr.Attribute,
			})
		}
		if len(row) > 0 {
			keyboard = append(keyboard, row)
		}

		for attr, buttons := range p.filters.Iter() {
			row = []models.InlineKeyboardButton{}
			i := 0
			for button, enabled := range buttons.Iter() {
				row = append(row, models.InlineKeyboardButton{
					Text:         []string{"", "✓"}[btoi(enabled)] + button,
					CallbackData: p.prefix + CB_FILTER_BY + attr + "/" + button,
				})
				if (i+1)%4 == 0 { // 4 buttons max
					keyboard = append(keyboard, row)
					row = []models.InlineKeyboardButton{}
				}
				i++
			}
			if len(row) > 0 {
				keyboard = append(keyboard, row)
			}
		}
	}

	row = []models.InlineKeyboardButton{
		chooseButton(p.activePage > 0,
			[2]buttonData{{"⬅", p.prefix + CB_PREV_PAGE}, {"-", p.prefix + CB_STUB}}),
		chooseButton(p.extControls,
			[2]buttonData{{"🔺", p.prefix + CB_TOGGLE_FILTERS}, {"🔻", p.prefix + CB_TOGGLE_FILTERS}}),
		chooseButton(p.activePage < p.lastPage(),
			[2]buttonData{{"➡", p.prefix + CB_NEXT_PAGE}, {"-", p.prefix + CB_STUB}}),
	}
	keyboard = append(keyboard, row)

	if !p.extControls && (p.selectedItem >= fromIndex) && (p.selectedItem < toIndex) {
		row = []models.InlineKeyboardButton{}
		for i, action := range p.Actor.Actions(p.selectedItem) {
			row = append(row, models.InlineKeyboardButton{
				Text:         action,
				CallbackData: p.prefix + CB_ACTION + action,
			})
			if (i+1)%2 == 0 {
				keyboard = append(keyboard, row)
				row = []models.InlineKeyboardButton{}
			}
		}
		if len(row) > 0 {
			keyboard = append(keyboard, row)
		}
	}
	return keyboard
}

func (p *Paginator) Show() {
	var err error
	p.Filter()
	p.Sort()
	text := p.buildText()
	keyboard := p.buildKeyboard()

	if p.message == nil { // Show() first call?
		if callbackHandlerID, ok := Handlers[p.prefix]; ok {
			p.bot.UnregisterHandler(callbackHandlerID)
		}
		Handlers[p.prefix] = p.bot.RegisterHandler(bot.HandlerTypeCallbackQueryData, p.prefix, bot.MatchTypePrefix, p.callbackHandler)
		p.text = text
		p.keyboard.InlineKeyboard = keyboard
		p.message, err = p.bot.SendMessage(p.ctx, &bot.SendMessageParams{
			ChatID:      p.update.Message.Chat.ID,
			Text:        p.text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: p.keyboard,
		})
	} else {
		textChanged := text != p.text
		kbdChanged := !reflect.DeepEqual(keyboard, p.keyboard.InlineKeyboard)
		if textChanged {
			p.text = text
		}
		if kbdChanged {
			p.keyboard.InlineKeyboard = keyboard
		}
		if textChanged {
			_, err = p.bot.EditMessageText(p.ctx, &bot.EditMessageTextParams{
				ChatID:      p.message.Chat.ID,
				MessageID:   p.message.ID,
				Text:        p.text,
				ParseMode:   models.ParseModeHTML,
				ReplyMarkup: p.keyboard,
			})
		}
		if !textChanged && kbdChanged {
			_, err = p.bot.EditMessageReplyMarkup(p.ctx, &bot.EditMessageReplyMarkupParams{
				ChatID:      p.message.Chat.ID,
				MessageID:   p.message.ID,
				ReplyMarkup: p.keyboard,
			})
		}
	}
	if err != nil {
		utils.LogError(err)
	}
}

// lastPage is the index of the final page. The "next" button and its handler
// must agree on this value, otherwise a press can move past the last non-empty
// page and show an empty list.
func (p *Paginator) lastPage() int {
	if p.itemsPerPage <= 0 || p.Len() == 0 {
		return 0
	}
	return (p.Len() - 1) / p.itemsPerPage
}

// parseItemIndex reports whether the command is a plain decimal item index.
// Only ASCII digits are accepted: the keyboard generates these values itself,
// so anything else is stale or forged data that must not be interpreted.
func parseItemIndex(cmd string) (int, bool) {
	if cmd == "" {
		return 0, false
	}
	for i := 0; i < len(cmd); i++ {
		if cmd[i] < '0' || cmd[i] > '9' {
			return 0, false
		}
	}
	item, err := strconv.Atoi(cmd)
	if err != nil {
		return 0, false
	}
	return item, true
}

func (p *Paginator) callbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.CallbackQuery == nil {
		return
	}

	cmd := strings.TrimPrefix(update.CallbackQuery.Data, p.prefix)

	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            cmd,
		ShowAlert:       false,
	}); err != nil {
		utils.LogError(err)
	}

	p.dispatch(cmd)
	p.Show()
}

// dispatch applies a single button press.
//
// Callback data comes back from Telegram and can be empty, malformed or stale
// (a message from an older build, a list that shrank since the keyboard was
// drawn), so every branch validates its input. Indexing into the command string
// or the item list without checking used to panic, and a panic in a handler
// goroutine takes down the whole bot.
func (p *Paginator) dispatch(cmd string) {

	if item, ok := parseItemIndex(cmd); ok {
		if item < p.Len() {
			p.selectedItem = item
			p.extControls = false
		}
		return
	}

	switch cmd {
	case CB_NEXT_PAGE:
		if p.activePage < p.lastPage() {
			p.activePage++
		}
		return

	case CB_PREV_PAGE:
		if p.activePage > 0 {
			p.activePage--
		}
		return

	case CB_TOGGLE_FILTERS:
		p.extControls = !p.extControls
		return

	case CB_STUB, "": // no-op, but must not fall through to the command parsing
		return
	}

	// Commands carrying a payload: <command><payload>.
	for _, command := range []string{CB_ORDER_BY, CB_FILTER_BY, CB_ACTION} {
		payload, ok := strings.CutPrefix(cmd, command)
		if !ok {
			continue
		}
		switch command {
		case CB_ORDER_BY:
			p.ToggleSorting(payload)
			p.selectedItem = -1
		case CB_FILTER_BY:
			attribute, value, ok := strings.Cut(payload, "/")
			if !ok {
				return
			}
			p.ToggleFilter(attribute, value)
			p.activePage = 0
			p.selectedItem = -1
		case CB_ACTION:
			// The selection may point outside the list if it was reloaded or
			// filtered after the keyboard was drawn.
			if (p.selectedItem >= 0) && (p.selectedItem < p.Len()) {
				if p.Actor.Execute(p.selectedItem, payload) {
					p.selectedItem = -1
				}
			}
		}
		return
	}
}
