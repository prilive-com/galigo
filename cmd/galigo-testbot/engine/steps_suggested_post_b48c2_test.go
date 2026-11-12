package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-48c2: DecideSuggestedPostStep (TEST-PLAN rows S1–S5).

// fakeDecide is a SenderClient for DecideSuggestedPostStep: it embeds the interface (any other method panics),
// records each approve / decline request and returns the scripted error.
type fakeDecide struct {
	SenderClient
	approves []sender.ApproveSuggestedPostRequest
	declines []sender.DeclineSuggestedPostRequest
	ctxs     []context.Context // the context of each call, in order
	err      error
}

func (f *fakeDecide) ApproveSuggestedPost(ctx context.Context, req sender.ApproveSuggestedPostRequest) error {
	f.ctxs = append(f.ctxs, ctx)
	f.approves = append(f.approves, req)
	return f.err
}

func (f *fakeDecide) DeclineSuggestedPost(ctx context.Context, req sender.DeclineSuggestedPostRequest) error {
	f.ctxs = append(f.ctxs, ctx)
	f.declines = append(f.declines, req)
	return f.err
}

const b48c2Chat = int64(-1001234567890)

func b48c2Runtime(f *fakeDecide, target int) *Runtime {
	rt := NewRuntime(f, b48c2Chat, 1)
	if target != 0 {
		rt.LastMessage = &tg.Message{MessageID: target, Chat: &tg.Chat{ID: b48c2Chat}}
	}
	return rt
}

func b48c2Refusal(text string) error {
	return &tg.APIError{Code: 400, Description: "Bad Request: " + text, Method: "x"}
}

type b48c2CtxKey struct{}

// S1: each step sends one request with the target id (or the target id + 1000000 with Missing) and its own value,
// passes the caller's context on, and records the id it sent.
func TestB48C2_Step_Requests(t *testing.T) {
	f := &fakeDecide{err: b48c2Refusal(SuggestedPostNotUsable)}
	rt := b48c2Runtime(f, 70)
	ctx := context.WithValue(context.Background(), b48c2CtxKey{}, "s1")
	_, err := (&DecideSuggestedPostStep{Method: "approveSuggestedPost", SendDate: 1900000000, WantError: SuggestedPostNotUsable}).Execute(ctx, rt)
	require.NoError(t, err)
	_, err = (&DecideSuggestedPostStep{Method: "declineSuggestedPost", Comment: "c", WantError: SuggestedPostNotUsable}).Execute(ctx, rt)
	require.NoError(t, err)
	f.err = b48c2Refusal(SuggestedPostNotFound)
	resA, err := (&DecideSuggestedPostStep{Method: "approveSuggestedPost", Missing: true, WantError: SuggestedPostNotFound}).Execute(ctx, rt)
	require.NoError(t, err)
	resD, err := (&DecideSuggestedPostStep{Method: "declineSuggestedPost", Missing: true, WantError: SuggestedPostNotFound}).Execute(ctx, rt)
	require.NoError(t, err)
	for _, res := range []*StepResult{resA, resD} { // the live checker reads this id (TEST-PLAN §7)
		require.NotNil(t, res)
		ev, ok := res.Evidence.(map[string]any)
		require.True(t, ok, "evidence %T", res.Evidence)
		assert.Equal(t, 1000070, ev["message_id"])
	}
	require.Len(t, f.ctxs, 4)
	for i, c := range f.ctxs {
		assert.Equal(t, "s1", c.Value(b48c2CtxKey{}), "call %d got the caller's context", i)
	}
	assert.Equal(t, []sender.ApproveSuggestedPostRequest{
		{ChatID: b48c2Chat, MessageID: 70, SendDate: 1900000000},
		{ChatID: b48c2Chat, MessageID: 1000070},
	}, f.approves)
	assert.Equal(t, []sender.DeclineSuggestedPostRequest{
		{ChatID: b48c2Chat, MessageID: 70, Comment: "c"},
		{ChatID: b48c2Chat, MessageID: 1000070},
	}, f.declines)
}

// S2: the step passes only on a 400 whose own text holds WantError (anywhere in it), also inside a wrapper, and records
// the server's own text; it fails on another text,
// another code, a non-API error, the wanted text only in a wrapper around an unrelated 400, and on success (the server
// took a message that is not a suggested post). Both target modes: the plain target (NotUsable) and Missing (NotFound,
// target + 1000000). The result carries the step's own name.
func TestB48C2_Step_Outcomes(t *testing.T) {
	for _, method := range []string{"approveSuggestedPost", "declineSuggestedPost"} {
		for _, mode := range []struct {
			prefix      string // the leaf name prefix: "" for the plain target, "missing_" for Missing
			missing     bool
			want, other string
			id          int
		}{
			{"", false, SuggestedPostNotUsable, SuggestedPostNotFound, 70},
			{"missing_", true, SuggestedPostNotFound, SuggestedPostNotUsable, 1000070},
		} {
			inText := &tg.APIError{Code: 400, Description: "Bad Request: x " + mode.want + " (y)"} // review findings A-T3-02 and O-T3-03
			for _, tc := range []struct {
				name    string
				err     error
				wantErr string // "" = the step passes
			}{
				{"wanted", b48c2Refusal(mode.want), ""},
				{"wanted_in_text", inText, ""},
				{"wrapped", fmt.Errorf("wrapper: %w", b48c2Refusal(mode.want)), ""},
				{"other_text", b48c2Refusal(mode.other), "want a 400 error containing"},
				{"other_code", &tg.APIError{Code: 403, Description: "Forbidden: " + mode.want}, "want a 400 error containing"},
				{"not_api", errors.New("Bad Request: " + mode.want), "want a 400 error containing"},
				{"wrapper_text", fmt.Errorf("%s: %w", mode.want, &tg.APIError{Code: 400, Description: "Bad Request: unrelated"}), "want a 400 error containing"},
				{"success", nil, fmt.Sprintf("took message %d", mode.id)},
			} {
				t.Run(method+"/"+mode.prefix+tc.name, func(t *testing.T) {
					f := &fakeDecide{err: tc.err}
					step := &DecideSuggestedPostStep{Method: method, Missing: mode.missing, WantError: mode.want}
					res, err := step.Execute(context.Background(), b48c2Runtime(f, 70))
					require.NotNil(t, res)
					assert.Equal(t, method, res.Method)
					assert.Equal(t, step.Name(), res.StepName)
					assert.Equal(t, 1, len(f.approves)+len(f.declines), "one request")
					if tc.wantErr == "" {
						require.NoError(t, err)
						var sent *tg.APIError
						require.True(t, errors.As(tc.err, &sent))
						assert.Equal(t, map[string]any{"message_id": mode.id, "error_code": 400, "description": sent.Description}, res.Evidence, "the server's own text")
						return
					}
					require.Error(t, err)
					assert.Contains(t, err.Error(), tc.wantErr)
					assert.Nil(t, res.Evidence)
				})
			}
		}
	}
}

// S3: no target message, or an unknown method, fails with no request.
func TestB48C2_Step_Refusals(t *testing.T) {
	f := &fakeDecide{err: b48c2Refusal(SuggestedPostNotUsable)}
	_, err := (&DecideSuggestedPostStep{Method: "approveSuggestedPost", WantError: SuggestedPostNotUsable}).Execute(context.Background(), b48c2Runtime(f, 0))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no target message")
	_, err = (&DecideSuggestedPostStep{Method: "x", WantError: SuggestedPostNotUsable}).Execute(context.Background(), b48c2Runtime(f, 70))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown method "x"`)
	assert.Empty(t, f.approves)
	assert.Empty(t, f.declines)
}

// S4: the step names.
func TestB48C2_Step_Names(t *testing.T) {
	assert.Equal(t, "approveSuggestedPost (9.2)", (&DecideSuggestedPostStep{Method: "approveSuggestedPost"}).Name())
	assert.Equal(t, "approveSuggestedPost (9.2), missing message", (&DecideSuggestedPostStep{Method: "approveSuggestedPost", Missing: true}).Name())
	assert.Equal(t, "declineSuggestedPost (9.2)", (&DecideSuggestedPostStep{Method: "declineSuggestedPost"}).Name())
	assert.Equal(t, "declineSuggestedPost (9.2), missing message", (&DecideSuggestedPostStep{Method: "declineSuggestedPost", Missing: true}).Name())
	// the two server texts, as probed live on 2026-11-12 (E29/lookups/probe-b48c2-raw-console.txt)
	assert.Equal(t, "message can't be used in the method", SuggestedPostNotUsable)
	assert.Equal(t, "suggested post not found", SuggestedPostNotFound)
}

// S5: the step with the real adapter and sender over a fake server that answers as the live server does in the test
// chat (MEASURED): NotFound when message_id is absent or names no message, NotUsable for the target.
func TestB48C2_StepThroughAdapter(t *testing.T) {
	for _, tc := range []struct {
		name string // fixed, not step.Name(): a mutant of Name() must not rename the leaves
		step *DecideSuggestedPostStep
		path string
		body string
	}{
		{"approve", &DecideSuggestedPostStep{Method: "approveSuggestedPost", SendDate: 1900000000, WantError: SuggestedPostNotUsable},
			"approveSuggestedPost", `{"chat_id":123456789,"message_id":70,"send_date":1900000000}`},
		{"decline", &DecideSuggestedPostStep{Method: "declineSuggestedPost", Comment: "c", WantError: SuggestedPostNotUsable},
			"declineSuggestedPost", `{"chat_id":123456789,"message_id":70,"comment":"c"}`},
		{"approve_missing", &DecideSuggestedPostStep{Method: "approveSuggestedPost", Missing: true, WantError: SuggestedPostNotFound},
			"approveSuggestedPost", `{"chat_id":123456789,"message_id":1000070}`},
		{"decline_missing", &DecideSuggestedPostStep{Method: "declineSuggestedPost", Missing: true, WantError: SuggestedPostNotFound},
			"declineSuggestedPost", `{"chat_id":123456789,"message_id":1000070}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/"+tc.path, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					MessageID int `json:"message_id"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body.MessageID == 70 {
					testutil.ReplyError(w, 400, "Bad Request: "+SuggestedPostNotUsable, nil)
					return
				}
				testutil.ReplyError(w, 400, "Bad Request: "+SuggestedPostNotFound, nil)
			})
			rt := NewRuntime(NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), testutil.TestChatID, testutil.TestUserID)
			rt.LastMessage = &tg.Message{MessageID: 70, Chat: &tg.Chat{ID: testutil.TestChatID}}
			_, err := tc.step.Execute(context.Background(), rt)
			require.NoError(t, err)
			require.Equal(t, 1, server.CaptureCount())
			assert.JSONEq(t, tc.body, string(server.LastCapture().Body))
		})
	}
}

// S5c: through the real adapter and sender, the server's other answers fail the step: success ("took message <id>",
// so the adapter cannot turn a success into a refusal), another 400 text and another code. Both methods, both target
// modes (O-T3-02; E-0123).
func TestB48C2_StepThroughAdapterOutcomes(t *testing.T) {
	for _, method := range []string{"approveSuggestedPost", "declineSuggestedPost"} {
		for _, mode := range []struct {
			prefix      string
			missing     bool
			want, other string
			id          int
		}{
			{"", false, SuggestedPostNotUsable, SuggestedPostNotFound, 70},
			{"missing_", true, SuggestedPostNotFound, SuggestedPostNotUsable, 1000070},
		} {
			for _, tc := range []struct {
				name    string
				reply   func(http.ResponseWriter)
				wantErr string
			}{
				{"success", func(w http.ResponseWriter) { testutil.ReplyBool(w, true) }, fmt.Sprintf("took message %d", mode.id)},
				{"other_text", func(w http.ResponseWriter) { testutil.ReplyError(w, 400, "Bad Request: "+mode.other, nil) }, "want a 400 error containing"},
				{"other_code", func(w http.ResponseWriter) { testutil.ReplyError(w, 403, "Forbidden: "+mode.want, nil) }, "want a 400 error containing"},
			} {
				t.Run(method+"/"+mode.prefix+tc.name, func(t *testing.T) {
					server := testutil.NewMockServer(t)
					server.On("/bot"+testutil.TestToken+"/"+method, func(w http.ResponseWriter, _ *http.Request) { tc.reply(w) })
					rt := NewRuntime(NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), testutil.TestChatID, testutil.TestUserID)
					rt.LastMessage = &tg.Message{MessageID: 70, Chat: &tg.Chat{ID: testutil.TestChatID}}
					res, err := (&DecideSuggestedPostStep{Method: method, Missing: mode.missing, WantError: mode.want}).Execute(context.Background(), rt)
					require.Error(t, err)
					assert.Contains(t, err.Error(), tc.wantErr)
					require.NotNil(t, res)
					assert.Nil(t, res.Evidence)
					assert.Equal(t, 1, server.CaptureCount())
				})
			}
		}
	}
}

// S5b: a cancelled context reaches the sender through the step and the real adapter: the step fails with
// context.Canceled and sends no request (E-0123: the adapter's ctx argument).
func TestB48C2_StepThroughAdapterCancel(t *testing.T) {
	for _, tc := range []struct {
		name string
		step *DecideSuggestedPostStep
	}{
		{"approve", &DecideSuggestedPostStep{Method: "approveSuggestedPost", WantError: SuggestedPostNotUsable}},
		{"decline", &DecideSuggestedPostStep{Method: "declineSuggestedPost", WantError: SuggestedPostNotUsable}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			rt := NewRuntime(NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), testutil.TestChatID, testutil.TestUserID)
			rt.LastMessage = &tg.Message{MessageID: 70, Chat: &tg.Chat{ID: testutil.TestChatID}}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := tc.step.Execute(ctx, rt)
			require.Error(t, err)
			assert.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}
