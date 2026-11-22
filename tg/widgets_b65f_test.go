package tg_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// B-65 65f: the two inline contents of Bot API 10.4 (TEST-PLAN C1, C2; C3 is sender/widgets_inline_b65f_test.go).

const (
	b65fWidgetFull = `{"html":"<b>w</b>","width":300,"height":200,"public_data":"pub-1","chat_data":"chat-2",` +
		`"caption":"  cap  ","parse_mode":"HTML","caption_entities":[{"type":"bold","offset":0,"length":2}],` +
		`"show_caption_above_media":true,"allow_queries":true,"is_shared":true}`
	b65fSharedFull = `{"widget_id":"wid-4","chat_data":"chat-2","caption":"  cap  ","parse_mode":"MarkdownV2",` +
		`"caption_entities":[{"type":"bold","offset":0,"length":2}],"show_caption_above_media":true}`
)

// C1: both contents encode the spec keys, full and minimal.
func TestB65F_C1_Encode(t *testing.T) {
	ents := []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 2}}
	rows := []struct {
		name string
		v    any
		want string
	}{
		{"widget full", tg.InputWidgetMessageContent{HTML: "<b>w</b>", Width: 300, Height: 200, PublicData: "pub-1",
			ChatData: "chat-2", Caption: "  cap  ", ParseMode: "HTML", CaptionEntities: ents, ShowCaptionAboveMedia: true,
			AllowQueries: true, IsShared: true}, b65fWidgetFull},
		{"widget minimal", tg.InputWidgetMessageContent{HTML: "<b>w</b>"}, `{"html":"<b>w</b>"}`},
		{"shared full", tg.InputSharedWidgetMessageContent{WidgetID: "wid-4", ChatData: "chat-2", Caption: "  cap  ",
			ParseMode: "MarkdownV2", CaptionEntities: ents, ShowCaptionAboveMedia: true}, b65fSharedFull},
		{"shared minimal", tg.InputSharedWidgetMessageContent{WidgetID: "wid-4"}, `{"widget_id":"wid-4"}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			b, err := json.Marshal(r.v)
			require.NoError(t, err)
			assert.JSONEq(t, r.want, string(b))
		})
	}
}

// C2: both are inline message contents (a compile-time proof).
var (
	_ tg.InputMessageContent = tg.InputWidgetMessageContent{}
	_ tg.InputMessageContent = tg.InputSharedWidgetMessageContent{}
)
