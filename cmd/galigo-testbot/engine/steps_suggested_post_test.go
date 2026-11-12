package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/tg"
)

// B-48c1: SendSuggestedPostStep, WithSuggestedPostParameters and the adapter's pass-through.

// fakeSuggested is a SenderClient for SendSuggestedPostStep: it embeds the interface (any other method panics),
// records each SendMessage / SendPhoto call with its chat id and applied options, and returns the scripted reply.
type fakeSuggested struct {
	SenderClient
	calls  []string
	chats  []int64
	opts   []SendOptions
	texts  []string
	photos []MediaInput
	msg    *tg.Message
	err    error
}

func (f *fakeSuggested) record(method string, chatID int64, opts []SendOption) {
	o := SendOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	f.calls = append(f.calls, method)
	f.chats = append(f.chats, chatID)
	f.opts = append(f.opts, o)
}

func (f *fakeSuggested) SendMessage(_ context.Context, chatID int64, text string, opts ...SendOption) (*tg.Message, error) {
	f.record("sendMessage", chatID, opts)
	f.texts = append(f.texts, text)
	return f.msg, f.err
}

func (f *fakeSuggested) SendPhoto(_ context.Context, chatID int64, photo MediaInput, opts ...SendOption) (*tg.Message, error) {
	f.record("sendPhoto", chatID, opts)
	f.photos = append(f.photos, photo)
	return f.msg, f.err
}

func TestB48C1_WithSuggestedPostParameters(t *testing.T) {
	p := &tg.SuggestedPostParameters{SendDate: 5}
	o := SendOptions{}
	WithSuggestedPostParameters(p)(&o)
	assert.Same(t, p, o.SuggestedPostParameters)
	WithSuggestedPostParameters(nil)(&o)
	assert.Nil(t, o.SuggestedPostParameters)
}

func TestB48C1_Step(t *testing.T) {
	params := &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: 5}}
	wrongChat := &tg.APIError{Code: 400, Description: "Bad Request: " + SuggestedPostWrongChat}
	wrongAmount := &tg.APIError{Code: 400, Description: "Bad Request: " + SuggestedPostWrongAmount}
	for _, method := range []string{"sendMessage", "sendPhoto"} {
		step := func() *SendSuggestedPostStep {
			return &SendSuggestedPostStep{Method: method, Params: params, WantError: SuggestedPostWrongChat}
		}
		runStep := func(t *testing.T, s *SendSuggestedPostStep, f *fakeSuggested) (*Runtime, *StepResult, error) {
			t.Helper()
			rt := NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
			res, err := s.Execute(context.Background(), rt)
			require.Equal(t, []string{method}, f.calls, "one call of the step's method")
			assert.Equal(t, []int64{testutil.TestChatID}, f.chats, "sent to the test chat")
			assert.Same(t, params, f.opts[0].SuggestedPostParameters, "the step's parameters reach the sender")
			return rt, res, err
		}
		run := func(t *testing.T, f *fakeSuggested) (*Runtime, *StepResult, error) {
			t.Helper()
			return runStep(t, step(), f)
		}
		t.Run(method+"/refused", func(t *testing.T) {
			f := &fakeSuggested{err: wrongChat}
			rt, res, err := run(t, f)
			require.NoError(t, err)
			assert.Equal(t, method+" suggested_post_parameters (9.2)", step().Name())
			assert.Equal(t, method, res.Method)
			assert.Equal(t, map[string]any{"error_code": 400, "description": wrongChat.Description}, res.Evidence)
			assert.Empty(t, rt.CreatedMessages)
			if method == "sendPhoto" {
				require.Len(t, f.photos, 1)
				ph := f.photos[0]
				assert.Equal(t, "galigo-suggested.jpg", ph.FileName)
				assert.Equal(t, "photo", ph.Type)
				assert.Empty(t, ph.FileID)
				assert.Empty(t, ph.URL)
				require.NotNil(t, ph.Reader, "an upload: the form path")
				assert.Equal(t, fixtures.PhotoBytes(), ph.Reader())
			} else {
				assert.Equal(t, []string{"galigo suggested post"}, f.texts)
			}
		})
		t.Run(method+"/refused_wrapped", func(t *testing.T) { // the wanted 400 inside a wrapper passes too [O-T2-04]
			rt, res, err := run(t, &fakeSuggested{err: fmt.Errorf("wrapper: %w", wrongChat)})
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"error_code": 400, "description": wrongChat.Description}, res.Evidence)
			assert.Empty(t, rt.CreatedMessages)
		})
		t.Run(method+"/taken", func(t *testing.T) {
			rt, res, err := run(t, &fakeSuggested{msg: &tg.Message{MessageID: 4242}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "did not send suggested_post_parameters")
			assert.Equal(t, []CreatedMessage{{ChatID: testutil.TestChatID, MessageID: 4242}}, rt.CreatedMessages, "tracked for cleanup")
			assert.Equal(t, []int{4242}, res.MessageIDs)
		})
		t.Run(method+"/taken_no_message", func(t *testing.T) {
			rt, _, err := run(t, &fakeSuggested{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "did not send suggested_post_parameters")
			assert.Empty(t, rt.CreatedMessages)
		})
		// A step that wants the amount text passes on that text and fails on the wrong-chat text.
		amountStep := func() *SendSuggestedPostStep {
			return &SendSuggestedPostStep{Method: method, Params: params, WantError: SuggestedPostWrongAmount}
		}
		t.Run(method+"/refused_amount", func(t *testing.T) {
			rt, res, err := runStep(t, amountStep(), &fakeSuggested{err: wrongAmount})
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"error_code": 400, "description": wrongAmount.Description}, res.Evidence)
			assert.Empty(t, rt.CreatedMessages)
		})
		t.Run(method+"/amount_wants_other_text", func(t *testing.T) {
			rt, res, err := runStep(t, amountStep(), &fakeSuggested{err: wrongChat})
			require.Error(t, err)
			assert.ErrorIs(t, err, wrongChat)
			assert.Nil(t, res.Evidence)
			assert.Empty(t, rt.CreatedMessages)
		})
		for _, bad := range []struct {
			name string
			err  error
		}{
			{"other_text", &tg.APIError{Code: 400, Description: "Bad Request: " + SuggestedPostWrongAmount}},
			{"other_code", &tg.APIError{Code: 403, Description: "Forbidden: " + SuggestedPostWrongChat}},
			{"not_api", errors.New(SuggestedPostWrongChat)},
			// the wanted text only in a wrapper around an unrelated 400: the API error's own text decides [O-T3-06]
			{"wrapper_text", fmt.Errorf("%s: %w", SuggestedPostWrongChat, &tg.APIError{Code: 400, Description: "Bad Request: unrelated"})},
		} {
			t.Run(method+"/"+bad.name, func(t *testing.T) {
				rt, res, err := run(t, &fakeSuggested{err: bad.err})
				require.Error(t, err)
				assert.ErrorIs(t, err, bad.err)
				assert.Nil(t, res.Evidence)
				assert.Empty(t, rt.CreatedMessages)
			})
		}
	}
	t.Run("unknown_method", func(t *testing.T) {
		f := &fakeSuggested{}
		_, err := (&SendSuggestedPostStep{Method: "sendVideo"}).Execute(context.Background(), NewRuntime(f, testutil.TestChatID, testutil.TestUserID))
		require.Error(t, err)
		assert.Empty(t, f.calls)
	})
}

// The step with the real adapter and sender over a fake server that refuses only a body carrying the parameter, as
// the live server does outside a channel direct messages chat: the step passes only if the parameter reaches the wire,
// with its value, in a request to the test chat.
func TestB48C1_StepThroughAdapter(t *testing.T) {
	params := &tg.SuggestedPostParameters{Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: 5}, SendDate: 1900000000}
	const paramsJSON = `{"price":{"currency":"XTR","amount":5},"send_date":1900000000}`
	for _, method := range []string{"sendMessage", "sendPhoto"} {
		t.Run(method, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/"+method, func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if bytes.Contains(body, []byte("suggested_post_parameters")) {
					testutil.ReplyError(w, 400, "Bad Request: "+SuggestedPostWrongChat, nil)
					return
				}
				testutil.ReplyMessage(w, 4242)
			})
			rt := NewRuntime(NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), testutil.TestChatID, testutil.TestUserID)
			_, err := (&SendSuggestedPostStep{Method: method, Params: params, WantError: SuggestedPostWrongChat}).
				Execute(context.Background(), rt)
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			c := server.LastCapture()
			if method == "sendPhoto" {
				mt, mp, err := mime.ParseMediaType(c.ContentType)
				require.NoError(t, err)
				require.Equal(t, "multipart/form-data", mt)
				form, err := multipart.NewReader(bytes.NewReader(c.Body), mp["boundary"]).ReadForm(1 << 20)
				require.NoError(t, err)
				assert.Equal(t, []string{strconv.FormatInt(testutil.TestChatID, 10)}, form.Value["chat_id"])
				require.Len(t, form.Value["suggested_post_parameters"], 1)
				assert.JSONEq(t, paramsJSON, form.Value["suggested_post_parameters"][0])
				assert.Len(t, form.File["photo"], 1, "the photo is an upload")
			} else {
				assert.JSONEq(t, `{"chat_id":123456789,"text":"galigo suggested post","suggested_post_parameters":`+paramsJSON+`}`, c.BodyString())
			}
			assert.Empty(t, rt.CreatedMessages)
		})
	}
}

// The real adapter with no option or a nil option sends no suggested_post_parameters, a non-nil empty value as {},
// and other values unchanged, on both methods [A-T2-01 = O-T2-03, A-T3-02 = O-T3-04].
func TestB48C1_AdapterOption(t *testing.T) {
	for _, method := range []string{"sendMessage", "sendPhoto"} {
		for _, r := range []struct {
			name string
			opts []SendOption
			want string // the parameter's JSON; "" = the key is absent
		}{
			{"no_option", nil, ""},
			{"nil_option", []SendOption{WithSuggestedPostParameters(nil)}, ""},
			{"empty", []SendOption{WithSuggestedPostParameters(&tg.SuggestedPostParameters{})}, `{}`},
			{"xtr5_no_date", []SendOption{WithSuggestedPostParameters(&tg.SuggestedPostParameters{
				Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: 5}})}, `{"price":{"currency":"XTR","amount":5}}`},
			{"xtr1_no_date", []SendOption{WithSuggestedPostParameters(&tg.SuggestedPostParameters{
				Price: &tg.SuggestedPostPrice{Currency: "XTR", Amount: 1}})}, `{"price":{"currency":"XTR","amount":1}}`},
			{"date_only", []SendOption{WithSuggestedPostParameters(&tg.SuggestedPostParameters{SendDate: 1900000000})},
				`{"send_date":1900000000}`},
		} {
			t.Run(method+"/"+r.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On("/bot"+testutil.TestToken+"/"+method, func(w http.ResponseWriter, _ *http.Request) {
					testutil.ReplyMessage(w, 4242)
				})
				a := NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL()))
				var err error
				if method == "sendPhoto" {
					_, err = a.SendPhoto(context.Background(), testutil.TestChatID, MediaFromBytes(fixtures.PhotoBytes(), "galigo-suggested.jpg", "photo"), r.opts...)
				} else {
					_, err = a.SendMessage(context.Background(), testutil.TestChatID, "galigo suggested post", r.opts...)
				}
				require.NoError(t, err)
				require.Equal(t, 1, server.CaptureCount())
				c := server.LastCapture()
				var got []string
				if method == "sendPhoto" {
					mt, mp, err := mime.ParseMediaType(c.ContentType)
					require.NoError(t, err)
					require.Equal(t, "multipart/form-data", mt)
					form, err := multipart.NewReader(bytes.NewReader(c.Body), mp["boundary"]).ReadForm(1 << 20)
					require.NoError(t, err)
					assert.Len(t, form.File["photo"], 1, "the photo is an upload")
					got = form.Value["suggested_post_parameters"]
				} else {
					var raw map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(c.Body, &raw))
					if v, ok := raw["suggested_post_parameters"]; ok {
						got = []string{string(v)}
					}
				}
				if r.want == "" {
					assert.Empty(t, got, "suggested_post_parameters sent")
				} else if assert.Len(t, got, 1) {
					assert.JSONEq(t, r.want, got[0])
				}
			})
		}
	}
}
