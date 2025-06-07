package paginator

import (
	"context"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

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

var callbackHandler map[string]string = make(map[string]string)

// ----------------------------------------
type Stringer interface {
	Header() string
	Footer() string
	ItemString(i int) string
	StringValueByName(item any, attributeName string) string
	Actions(i int) []string
}

type Executor interface {
	Execute(i int, actionKey string) (unselectItem bool)
	Reload() error
}

type Comparator interface {
	Compare(i int, j int, attributeName string) bool
}

type Paginator struct {
	list  []any
	index []int

	Sorting   SortingState
	Filtering FilteringState

	Ctx     context.Context
	Bot     *bot.Bot
	Message *models.Message

	extControlsVisible bool
	activePage         int
	itemsPerPage       int
	selectedItem       int

	prefix   string
	text     string
	keyboard models.InlineKeyboardMarkup
}

func New(prefix string, itemsPerPage int) *Paginator {
	return &Paginator{
		itemsPerPage: itemsPerPage,
		prefix:       prefix,
		selectedItem: -1,
	}
}

func (p *Paginator) Alloc(l int) {
	p.list = make([]any, 0, l)
	p.index = make([]int, 0, l)
}

func (p *Paginator) Append(item any) {
	p.list = append(p.list, item)
	p.index = append(p.index, len(p.list)-1)

	for i := range p.Filtering.attributes {
		attr := &p.Filtering.attributes[i]
		value := Stringer(p).StringValueByName(item, attr.AttributeName)
		if _, ok := attr.State[value]; !ok {
			attr.State[value] = false
			attr.Values = append(attr.Values, value)
		}
	}
}

func (p *Paginator) Delete(i int) {
	idx := p.index[i]
	p.list = slices.Delete(p.list, idx, idx+1) // p.list = append(p.list[:idx], p.list[idx+1:]...)
	p.Filter()                                 // <-- just for rebuild the indexes
}

func (p *Paginator) Item(i int) any {
	return p.list[p.index[i]]
}

func (p *Paginator) ReplyMessage(text string) {
	_, err := p.Bot.SendMessage(p.Ctx, &bot.SendMessageParams{
		ChatID:      p.Message.Chat.ID,
		Text:        text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: nil,
	})
	if err != nil {
		utils.LogError(err)
	}
}

func (p *Paginator) ReplyDocument(doc *models.InputFileUpload) {
	_, err := p.Bot.SendDocument(p.Ctx, &bot.SendDocumentParams{
		ChatID:      p.Message.Chat.ID,
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

// ----------part of VirtualMethods interface----------------
func (p *Paginator) Header() string {
	var fromIndex, toIndex = p.pageBounds()
	if fromIndex < toIndex {
		return "<b>results: " + strconv.Itoa(fromIndex+1) + "-" + strconv.Itoa(toIndex) + " of " + strconv.Itoa(p.Len()) + "</b>"
	} else {
		return "<b>the list is empty</b>"
	}
}

func (p *Paginator) ItemString(item int) string {
	return ""
}

func (p *Paginator) Footer() string {
	return ""
}

func (p *Paginator) StringValueByName(item any, attributeName string) string {
	return ""
}

func (p *Paginator) Compare(i int, j int, attributeName string) bool {
	return false
}

func (p *Paginator) Actions(i int) []string {
	return nil
}

func (p *Paginator) Execute(i int, actionKey string) (unselectItem bool) {
	return true
}

func (p *Paginator) Reload() error {
	p.Filter()
	p.Sort()
	return nil
}

// ----------------------------------------
func (p *Paginator) buildText() string {

	var text string
	hr := "\n<b>⸻⸻⸻⸻⸻</b>\n"
	br := "\n\n"

	text = text + Stringer(p).Header() + hr
	fromIndex, toIndex := p.pageBounds()
	for i := fromIndex; i < toIndex; i++ {
		text = text + "<b>" + strconv.Itoa(i+1) + ".</b> " +
			(func() string {
				if p.selectedItem == i {
					return "<u>" + Stringer(p).ItemString(i) + "</u>"
				} else {
					return Stringer(p).ItemString(i)
				}
			})()
		if i < toIndex-1 {
			text = text + br
		}
	}
	footer := Stringer(p).Footer()
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

	if p.extControlsVisible {
		row = []models.InlineKeyboardButton{}
		for _, v := range p.Sorting.headers {
			row = append(row, models.InlineKeyboardButton{
				Text:         v.ButtonText + sortChars[int(v.Order)],
				CallbackData: p.prefix + CB_ORDER_BY + v.AttributeName,
			})
		}
		if len(row) > 0 {
			keyboard = append(keyboard, row)
		}

		for _, attr := range p.Filtering.attributes {
			row = []models.InlineKeyboardButton{}
			for j, val := range attr.Values {
				row = append(row, models.InlineKeyboardButton{
					Text:         []string{"", "✓"}[btoi(attr.State[val])] + val,
					CallbackData: p.prefix + CB_FILTER_BY + attr.AttributeName + "/" + val,
				})
				if (j+1)%4 == 0 { // 4 buttons max
					keyboard = append(keyboard, row)
					row = []models.InlineKeyboardButton{}
				}
			}
			if len(row) > 0 {
				keyboard = append(keyboard, row)
			}
		}
	}

	row = []models.InlineKeyboardButton{
		chooseButton(p.activePage > 0,
			[2]buttonData{{"⬅", p.prefix + CB_PREV_PAGE}, {"-", p.prefix + CB_STUB}}),
		chooseButton(p.extControlsVisible,
			[2]buttonData{{"🔺", p.prefix + CB_TOGGLE_FILTERS}, {"🔻", p.prefix + CB_TOGGLE_FILTERS}}),
		chooseButton(p.activePage < ((p.Len()-1)/p.itemsPerPage),
			[2]buttonData{{"➡", p.prefix + CB_NEXT_PAGE}, {"-", p.prefix + CB_STUB}}),
	}
	keyboard = append(keyboard, row)

	if !p.extControlsVisible && (p.selectedItem >= fromIndex) && (p.selectedItem < toIndex) {
		row = []models.InlineKeyboardButton{}
		for i, action := range Stringer(p).Actions(p.selectedItem) {
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

func (p *Paginator) Show(ctx context.Context, b *bot.Bot, chatID any) {

	if callbackHandlerID, ok := callbackHandler[p.prefix]; ok {
		b.UnregisterHandler(callbackHandlerID)
	}
	callbackHandler[p.prefix] = b.RegisterHandler(bot.HandlerTypeCallbackQueryData, p.prefix, bot.MatchTypePrefix, p.callbackHandler)

	p.Ctx = ctx
	p.Bot = b
	p.text = p.buildText()
	p.keyboard.InlineKeyboard = p.buildKeyboard()

	var err error
	p.Message, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        p.text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: p.keyboard,
	})
	if err != nil {
		utils.LogError(err)
	}
}

func (p *Paginator) Refresh() {

	keyboard := p.buildKeyboard()
	text := p.buildText()
	textChanged := text != p.text
	kbdChanged := !reflect.DeepEqual(keyboard, p.keyboard.InlineKeyboard)

	if textChanged {
		p.text = text
	}
	if kbdChanged {
		p.keyboard.InlineKeyboard = keyboard
	}
	var err error
	if textChanged {
		_, err = p.Bot.EditMessageText(p.Ctx, &bot.EditMessageTextParams{
			ChatID:    p.Message.Chat.ID,
			MessageID: p.Message.ID,
			// InlineMessageID: p.callbackQuery.InlineMessageID,
			Text:        p.text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: p.keyboard,
		})
	}

	if !textChanged && kbdChanged {
		_, err = p.Bot.EditMessageReplyMarkup(p.Ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:    p.Message.Chat.ID,
			MessageID: p.Message.ID,
			// InlineMessageID: p.callbackQuery.InlineMessageID,
			ReplyMarkup: p.keyboard,
		})
	}

	if err != nil {
		utils.LogError(err)
	}
}

func (p *Paginator) callbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {

	cmd := strings.TrimPrefix(update.CallbackQuery.Data, p.prefix)
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		Text:            cmd,
		ShowAlert:       false,
	})

	if unicode.IsNumber(rune(cmd[0])) {
		p.selectedItem, _ = strconv.Atoi(cmd)
		p.extControlsVisible = false
	}

	switch cmd {
	case CB_NEXT_PAGE:
		if p.activePage < (p.Len() / p.itemsPerPage) {
			p.activePage++
		}

	case CB_PREV_PAGE:
		if p.activePage > 0 {
			p.activePage--
		}

	case CB_TOGGLE_FILTERS:
		p.extControlsVisible = !p.extControlsVisible
	}

	if len(cmd) > 10 {
		var payload = cmd[10:]
		switch cmd[0:10] {
		case CB_ORDER_BY:
			p.Sorting.ToggleKey(payload)
			p.Sort()
			p.selectedItem = -1
		case CB_FILTER_BY:
			split := strings.Split(payload, "/")
			p.Filtering.Toggle(split[0], split[1])
			p.activePage = 0
			p.selectedItem = -1
			p.Filter()
			p.Sort()
		case CB_ACTION:
			if p.selectedItem != -1 {
				if Executor(p).Execute(p.selectedItem, payload) {
					p.selectedItem = -1
				}
			}
		}
	}
	p.Refresh()
}
