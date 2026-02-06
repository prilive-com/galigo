package tg_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// ==================== Button Constructors ====================

func TestBtn_CreatesCallbackButton(t *testing.T) {
	btn := tg.Btn("Click me", "action:click")
	assert.Equal(t, "Click me", btn.Text)
	assert.Equal(t, "action:click", btn.CallbackData)
	assert.Empty(t, btn.URL)
}

func TestBtnURL_CreatesURLButton(t *testing.T) {
	btn := tg.BtnURL("Visit", "https://example.com")
	assert.Equal(t, "Visit", btn.Text)
	assert.Equal(t, "https://example.com", btn.URL)
	assert.Empty(t, btn.CallbackData)
}

func TestBtnWebApp_CreatesWebAppButton(t *testing.T) {
	btn := tg.BtnWebApp("Open App", "https://webapp.example.com")
	assert.Equal(t, "Open App", btn.Text)
	require.NotNil(t, btn.WebApp)
	assert.Equal(t, "https://webapp.example.com", btn.WebApp.URL)
}

func TestBtnSwitch_CreatesSwitchButton(t *testing.T) {
	btn := tg.BtnSwitch("Share", "search query")
	assert.Equal(t, "Share", btn.Text)
	assert.Equal(t, "search query", btn.SwitchInlineQuery)
}

func TestBtnSwitchCurrent_CreatesSwitchCurrentButton(t *testing.T) {
	btn := tg.BtnSwitchCurrent("Search Here", "query")
	assert.Equal(t, "Search Here", btn.Text)
	assert.Equal(t, "query", btn.SwitchInlineQueryCurrentChat)
}

func TestBtnLogin_CreatesLoginButton(t *testing.T) {
	loginURL := tg.LoginURL{
		URL:                "https://auth.example.com",
		ForwardText:        "Login to Example",
		BotUsername:        "example_bot",
		RequestWriteAccess: true,
	}
	btn := tg.BtnLogin("Login", loginURL)
	assert.Equal(t, "Login", btn.Text)
	require.NotNil(t, btn.LoginURL)
	assert.Equal(t, "https://auth.example.com", btn.LoginURL.URL)
	assert.Equal(t, "Login to Example", btn.LoginURL.ForwardText)
	assert.Equal(t, "example_bot", btn.LoginURL.BotUsername)
	assert.True(t, btn.LoginURL.RequestWriteAccess)
}

func TestBtnPay_CreatesPayButton(t *testing.T) {
	btn := tg.BtnPay("Pay $10")
	assert.Equal(t, "Pay $10", btn.Text)
	assert.True(t, btn.Pay)
}

// ==================== Keyboard Builder ====================

func TestNewKeyboard_CreatesEmptyKeyboard(t *testing.T) {
	k := tg.NewKeyboard()
	assert.True(t, k.Empty())
	assert.Equal(t, 0, k.RowCount())
}

func TestKeyboard_Row_AddsRow(t *testing.T) {
	k := tg.NewKeyboard().
		Row(tg.Btn("Btn1", "data1")).
		Row(tg.Btn("Btn2", "data2"), tg.Btn("Btn3", "data3"))

	assert.False(t, k.Empty())
	assert.Equal(t, 2, k.RowCount())
}

func TestKeyboard_Row_IgnoresEmptyRow(t *testing.T) {
	k := tg.NewKeyboard().
		Row().
		Row(tg.Btn("Btn1", "data1"))

	assert.Equal(t, 1, k.RowCount())
}

func TestKeyboard_Add_CreatesNewRowWhenEmpty(t *testing.T) {
	k := tg.NewKeyboard().
		Add(tg.Btn("Btn1", "data1"))

	assert.Equal(t, 1, k.RowCount())
}

func TestKeyboard_Add_AppendsToLastRow(t *testing.T) {
	k := tg.NewKeyboard().
		Row(tg.Btn("Btn1", "data1")).
		Add(tg.Btn("Btn2", "data2"))

	assert.Equal(t, 1, k.RowCount())

	markup := k.Build()
	assert.Len(t, markup.InlineKeyboard[0], 2)
}

func TestKeyboard_Build_ReturnsMarkup(t *testing.T) {
	k := tg.NewKeyboard().
		Row(tg.Btn("Btn1", "data1"))

	markup := k.Build()
	require.NotNil(t, markup)
	assert.Len(t, markup.InlineKeyboard, 1)
	assert.Equal(t, "Btn1", markup.InlineKeyboard[0][0].Text)
}

func TestKeyboard_Inline_AliasForBuild(t *testing.T) {
	k := tg.NewKeyboard().Row(tg.Btn("Test", "data"))

	build := k.Build()
	inline := k.Inline()

	assert.Equal(t, build.InlineKeyboard, inline.InlineKeyboard)
}

func TestKeyboard_Empty(t *testing.T) {
	empty := tg.NewKeyboard()
	assert.True(t, empty.Empty())

	notEmpty := tg.NewKeyboard().Row(tg.Btn("Test", "data"))
	assert.False(t, notEmpty.Empty())
}

func TestKeyboard_RowCount(t *testing.T) {
	k := tg.NewKeyboard().
		Row(tg.Btn("R1", "d1")).
		Row(tg.Btn("R2", "d2")).
		Row(tg.Btn("R3", "d3"))

	assert.Equal(t, 3, k.RowCount())
}

func TestKeyboard_Rows_Iterator(t *testing.T) {
	k := tg.NewKeyboard().
		Row(tg.Btn("R1B1", "d1")).
		Row(tg.Btn("R2B1", "d2"), tg.Btn("R2B2", "d3"))

	var rowTexts [][]string
	for row := range k.Rows() {
		var texts []string
		for _, btn := range row {
			texts = append(texts, btn.Text)
		}
		rowTexts = append(rowTexts, texts)
	}

	assert.Len(t, rowTexts, 2)
	assert.Equal(t, []string{"R1B1"}, rowTexts[0])
	assert.Equal(t, []string{"R2B1", "R2B2"}, rowTexts[1])
}

func TestKeyboard_AllButtons_Iterator(t *testing.T) {
	k := tg.NewKeyboard().
		Row(tg.Btn("B1", "d1")).
		Row(tg.Btn("B2", "d2"), tg.Btn("B3", "d3"))

	var texts []string
	for btn := range k.AllButtons() {
		texts = append(texts, btn.Text)
	}

	assert.Equal(t, []string{"B1", "B2", "B3"}, texts)
}

func TestKeyboard_MarshalJSON(t *testing.T) {
	k := tg.NewKeyboard().
		Row(tg.Btn("Test", "data"))

	data, err := json.Marshal(k)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Contains(t, result, "inline_keyboard")
}

// ==================== Quick Builders ====================

func TestInlineKeyboard_CreatesMarkup(t *testing.T) {
	markup := tg.InlineKeyboard(
		tg.Row(tg.Btn("B1", "d1")),
		tg.Row(tg.Btn("B2", "d2"), tg.Btn("B3", "d3")),
	)

	assert.Len(t, markup.InlineKeyboard, 2)
	assert.Len(t, markup.InlineKeyboard[0], 1)
	assert.Len(t, markup.InlineKeyboard[1], 2)
}

func TestRow_CreatesButtonSlice(t *testing.T) {
	row := tg.Row(tg.Btn("B1", "d1"), tg.Btn("B2", "d2"))
	assert.Len(t, row, 2)
	assert.Equal(t, "B1", row[0].Text)
	assert.Equal(t, "B2", row[1].Text)
}

// ==================== Pagination ====================

func TestPagination_FirstPage(t *testing.T) {
	markup := tg.Pagination(1, 5, "page")

	assert.Len(t, markup.InlineKeyboard, 1)
	row := markup.InlineKeyboard[0]

	// First page: no Prev button
	assert.Len(t, row, 2) // Current, Next
	assert.Equal(t, "1/5", row[0].Text)
	assert.Contains(t, row[1].Text, "Next")
	assert.Equal(t, "page:2", row[1].CallbackData)
}

func TestPagination_MiddlePage(t *testing.T) {
	markup := tg.Pagination(3, 5, "page")

	assert.Len(t, markup.InlineKeyboard, 1)
	row := markup.InlineKeyboard[0]

	// Middle page: Prev, Current, Next
	assert.Len(t, row, 3)
	assert.Contains(t, row[0].Text, "Prev")
	assert.Equal(t, "page:2", row[0].CallbackData)
	assert.Equal(t, "3/5", row[1].Text)
	assert.Contains(t, row[2].Text, "Next")
	assert.Equal(t, "page:4", row[2].CallbackData)
}

func TestPagination_LastPage(t *testing.T) {
	markup := tg.Pagination(5, 5, "page")

	assert.Len(t, markup.InlineKeyboard, 1)
	row := markup.InlineKeyboard[0]

	// Last page: Prev, Current, no Next
	assert.Len(t, row, 2)
	assert.Contains(t, row[0].Text, "Prev")
	assert.Equal(t, "5/5", row[1].Text)
}

func TestPagination_SinglePage(t *testing.T) {
	markup := tg.Pagination(1, 1, "page")

	assert.Len(t, markup.InlineKeyboard, 1)
	row := markup.InlineKeyboard[0]

	// Single page: only Current
	assert.Len(t, row, 1)
	assert.Equal(t, "1/1", row[0].Text)
}

// ==================== Confirm ====================

func TestConfirm_CreatesYesNoKeyboard(t *testing.T) {
	markup := tg.Confirm("yes:123", "no:123")

	assert.Len(t, markup.InlineKeyboard, 1)
	row := markup.InlineKeyboard[0]

	assert.Len(t, row, 2)
	assert.Equal(t, "Yes", row[0].Text)
	assert.Equal(t, "yes:123", row[0].CallbackData)
	assert.Equal(t, "No", row[1].Text)
	assert.Equal(t, "no:123", row[1].CallbackData)
}

func TestConfirmCustom_CreatesCustomLabels(t *testing.T) {
	markup := tg.ConfirmCustom("Accept", "accept:id", "Decline", "decline:id")

	assert.Len(t, markup.InlineKeyboard, 1)
	row := markup.InlineKeyboard[0]

	assert.Len(t, row, 2)
	assert.Equal(t, "Accept", row[0].Text)
	assert.Equal(t, "accept:id", row[0].CallbackData)
	assert.Equal(t, "Decline", row[1].Text)
	assert.Equal(t, "decline:id", row[1].CallbackData)
}

// ==================== Grid ====================

func TestGrid_CreatesButtonGrid(t *testing.T) {
	items := []string{"A", "B", "C", "D", "E"}
	markup := tg.Grid(items, 2, func(s string) tg.InlineKeyboardButton {
		return tg.Btn(s, "item:"+s)
	})

	// 5 items, 2 columns = 3 rows (2, 2, 1)
	assert.Len(t, markup.InlineKeyboard, 3)
	assert.Len(t, markup.InlineKeyboard[0], 2) // A, B
	assert.Len(t, markup.InlineKeyboard[1], 2) // C, D
	assert.Len(t, markup.InlineKeyboard[2], 1) // E

	assert.Equal(t, "A", markup.InlineKeyboard[0][0].Text)
	assert.Equal(t, "B", markup.InlineKeyboard[0][1].Text)
	assert.Equal(t, "E", markup.InlineKeyboard[2][0].Text)
}

func TestGrid_ExactFit(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6}
	markup := tg.Grid(items, 3, func(n int) tg.InlineKeyboardButton {
		return tg.Btn("N", "n")
	})

	// 6 items, 3 columns = 2 rows exactly
	assert.Len(t, markup.InlineKeyboard, 2)
	assert.Len(t, markup.InlineKeyboard[0], 3)
	assert.Len(t, markup.InlineKeyboard[1], 3)
}

func TestGrid_EmptyItems(t *testing.T) {
	var items []string
	markup := tg.Grid(items, 2, func(s string) tg.InlineKeyboardButton {
		return tg.Btn(s, s)
	})

	assert.Empty(t, markup.InlineKeyboard)
}
