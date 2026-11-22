package suites

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65g: S77 rich quotes (TEST-PLAN section 4). Every value is written out here.

func TestB65G_S77(t *testing.T) {
	sc := S77_RichQuote()
	assert.Equal(t, "S77-RichQuote", sc.Name())
	assert.Equal(t, []string{"sendRichMessage", "sendMessage"}, sc.Covers())
	assert.Equal(t, 60*time.Second, sc.Timeout())
	bold := tg.RichText{Node: tg.RichTextBold{Type: "bold", Text: tg.RichText{Plain: "bold"}}}
	assert.Equal(t, []engine.Step{
		&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{Blocks: []tg.InputRichBlock{
			tg.InputRichBlockParagraph{Text: tg.RichText{Parts: []tg.RichText{{Plain: "galigo quote "}, bold, {Plain: " end"}}}},
			tg.InputRichBlockParagraph{Text: tg.RichText{Plain: "second para"}},
		}}, WantBlocks: []string{"paragraph", "paragraph"}},
		&engine.ReplyRichQuoteStep{Label: "plain", Texts: []tg.RichText{{Plain: "quote"}}, Position: 7},
		&engine.ReplyRichQuoteStep{Label: "bold", Texts: []tg.RichText{{Parts: []tg.RichText{{Plain: "quote "}, bold}}}, Position: 7},
		&engine.ReplyRichQuoteStep{Label: "two texts", Texts: []tg.RichText{{Plain: "end"}, {Plain: "second"}}, Position: 18},
		&engine.ReplyRichQuoteStep{Label: "not in the message", Texts: []tg.RichText{{Plain: "absent text"}}, WantError: "QUOTE_TEXT_INVALID"},
		&engine.CleanupStep{},
	}, sc.Steps())
}
