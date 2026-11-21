package receiver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.4 widget queries through the real receiver paths (B-65 65e, TEST-PLAN §3): the webhook handler and a
// running PollingClient (the 65d helpers api102aWebhook and b65dPoll). Ids are fake.

const (
	b65eQueryBody = `{"update_id":10,"widget_query":{"id":"q1","from":{"id":456,"is_bot":false,"first_name":"u"},` +
		`"chat":{"id":-1009876543210,"type":"supergroup"},"message_id":5,"chat_instance":"ci1",` +
		`"type":{"type":"sign_data","wallet":{"address":"a","amount":"1"},"sign_data":"{\"t\":1}"},"data":"d1","widget_id":"w1"}}`
	// an inline widget query: no chat, an inline_message_id and a chat_type (change round c1, O-C1-01)
	b65eInlineBody = `{"update_id":11,"widget_query":{"id":"q2","from":{"id":456,"is_bot":false,"first_name":"u"},` +
		`"inline_message_id":"im1","chat_instance":"ci2","chat_type":"sender","type":{"type":"data"},"data":"d2"}}`
	b65eSharedBody = `{"update_id":12,"shared_widget_query":{"id":"q3","from":{"id":456,"is_bot":false,"first_name":"u"},` +
		`"type":{"type":"data"},"data":"d3","widget_id":"w3"}}`
)

func b65eWantQuery() tg.Update {
	return tg.Update{UpdateID: 10, WidgetQuery: &tg.WidgetQuery{ID: "q1", From: &tg.User{ID: 456, FirstName: "u"},
		Chat: &tg.Chat{ID: -1009876543210, Type: "supergroup"}, MessageID: 5, ChatInstance: "ci1",
		Type: tg.WidgetQueryType{Type: "sign_data", Wallet: &tg.WidgetQueryWallet{Address: "a", Amount: "1"}, SignData: `{"t":1}`},
		Data: "d1", WidgetID: "w1"}}
}

func b65eWantInline() tg.Update {
	return tg.Update{UpdateID: 11, WidgetQuery: &tg.WidgetQuery{ID: "q2", From: &tg.User{ID: 456, FirstName: "u"},
		InlineMessageID: "im1", ChatInstance: "ci2", ChatType: "sender", Type: tg.WidgetQueryType{Type: "data"}, Data: "d2"}}
}

func b65eWantShared() tg.Update {
	return tg.Update{UpdateID: 12, SharedWidgetQuery: &tg.SharedWidgetQuery{ID: "q3", From: &tg.User{ID: 456, FirstName: "u"},
		Type: tg.WidgetQueryType{Type: "data"}, Data: "d3", WidgetID: "w3"}}
}

// T-recv-query
func TestB65E_ReceiveQuery(t *testing.T) {
	t.Run("webhook", func(t *testing.T) { assert.Equal(t, b65eWantQuery(), api102aWebhook(t, b65eQueryBody)) })
	t.Run("polling", func(t *testing.T) { assert.Equal(t, b65eWantQuery(), b65dPoll(t, b65eQueryBody)) })
	t.Run("webhook_inline", func(t *testing.T) { assert.Equal(t, b65eWantInline(), api102aWebhook(t, b65eInlineBody)) })
	t.Run("polling_inline", func(t *testing.T) { assert.Equal(t, b65eWantInline(), b65dPoll(t, b65eInlineBody)) })
}

// T-recv-shared
func TestB65E_ReceiveShared(t *testing.T) {
	t.Run("webhook", func(t *testing.T) { assert.Equal(t, b65eWantShared(), api102aWebhook(t, b65eSharedBody)) })
	t.Run("polling", func(t *testing.T) { assert.Equal(t, b65eWantShared(), b65dPoll(t, b65eSharedBody)) })
}
