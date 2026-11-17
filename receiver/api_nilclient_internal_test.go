package receiver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Fake token in the real token shape.
const nilClientToken = "123456789:AAFakeNilClientTokenForTestsOnly0"

var errNilClientStub = errors.New("stub transport down")

type nilClientReq struct {
	method      string
	url         string
	contentType string
	body        []byte
	deadline    time.Time
	hasDeadline bool
}

// nilClientStub is the transport of the client the tests install as
// defaultAPIClient. It records every request and never opens a socket.
type nilClientStub struct {
	status int
	body   string
	err    error

	mu   sync.Mutex
	reqs []nilClientReq
}

func (s *nilClientStub) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := nilClientReq{
		method:      req.Method,
		url:         req.URL.String(),
		contentType: req.Header.Get("Content-Type"),
	}
	rec.deadline, rec.hasDeadline = req.Context().Deadline()
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		rec.body = b
	}
	s.mu.Lock()
	s.reqs = append(s.reqs, rec)
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return &http.Response{
		StatusCode: s.status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Request:    req,
	}, nil
}

func (s *nilClientStub) requests() []nilClientReq {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]nilClientReq(nil), s.reqs...)
}

// useNilClientStub replaces the package variable defaultAPIClient with a
// client that sends through s and has the timeout want, and restores the
// old value when the test ends. The swap is not synchronized: do not use it
// from a parallel test, or while another goroutine may call SetWebhook,
// DeleteWebhook or GetWebhookInfo with a nil client.
func useNilClientStub(t *testing.T, s *nilClientStub, want time.Duration) {
	t.Helper()
	old := defaultAPIClient
	defaultAPIClient = &http.Client{Transport: s, Timeout: want}
	t.Cleanup(func() { defaultAPIClient = old })
}

// requireInstalledDeadline checks that the request carried the deadline that
// http.Client sets from its Timeout: Do computes time.Now().Add(Timeout), so
// the deadline lies between before+want and after+want.
func requireInstalledDeadline(t *testing.T, rec nilClientReq, want time.Duration, before, after time.Time) {
	t.Helper()
	require.True(t, rec.hasDeadline, "request has no deadline: the installed client's Timeout was not used")
	lo, hi := before.Add(want), after.Add(want)
	require.False(t, rec.deadline.Before(lo), "deadline %v is before %v: timeout shorter than %v", rec.deadline, lo, want)
	require.False(t, rec.deadline.After(hi), "deadline %v is after %v: timeout longer than %v", rec.deadline, hi, want)
}

// requireNilClientScrubbed compares the whole error text with want, so no
// form of the token (a piece, an escaped copy) can hide next to [REDACTED].
func requireNilClientScrubbed(t *testing.T, err error, want string) {
	t.Helper()
	require.Error(t, err)
	assert.ErrorIs(t, err, errNilClientStub)
	assert.Equal(t, want, err.Error())
}

// TestNilClient_UsesDefaultAPIClient checks that SetWebhook, DeleteWebhook and
// GetWebhookInfo send a nil client's request through defaultAPIClient (its
// transport and its timeout) and return what the server answered. No row opens
// a socket.
func TestNilClient_UsesDefaultAPIClient(t *testing.T) {
	const (
		okWant  = time.Minute
		errWant = 90 * time.Second
		hookURL = "https://example.com/hook"
		apiBase = "https://api.telegram.org/bot" + nilClientToken
	)
	tok := tg.SecretToken(nilClientToken)
	okBody := `{"ok":true,"result":true}`
	infoBody := `{"ok":true,"result":{"url":"` + hookURL + `","has_custom_certificate":true,` +
		`"pending_update_count":7,"ip_address":"192.0.2.1","last_error_date":1234,` +
		`"last_error_message":"reply failure","last_synchronization_error_date":5678,` +
		`"max_connections":40,"allowed_updates":["message","callback_query"]}}`
	wantInfo := &WebhookInfo{
		URL:                          hookURL,
		HasCustomCertificate:         true,
		PendingUpdateCount:           7,
		IPAddress:                    "192.0.2.1",
		LastErrorDate:                1234,
		LastErrorMessage:             "reply failure",
		LastSynchronizationErrorDate: 5678,
		MaxConnections:               40,
		AllowedUpdates:               []string{"message", "callback_query"},
	}
	const scrubbedBase = "https://api.telegram.org/bot[REDACTED]"

	original := defaultAPIClient

	tests := []struct {
		name   string
		stub   *nilClientStub
		want   time.Duration
		call   func() (*WebhookInfo, error)
		verify func(t *testing.T, info *WebhookInfo, err error, rec nilClientReq)
	}{
		{
			name: "R1 SetWebhook ok",
			stub: &nilClientStub{status: http.StatusOK, body: okBody},
			want: okWant,
			call: func() (*WebhookInfo, error) {
				return nil, SetWebhook(context.Background(), nil, tok, hookURL, "")
			},
			verify: func(t *testing.T, _ *WebhookInfo, err error, rec nilClientReq) {
				require.NoError(t, err)
				assert.Equal(t, http.MethodPost, rec.method)
				assert.Equal(t, apiBase+"/setWebhook", rec.url)
				assert.Equal(t, "application/json", rec.contentType)
				var got map[string]any
				require.NoError(t, json.Unmarshal(rec.body, &got))
				assert.Equal(t, map[string]any{"url": hookURL}, got)
			},
		},
		{
			name: "R2 DeleteWebhook ok",
			stub: &nilClientStub{status: http.StatusOK, body: okBody},
			want: okWant,
			call: func() (*WebhookInfo, error) {
				return nil, DeleteWebhook(context.Background(), nil, tok, false)
			},
			verify: func(t *testing.T, _ *WebhookInfo, err error, rec nilClientReq) {
				require.NoError(t, err)
				assert.Equal(t, http.MethodGet, rec.method)
				assert.Equal(t, apiBase+"/deleteWebhook?drop_pending_updates=false", rec.url)
				assert.Empty(t, rec.body)
			},
		},
		{
			name: "R3 GetWebhookInfo ok",
			stub: &nilClientStub{status: http.StatusOK, body: infoBody},
			want: okWant,
			call: func() (*WebhookInfo, error) {
				return GetWebhookInfo(context.Background(), nil, tok)
			},
			verify: func(t *testing.T, info *WebhookInfo, err error, rec nilClientReq) {
				require.NoError(t, err)
				assert.Equal(t, wantInfo, info)
				assert.Equal(t, http.MethodGet, rec.method)
				assert.Equal(t, apiBase+"/getWebhookInfo", rec.url)
				assert.Empty(t, rec.body)
			},
		},
		{
			name: "R4 SetWebhook transport error",
			stub: &nilClientStub{err: errNilClientStub},
			want: errWant,
			call: func() (*WebhookInfo, error) {
				return nil, SetWebhook(context.Background(), nil, tok, hookURL, "")
			},
			verify: func(t *testing.T, _ *WebhookInfo, err error, _ nilClientReq) {
				requireNilClientScrubbed(t, err,
					`request failed: Post "`+scrubbedBase+`/setWebhook": stub transport down`)
			},
		},
		{
			name: "R5 DeleteWebhook transport error",
			stub: &nilClientStub{err: errNilClientStub},
			want: errWant,
			call: func() (*WebhookInfo, error) {
				return nil, DeleteWebhook(context.Background(), nil, tok, false)
			},
			verify: func(t *testing.T, _ *WebhookInfo, err error, _ nilClientReq) {
				requireNilClientScrubbed(t, err,
					`request failed: Get "`+scrubbedBase+`/deleteWebhook?drop_pending_updates=false": stub transport down`)
			},
		},
		{
			name: "R6 GetWebhookInfo transport error",
			stub: &nilClientStub{err: errNilClientStub},
			want: errWant,
			call: func() (*WebhookInfo, error) {
				return GetWebhookInfo(context.Background(), nil, tok)
			},
			verify: func(t *testing.T, info *WebhookInfo, err error, _ nilClientReq) {
				requireNilClientScrubbed(t, err,
					`request failed: Get "`+scrubbedBase+`/getWebhookInfo": stub transport down`)
				assert.Nil(t, info)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			useNilClientStub(t, tc.stub, tc.want)
			before := time.Now()
			info, err := tc.call()
			after := time.Now()
			reqs := tc.stub.requests()
			require.Len(t, reqs, 1, "want exactly one request through the installed client")
			requireInstalledDeadline(t, reqs[0], tc.want, before, after)
			tc.verify(t, info, err, reqs[0])
		})
	}

	t.Run("R7 defaultAPIClient restored", func(t *testing.T) {
		assert.Same(t, original, defaultAPIClient)
	})
}
