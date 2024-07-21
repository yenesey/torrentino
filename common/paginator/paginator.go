package paginator

import (
	"context"
	"slices"

	"reflect"
	"strconv"
	"strings"
	"unicode"

	"torrentino/common/utils"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
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
type VirtualMethods interface {
	HeaderString() string
	FooterString() string
	ItemString(item any) string
	AttributeByName(item any, attributeName string) string
	ItemActions(item any) []string
	ItemActionExec(item any, actionKey string) (unselectItem bool)
	LessItem(i int, j int, attributeName string) bool
	Reload()
}

type Paginator struct {
	virtual VirtualMethods
	list    []any
	index   []int

	Sorting   SortingState
	Filtering FilteringState

	actions            []string
	extControlsVisible bool
	activePage         int
	itemsPerPage       int
	selectedItem       int

	Bot    *bot.Bot
	ChatID any
	Ctx    context.Context

	prefix      string
	message     *models.Message
	text        string
	kbd         models.InlineKeyboardMarkup
	textChanged bool
	kbdChanged  bool
}

func New(virtualMethods VirtualMethods, prefix string, itemsPerPage int) *Paginator {
	var pg = &Paginator{
		virtual:      virtualMethods,
		itemsPerPage: itemsPerPage,
		prefix:       prefix,
	}
	pg.selectedItem = -1
	return pg
}

func (p *Paginator) Alloc(l int) {
	p.list = make([]any, 0, l)
	p.index = make([]int, 0, l)
}

func (p *Paginator) Append(item any) {
	p.list = append(p.list, item)
	p.index = append(p.index, len(p.list)-1)
}

func (p *Paginator) Delete(i int) {
	idx := p.index[i]
	p.list = slices.Delete(p.list, idx, idx+1) // p.list = append(p.list[:idx], p.list[idx+1:]...)
	p.Filter()                                 // <-- just for rebuild the indexes
}

func (p *Paginator) Item(i int) any {
	return p.list[p.index[i]]
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

func (p *Paginator) HeaderString() string {
	var fromIndex, toIndex = p.pageBounds()
	return "<b>results: " + strconv.Itoa(fromIndex+1) + "-" + strconv.Itoa(toIndex) + " of " + strconv.Itoa(p.Len()) + "</b>"
}

func (p *Paginator) ItemString(item any) string {
	return ""
}

func (p *Paginator) FooterString() string {
	return ""
}

func (p *Paginator) AttributeByName(item any, attributeName string) string {
	return ""
}

func (p *Paginator) LessItem(i int, j int, attributeName string) bool {
	return false
}

func (p *Paginator) ItemActions(item any) []string {
	return nil
}

func (p *Paginator) ItemActionExec(item any, actionKey string) (unselectItem bool) {
	return true
}

func (p *Paginator) Reload() {
	p.Filtering.pg = p
	p.Filtering.ClassifyItems()
	p.Filter()
	p.Sort()
}

// ----------------------------------------
func (p *Paginator) buildText() {

	var text string
	hr := "\n<b>⸻⸻⸻⸻⸻</b>\n"
	br := "\n\n"

	text = text + p.virtual.HeaderString() + hr
	fromIndex, toIndex := p.pageBounds()
	for i := fromIndex; i < toIndex; i++ {
		text = text + "<b>" + strconv.Itoa(i+1) + ".</b> " +
			(func() string {
				if p.selectedItem == i {
					return "<u>" + p.virtual.ItemString(p.Item(i)) + "</u>"
				} else {
					return p.virtual.ItemString(p.Item(i))
				}
			})()
		if i < toIndex-1 {
			text = text + br
		}
	}
	footer := p.virtual.FooterString()
	if len(footer) > 0 {
		text = text + hr + "<b>" + footer + "</b>"
	}

	p.textChanged = (text != p.text)
	if p.textChanged {
		p.text = text
	}
}

func (p *Paginator) buildKeyboard() {

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

	var kbd [][]models.InlineKeyboardButton
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
		kbd = append(kbd, row)
	}

	if p.extControlsVisible {
		row = []models.InlineKeyboardButton{}
		for _, v := range p.Sorting.headers {
			row = append(row, models.InlineKeyboardButton{
				Text:         v.ShortName + sortChars[int(v.Order)],
				CallbackData: p.prefix + CB_ORDER_BY + v.Name,
			})
		}
		if len(row) > 0 {
			kbd = append(kbd, row)
		}

		for _, attr := range p.Filtering.attributes {
			row = []models.InlineKeyboardButton{}
			for j, val := range attr.Values {
				row = append(row, models.InlineKeyboardButton{
					Text:         []string{"", "✓"}[btoi(val.Enabled)] + val.Value,
					CallbackData: p.prefix + CB_FILTER_BY + attr.Name + "/" + val.Value,
				})
				if (j+1)%4 == 0 { // 4 buttons max
					kbd = append(kbd, row)
					row = []models.InlineKeyboardButton{}
				}
			}
			if len(row) > 0 {
				kbd = append(kbd, row)
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
	kbd = append(kbd, row)

	if !p.extControlsVisible && (p.selectedItem >= fromIndex) && (p.selectedItem < toIndex) {
		p.actions = p.virtual.ItemActions(p.Item(p.selectedItem))
		row = []models.InlineKeyboardButton{}
		for i, action := range p.actions {
			row = append(row, models.InlineKeyboardButton{
				Text:         action,
				CallbackData: p.prefix + CB_ACTION + action,
			})
			if (i+1)%2 == 0 {
				kbd = append(kbd, row)
				row = []models.InlineKeyboardButton{}
			}
		}
		if len(row) > 0 {
			kbd = append(kbd, row)
		}
	}

	p.kbdChanged = !reflect.DeepEqual(kbd, p.kbd.InlineKeyboard)
	if p.kbdChanged {
		p.kbd.InlineKeyboard = kbd
	}
}

func (p *Paginator) Show(ctx context.Context, b *bot.Bot, chatID any) *models.Message {

	if callbackHandlerID, ok := callbackHandler[p.prefix]; ok {
		b.UnregisterHandler(callbackHandlerID)
	}
	callbackHandler[p.prefix] = b.RegisterHandler(bot.HandlerTypeCallbackQueryData, p.prefix, bot.MatchTypePrefix, p.callbackHandler)

	p.Ctx = ctx
	p.Bot = b
	p.ChatID = chatID

	p.buildText()
	p.buildKeyboard()

	var err error
	p.message, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      chatID,
		Text:        p.text,
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: p.kbd,
	})
	if err != nil {
		utils.LogError(err)
	}
	return p.message
}

func (p *Paginator) Refresh() {
	p.buildKeyboard()
	p.buildText()
	if p.textChanged {
		var _, err = p.Bot.EditMessageText(p.Ctx, &bot.EditMessageTextParams{
			ChatID:    p.message.Chat.ID,
			MessageID: p.message.ID,
			// InlineMessageID: p.callbackQuery.InlineMessageID,
			Text:        p.text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: p.kbd,
		})
		if err != nil {
			utils.LogError(err)
		}
	}

	if !p.textChanged && p.kbdChanged {
		var _, err = p.Bot.EditMessageReplyMarkup(p.Ctx, &bot.EditMessageReplyMarkupParams{
			ChatID:    p.message.Chat.ID,
			MessageID: p.message.ID,
			// InlineMessageID: p.callbackQuery.InlineMessageID,
			ReplyMarkup: p.kbd,
		})
		if err != nil {
			utils.LogError(err)
		}
	}
}

func (p *Paginator) callbackHandler(ctx context.Context, b *bot.Bot, update *models.Update) {

	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})

	var cmd = strings.TrimPrefix(update.CallbackQuery.Data, p.prefix)

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
			var split = strings.Split(payload, "/")
			var hdr = p.Filtering.Get(split[0], split[1])
			hdr.Enabled = !hdr.Enabled
			p.activePage = 0
			p.selectedItem = -1
			p.Filter()
			p.Sort()
		case CB_ACTION:
			if p.selectedItem != -1 {
				for i := range p.actions {
					if p.actions[i] == payload {
						if p.virtual.ItemActionExec(p.Item(p.selectedItem), payload) {
							p.selectedItem = -1
						}
					}
				}
			}
		}
	}
	p.Refresh()
}
