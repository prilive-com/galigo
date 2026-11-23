package engine

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-65 65i: GetUniqueGiftStep and DeleteBusinessAccountGiftStep (TEST-PLAN section 4). The fake answers from a
// script, one answer per call, and records every call with its arguments and its context. Fake ids only.

const (
	b65iMT       = "Bad Request: business connection not found" // the measured refusal (probes p1, p2)
	b65iGuardErr = "deleteBusinessAccountGift: guard: getBusinessConnection did not refuse the test connection; the delete is not sent"
)

type b65iAnswer struct {
	gift *tg.UniqueGift
	conn *tg.BusinessConnection
	err  error
}

type b65iFake struct {
	SenderClient
	script []b65iAnswer
	calls  []string
	ctxs   []context.Context
}

func (f *b65iFake) next(ctx context.Context, call string) b65iAnswer {
	f.calls = append(f.calls, call)
	f.ctxs = append(f.ctxs, ctx)
	if len(f.script) == 0 {
		return b65iAnswer{err: errors.New("b65iFake: no scripted answer for " + call)}
	}
	a := f.script[0]
	f.script = f.script[1:]
	return a
}

func (f *b65iFake) GetUniqueGift(ctx context.Context, name string) (*tg.UniqueGift, error) {
	a := f.next(ctx, "GetUniqueGift("+name+")")
	return a.gift, a.err
}

func (f *b65iFake) GetBusinessConnection(ctx context.Context, id string) (*tg.BusinessConnection, error) {
	a := f.next(ctx, "GetBusinessConnection("+id+")")
	return a.conn, a.err
}

func (f *b65iFake) DeleteBusinessAccountGift(ctx context.Context, req sender.DeleteBusinessAccountGiftRequest) error {
	a := f.next(ctx, "DeleteBusinessAccountGift("+req.BusinessConnectionID+","+req.OwnedGiftID+")")
	return a.err
}

const (
	b65iGuardCall  = "GetBusinessConnection(zzNoSuchConnection)"
	b65iDeleteCall = "DeleteBusinessAccountGift(zzNoSuchConnection,zzNoSuchGift)"
)

func b65iAPI(code int, desc string) error { return &tg.APIError{Code: code, Description: desc} }

func b65i429() error {
	return &tg.APIError{Code: 429, Description: "Too Many Requests", RetryAfter: time.Millisecond}
}

func b65iGift() *tg.UniqueGift {
	return &tg.UniqueGift{GiftID: "g1", BaseName: "Plush Pepe", Name: "PlushPepe-1", Number: 1}
}

type b65iCtxKey struct{}

// b65iExec runs one step with its own context and checks that every call got that context.
func b65iExec(t *testing.T, step Step, f *b65iFake) (*StepResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), b65iCtxKey{}, "run"), time.Minute)
	defer cancel()
	var res *StepResult
	var err error
	require.NotPanics(t, func() { res, err = step.Execute(ctx, NewRuntime(f, testutil.TestChatID, 0)) })
	for i, c := range f.ctxs {
		assert.True(t, c == ctx, "call %d (%s) got the step's own context", i, f.calls[i])
	}
	return res, err
}

// E-ug-ok: the whole result and the one call.
func TestB65I_UniqueGiftOK(t *testing.T) {
	f := &b65iFake{script: []b65iAnswer{{gift: b65iGift()}}}
	res, err := b65iExec(t, &GetUniqueGiftStep{GiftName: "PlushPepe-1"}, f)
	require.NoError(t, err)
	assert.Equal(t, &StepResult{StepName: "getUniqueGift", Method: "getUniqueGift",
		Evidence: map[string]any{"name": "PlushPepe-1", "gift_id": "g1", "number": 1}}, res)
	assert.Equal(t, []string{"GetUniqueGift(PlushPepe-1)"}, f.calls)
}

// E-ug-fields: each required value is checked.
func TestB65I_UniqueGiftFields(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		edit        func(*tg.UniqueGift)
	}{
		{"other_name", "name", func(g *tg.UniqueGift) { g.Name = "Other-1" }},
		{"empty_gift_id", "gift_id", func(g *tg.UniqueGift) { g.GiftID = "" }},
		{"empty_base_name", "base_name", func(g *tg.UniqueGift) { g.BaseName = "" }},
		{"number_0", "number", func(g *tg.UniqueGift) { g.Number = 0 }},
		{"number_negative", "number", func(g *tg.UniqueGift) { g.Number = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := b65iGift()
			tc.edit(g)
			f := &b65iFake{script: []b65iAnswer{{gift: g}}}
			res, err := b65iExec(t, &GetUniqueGiftStep{GiftName: "PlushPepe-1"}, f)
			assert.Nil(t, res)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.field)
			assert.Equal(t, []string{"GetUniqueGift(PlushPepe-1)"}, f.calls)
		})
	}
}

// E-ug-want: the refusal row matches its own WantError as a substring, on a 400 API error only.
func TestB65I_UniqueGiftRefusal(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		f := &b65iFake{script: []b65iAnswer{{err: b65iAPI(400, "Bad Request: gift not found")}}}
		res, err := b65iExec(t, &GetUniqueGiftStep{GiftName: "zzNoSuchGift-1", WantError: "gift not found"}, f)
		require.NoError(t, err)
		assert.Equal(t, &StepResult{StepName: "getUniqueGift", Method: "getUniqueGift",
			Evidence: map[string]any{"error_code": 400, "description": "Bad Request: gift not found"}}, res)
		assert.Equal(t, []string{"GetUniqueGift(zzNoSuchGift-1)"}, f.calls)
	})
	for _, tc := range []struct {
		name, want string
		err        error
		reason     string // "" = no error
	}{
		{"suffix", "gift not found", b65iAPI(400, "Bad Request: gift not found."), ""},
		{"other_text", "gift not found", b65iAPI(400, "Bad Request: x"), "does not contain"},
		{"code_404", "gift not found", b65iAPI(404, "Bad Request: gift not found"), "code 404"},
		{"not_api", "gift not found", errors.New("Bad Request: gift not found"), "API refusal"},
		{"success", "gift not found", nil, "the server took the call"},
		{"own_phrase", "zz other phrase", b65iAPI(400, "Bad Request: zz other phrase"), ""},
		{"own_phrase_other_text", "zz other phrase", b65iAPI(400, "Bad Request: gift not found"), "does not contain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := b65iAnswer{err: tc.err}
			if tc.err == nil {
				a.gift = b65iGift()
			}
			f := &b65iFake{script: []b65iAnswer{a}}
			res, err := b65iExec(t, &GetUniqueGiftStep{GiftName: "zzNoSuchGift-1", WantError: tc.want}, f)
			assert.Equal(t, []string{"GetUniqueGift(zzNoSuchGift-1)"}, f.calls)
			if tc.reason == "" {
				require.NoError(t, err)
				require.NotNil(t, res)
				return
			}
			assert.Nil(t, res)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.reason)
		})
	}
}

// E-ug-err and E-ug-nilresult: a sender error is returned; a nil result is an error, not a panic.
func TestB65I_UniqueGiftErrors(t *testing.T) {
	cause := errors.New("cause")
	f := &b65iFake{script: []b65iAnswer{{err: cause}}}
	res, err := b65iExec(t, &GetUniqueGiftStep{GiftName: "PlushPepe-1"}, f)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, cause)

	f = &b65iFake{script: []b65iAnswer{{}}}
	res, err = b65iExec(t, &GetUniqueGiftStep{GiftName: "PlushPepe-1"}, f)
	assert.Nil(t, res)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no result")
}

// b65iRun runs one step in a real runner that repeats a step once on a 429 (MaxMessages 10: 0 stops the run).
func b65iRun(f *b65iFake, step Step) *ScenarioResult {
	rt := NewRuntime(f, testutil.TestChatID, 0)
	runner := NewRunner(rt, RunnerConfig{RetryOn429: true, Max429Retries: 1, MaxMessages: 10}, slog.New(slog.DiscardHandler))
	return runner.Run(context.Background(), &BaseScenario{ScenarioName: "b65i", ScenarioSteps: []Step{step},
		ScenarioTimeout: 10 * time.Second})
}

// E-ug-429 and E-ug-429-want: a 429 in either mode is repeated by the runner (the step keeps it with %w).
func TestB65I_UniqueGift429(t *testing.T) {
	f := &b65iFake{script: []b65iAnswer{{err: b65i429()}, {gift: b65iGift()}}}
	res := b65iRun(f, &GetUniqueGiftStep{GiftName: "PlushPepe-1"})
	assert.True(t, res.Success, "error: %s", res.Error)
	assert.Equal(t, []string{"GetUniqueGift(PlushPepe-1)", "GetUniqueGift(PlushPepe-1)"}, f.calls)

	f = &b65iFake{script: []b65iAnswer{{err: b65i429()}, {err: b65iAPI(400, "Bad Request: gift not found")}}}
	res = b65iRun(f, &GetUniqueGiftStep{GiftName: "zzNoSuchGift-1", WantError: "gift not found"})
	assert.True(t, res.Success, "error: %s", res.Error)
	assert.Equal(t, []string{"GetUniqueGift(zzNoSuchGift-1)", "GetUniqueGift(zzNoSuchGift-1)"}, f.calls)
}

// E-del-ok: the guard is refused as measured, then the delete; the whole result.
func TestB65I_DeleteOK(t *testing.T) {
	f := &b65iFake{script: []b65iAnswer{{err: b65iAPI(400, b65iMT)}, {err: b65iAPI(400, b65iMT)}}}
	res, err := b65iExec(t, &DeleteBusinessAccountGiftStep{}, f)
	require.NoError(t, err)
	assert.Equal(t, &StepResult{StepName: "deleteBusinessAccountGift", Method: "deleteBusinessAccountGift",
		Evidence: map[string]any{"guard_error_code": 400, "guard_description": b65iMT, "error_code": 400, "description": b65iMT}}, res)
	assert.Equal(t, []string{b65iGuardCall, b65iDeleteCall}, f.calls)
}

// E-del-guard-found and E-del-guard-nil: a connection (or a nil error) stops the step before the delete, with a
// fixed error that names no id.
func TestB65I_DeleteGuardFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		conn *tg.BusinessConnection
	}{
		{"found", &tg.BusinessConnection{ID: "bc-seen-9191", User: tg.User{ID: 4242, FirstName: "u"}, UserChatID: 4343}},
		{"nil_nil", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &b65iFake{script: []b65iAnswer{{conn: tc.conn}}}
			res, err := b65iExec(t, &DeleteBusinessAccountGiftStep{}, f)
			assert.Nil(t, res)
			require.EqualError(t, err, b65iGuardErr)
			for _, id := range []string{"bc-seen-9191", "4242", "4343"} {
				assert.NotContains(t, err.Error(), id)
			}
			assert.Equal(t, []string{b65iGuardCall}, f.calls, "no delete")
		})
	}
}

// E-del-guard-other: any guard answer but the exact measured 400 stops the step before the delete.
func TestB65I_DeleteGuardOther(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		err          error
	}{
		{"suffix", "guard", b65iAPI(400, b65iMT+" check is unavailable")},
		{"other_text", "guard", b65iAPI(400, "Bad Request: x")},
		{"code_403", "guard", b65iAPI(403, b65iMT)},
		{"not_api", "guard", errors.New(b65iMT)},
		{"code_500", "guard", b65iAPI(500, b65iMT)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &b65iFake{script: []b65iAnswer{{err: tc.err}}}
			res, err := b65iExec(t, &DeleteBusinessAccountGiftStep{}, f)
			assert.Nil(t, res)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.reason)
			assert.NotEqual(t, b65iGuardErr, err.Error())
			assert.Equal(t, []string{b65iGuardCall}, f.calls, "no delete")
		})
	}
}

// E-del-took and E-del-other: the delete must be refused with the exact measured 400.
func TestB65I_DeleteOther(t *testing.T) {
	for _, tc := range []struct {
		name, reason string
		err          error
	}{
		{"took", "the server took the call", nil},
		{"suffix", "refusal", b65iAPI(400, b65iMT+" check is unavailable")},
		{"code_403", "refusal", b65iAPI(403, b65iMT)},
		{"code_500", "refusal", b65iAPI(500, b65iMT)},
		{"other_text", "refusal", b65iAPI(400, "Bad Request: x")},
		{"not_found_404", "refusal", b65iAPI(404, "Not Found")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &b65iFake{script: []b65iAnswer{{err: b65iAPI(400, b65iMT)}, {err: tc.err}}}
			res, err := b65iExec(t, &DeleteBusinessAccountGiftStep{}, f)
			assert.Nil(t, res)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.reason)
			assert.NotContains(t, err.Error(), "guard")
			assert.Equal(t, []string{b65iGuardCall, b65iDeleteCall}, f.calls)
		})
	}
}

// E-del-429-guard and E-del-429-delete: a 429 from either call repeats the whole step, guard first.
func TestB65I_Delete429(t *testing.T) {
	f := &b65iFake{script: []b65iAnswer{{err: b65i429()}, {err: b65iAPI(400, b65iMT)}, {err: b65iAPI(400, b65iMT)}}}
	res := b65iRun(f, &DeleteBusinessAccountGiftStep{})
	assert.True(t, res.Success, "error: %s", res.Error)
	assert.Equal(t, []string{b65iGuardCall, b65iGuardCall, b65iDeleteCall}, f.calls)

	f = &b65iFake{script: []b65iAnswer{{err: b65iAPI(400, b65iMT)}, {err: b65i429()}, {conn: &tg.BusinessConnection{ID: "bc-seen-9191"}}}}
	res = b65iRun(f, &DeleteBusinessAccountGiftStep{})
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, b65iGuardErr)
	assert.Equal(t, []string{b65iGuardCall, b65iDeleteCall, b65iGuardCall}, f.calls, "no second delete")
}

// E-adapter: the real adapter returns the sender's values and errors and passes the caller's context; a canceled
// context sends no request.
func TestB65I_Adapter(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getUniqueGift", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyOK(w, json.RawMessage(`{"gift_id":"g1","base_name":"Plush Pepe","name":"PlushPepe-1","number":1}`))
	})
	for _, m := range []string{"getBusinessConnection", "deleteBusinessAccountGift"} {
		server.On("/bot"+testutil.TestToken+"/"+m, func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyBadRequest(w, "business connection not found")
		})
	}
	a := NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL()))
	ctx := context.Background()
	gift, err := a.GetUniqueGift(ctx, "PlushPepe-1")
	require.NoError(t, err)
	assert.Equal(t, b65iGift(), gift)
	conn, err := a.GetBusinessConnection(ctx, "zzNoSuchConnection")
	assert.Nil(t, conn)
	var apiErr *tg.APIError
	require.True(t, errors.As(err, &apiErr), "error %v", err)
	assert.Equal(t, 400, apiErr.Code)
	assert.Equal(t, b65iMT, apiErr.Description)
	err = a.DeleteBusinessAccountGift(ctx, sender.DeleteBusinessAccountGiftRequest{BusinessConnectionID: "zzNoSuchConnection", OwnedGiftID: "zzNoSuchGift"})
	require.True(t, errors.As(err, &apiErr), "error %v", err)
	assert.Equal(t, b65iMT, apiErr.Description)
	require.Equal(t, 3, server.CaptureCount())
	assert.True(t, strings.HasSuffix(server.CaptureAt(1).Path, "/getBusinessConnection"))

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = a.GetUniqueGift(canceled, "PlushPepe-1")
	assert.ErrorIs(t, err, context.Canceled)
	_, err = a.GetBusinessConnection(canceled, "zzNoSuchConnection")
	assert.ErrorIs(t, err, context.Canceled)
	err = a.DeleteBusinessAccountGift(canceled, sender.DeleteBusinessAccountGiftRequest{BusinessConnectionID: "zzNoSuchConnection", OwnedGiftID: "zzNoSuchGift"})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 3, server.CaptureCount(), "the canceled calls send nothing")
}

// E-guard-found-http (change round c2, O-C2-01): through the real adapter and a mock server, a found connection stops
// the step with the fixed guard error, and only the lookup is sent (no delete).
func TestB65I_DeleteGuardFoundHTTP(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/getBusinessConnection", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyOK(w, json.RawMessage(`{"id":"bc-seen-9191","user":{"id":1,"is_bot":false,"first_name":"A"},"user_chat_id":1,"date":1,"rights":{},"is_enabled":true}`))
	})
	server.On("/bot"+testutil.TestToken+"/deleteBusinessAccountGift", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyOK(w, true)
	})
	rt := NewRuntime(NewSenderAdapter(testutil.NewTestClient(t, server.BaseURL())), testutil.TestChatID, 0)
	runner := NewRunner(rt, RunnerConfig{RetryOn429: true, Max429Retries: 1, MaxMessages: 10}, slog.New(slog.DiscardHandler))
	res := runner.Run(context.Background(), &BaseScenario{ScenarioName: "b65i", ScenarioSteps: []Step{&DeleteBusinessAccountGiftStep{}},
		ScenarioTimeout: 10 * time.Second})
	assert.False(t, res.Success)
	assert.Contains(t, res.Error, b65iGuardErr)
	assert.NotContains(t, res.Error, "bc-seen-9191")
	require.Equal(t, 1, server.CaptureCount(), "only the lookup is sent")
	assert.True(t, strings.HasSuffix(server.CaptureAt(0).Path, "/getBusinessConnection"))
}

// E-names.
func TestB65I_StepNames(t *testing.T) {
	assert.Equal(t, "getUniqueGift", (&GetUniqueGiftStep{}).Name())
	assert.Equal(t, "deleteBusinessAccountGift", (&DeleteBusinessAccountGiftStep{}).Name())
}
