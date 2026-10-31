package scrub_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/scrub"
	"github.com/prilive-com/galigo/tg"
)

func TestTokenFromError_NilError(t *testing.T) {
	result := scrub.TokenFromError(nil, tg.SecretToken("123:ABC"))
	assert.Nil(t, result)
}

func TestTokenFromError_EmptyToken(t *testing.T) {
	original := errors.New("some error")
	result := scrub.TokenFromError(original, tg.SecretToken(""))
	assert.Equal(t, original, result)
}

func TestTokenFromError_NoTokenInMessage(t *testing.T) {
	original := errors.New("connection refused")
	result := scrub.TokenFromError(original, tg.SecretToken("123:ABC"))
	assert.Equal(t, original, result)
}

func TestTokenFromError_ScrubsToken(t *testing.T) {
	token := tg.SecretToken("123456:ABCdef")
	original := fmt.Errorf("Post https://api.telegram.org/bot123456:ABCdef/sendMessage: dial tcp: no such host")
	result := scrub.TokenFromError(original, token)

	require.NotEqual(t, original, result)
	assert.Contains(t, result.Error(), "[REDACTED]")
	assert.NotContains(t, result.Error(), "123456:ABCdef")
}

func TestTokenFromError_PreservesErrorChain(t *testing.T) {
	token := tg.SecretToken("123456:ABCdef")
	netErr := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	wrapped := fmt.Errorf("Post https://api.telegram.org/bot123456:ABCdef/sendMessage: %w", netErr)

	result := scrub.TokenFromError(wrapped, token)

	// Original error chain is preserved via Unwrap
	var opErr *net.OpError
	assert.True(t, errors.As(result, &opErr))
}

// The rows below drive real errors from http.NewRequestWithContext and http.Client.Do, so every
// rendering of the token is the one the standard library produces.

const (
	secretPre  = "ABCdefGHI"
	secretPost = "jklMNOpqr"
	goodBase   = "https://api.telegram.org/bot"
	ctlBase    = "https://api.telegram.org/bot\x7f"
	noSchBase  = "://bot"
)

var errBoom = errors.New("boom")

type errKind int

const (
	kindParse errKind = iota
	kindDo
)

type countingTransport struct{ calls atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.calls.Add(1)
	return nil, errBoom
}

func shapedToken(x string) string { return "123456789:" + secretPre + x + secretPost }

// makeErr returns the error for base+tok used as a request URL. A kindDo fixture must pass request
// creation and reach the transport exactly once; a kindParse fixture must fail at request creation.
func makeErr(t *testing.T, base, tok string, kind errKind) error {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+tok+"/getUpdates?offset=0", nil)
	if kind == kindParse {
		require.Error(t, err, "fixture must fail at request creation")
		return err
	}
	require.NoError(t, err, "fixture must reach Do")
	rt := &countingTransport{}
	_, err = (&http.Client{Transport: rt}).Do(req) //nolint:bodyclose // the transport never returns a response
	require.Error(t, err)
	require.Equal(t, int32(1), rt.calls.Load())
	return err
}

func requireScrubbed(t *testing.T, err error, tok string, kind errKind, secrets ...string) string {
	t.Helper()
	got := scrub.TokenFromError(err, tg.SecretToken(tok))
	require.Error(t, got)
	msg := got.Error()
	for _, s := range secrets {
		assert.NotContains(t, msg, s)
	}
	assert.Contains(t, msg, "[REDACTED]")
	var urlErr *url.Error
	assert.True(t, errors.As(got, &urlErr), "error chain must keep *url.Error")
	if kind == kindDo {
		assert.ErrorIs(t, got, errBoom)
	}
	return msg
}

func TestTokenFromError_EscapedForms(t *testing.T) {
	doForms := []string{" ", "\"", "\\", "é", "%41 ", "%2f ", " ?x", "?\"", "?\\"}
	for _, x := range doForms {
		t.Run("do/"+strconv.Quote(x), func(t *testing.T) {
			tok := shapedToken(x)
			requireScrubbed(t, makeErr(t, goodBase, tok, kindDo), tok, kindDo, secretPre, secretPost)
		})
	}
	for _, x := range append(doForms, "\r", "\x7f", "\x00") {
		t.Run("parse/"+strconv.Quote(x), func(t *testing.T) {
			tok := shapedToken(x)
			requireScrubbed(t, makeErr(t, ctlBase, tok, kindParse), tok, kindParse, secretPre, secretPost)
		})
	}
}

func TestTokenFromError_FragmentForms(t *testing.T) {
	for _, x := range []string{"#", "#é", "#A B", "#\x7f"} {
		t.Run("do/"+strconv.Quote(x), func(t *testing.T) {
			tok := shapedToken(x)
			requireScrubbed(t, makeErr(t, goodBase, tok, kindDo), tok, kindDo, secretPre, secretPost)
		})
		t.Run("parse/"+strconv.Quote(x), func(t *testing.T) {
			tok := shapedToken(x)
			requireScrubbed(t, makeErr(t, noSchBase, tok, kindParse), tok, kindParse, secretPre, secretPost)
		})
	}
}

func TestTokenFromError_AllBytes(t *testing.T) {
	ctl := func(b byte) bool { return b < 0x20 || b == 0x7f }
	wantParseOnly := map[string]func(b byte) bool{
		"":    func(b byte) bool { return ctl(b) || b == '%' },
		"%41": func(b byte) bool { return ctl(b) || b == '%' },
		"?":   ctl,
		"#":   func(b byte) bool { return b == '%' },
	}
	for prefix, parseOnly := range wantParseOnly {
		for b := range 256 {
			tok := shapedToken(prefix + string([]byte{byte(b)}))
			msg := scrub.TokenFromError(makeErr(t, noSchBase, tok, kindParse), tg.SecretToken(tok)).Error()
			assert.NotContains(t, msg, secretPre, "parse prefix %q byte %#x", prefix, b)
			assert.NotContains(t, msg, secretPost, "parse prefix %q byte %#x", prefix, b)

			_, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, goodBase+tok+"/getUpdates?offset=0", nil)
			require.Equal(t, parseOnly(byte(b)), reqErr != nil, "parse-only set: prefix %q byte %#x", prefix, b)
			if reqErr != nil {
				continue
			}
			msg = scrub.TokenFromError(makeErr(t, goodBase, tok, kindDo), tg.SecretToken(tok)).Error()
			assert.NotContains(t, msg, secretPre, "do prefix %q byte %#x", prefix, b)
			assert.NotContains(t, msg, secretPost, "do prefix %q byte %#x", prefix, b)
		}
	}
}

func TestTokenFromError_NoPathBase(t *testing.T) {
	tok := "123456789:" + secretPre + "STUVWXYZabcdefgh" + secretPost
	for _, base := range []string{"https://", "https://api.telegram.org", "https://bot"} {
		t.Run(base, func(t *testing.T) {
			msg := requireScrubbed(t, makeErr(t, base, tok, kindParse), tok, kindParse, secretPre, secretPost)
			assert.Contains(t, msg, `invalid port "[REDACTED]"`)
		})
	}
}

func TestTokenFromError_ContextRenderings(t *testing.T) {
	for _, tc := range []struct{ base, x string }{
		{"http://h/bot ", "%41"},
		{"http://h/boté", "%41"},
		{"http://h/bot#", "?é"},
		{"http://h/bot#", "#é"},
	} {
		t.Run(tc.base+tc.x, func(t *testing.T) {
			tok := shapedToken(tc.x)
			requireScrubbed(t, makeErr(t, tc.base, tok, kindDo), tok, kindDo, secretPre, secretPost)
		})
	}
}

func TestTokenFromError_NormalTokenShape(t *testing.T) {
	tok := "123456789:" + secretPre + "STUVWXYZabcdefgh" + secretPost
	got := scrub.TokenFromError(makeErr(t, goodBase, tok, kindDo), tg.SecretToken(tok))
	assert.Equal(t, `Get "https://api.telegram.org/bot[REDACTED]/getUpdates?offset=0": boom`, got.Error())
}

func TestTokenFromError_TokenStartsWithHash(t *testing.T) {
	tok := "#" + secretPre + secretPost
	original := makeErr(t, noSchBase, tok, kindParse)
	got := scrub.TokenFromError(original, tg.SecretToken(tok))
	assert.Same(t, original, got, "no token piece in the text: same error value")
}

func TestTokenFromError_ShortRuns(t *testing.T) {
	pieces := []string{"Zq", "Wv", "Kj", "Yp"}
	for _, tc := range []struct {
		base, tok string
		kind      errKind
	}{
		{goodBase, "123456789:Qx Zq\"Wv\rKj éYp", kindParse},
		{"http://h/bot#", "123456789:Qx Zq\"Wv\rKj éYp", kindDo},
		{goodBase, "123456789:Qx%41 Zq", kindDo},
		{goodBase, "123456789:Qx Zq#Wv Kj", kindDo},
		{goodBase, "123456789:Qx?Zq\\Wv", kindDo},
		{"http://h/bot ", "123456789:Qx%41 Zq?Wv Kj", kindDo},
		{noSchBase, "123456789:Qx%41 Zq", kindParse},
		{noSchBase, "123456789:Qx Zq#Wv Kj", kindParse},
		{noSchBase, "123456789:Qx?Zq\\Wv", kindParse},
		{noSchBase, "123456789:Qx%41 Zq?Wv Kj", kindParse},
	} {
		t.Run(tc.base+strconv.Quote(tc.tok), func(t *testing.T) {
			var present []string
			for _, p := range pieces {
				if strings.Contains(tc.tok, p) {
					present = append(present, p)
				}
			}
			requireScrubbed(t, makeErr(t, tc.base, tc.tok, tc.kind), tc.tok, tc.kind, present...)
		})
	}
}

func TestTokenFromError_DecodedPercent(t *testing.T) {
	for _, tok := range []string{
		"123456789:%41%42%43%44%45%46%47%48%49",
		"123456789:Qx%41%42%43%44Zq",
		"123456789:%41%42%43%44?x=%zz",
		"123456789:%41%42%43%44?x=%",
	} {
		for _, base := range []string{"http://h/bot ", goodBase} {
			t.Run(base+tok, func(t *testing.T) {
				requireScrubbed(t, makeErr(t, base, tok, kindDo), tok, kindDo, "ABCD", "EFGHI", "Zq")
			})
		}
	}
}

func TestTokenFromError_PieceBoundary(t *testing.T) {
	t.Run("4-byte piece", func(t *testing.T) {
		tok := "ABCD#efghijklmnopqrstuvwxyz0123456789"
		got := scrub.TokenFromError(makeErr(t, noSchBase, tok, kindParse), tg.SecretToken(tok))
		assert.Equal(t, `parse "://bot[REDACTED]": missing protocol scheme`, got.Error())
	})
	t.Run("5-byte piece", func(t *testing.T) {
		tok := "123456789:ABCD/efghijklmnopqrstuvwxyz0123456789"
		msg := requireScrubbed(t, makeErr(t, "https://", tok, kindParse), tok, kindParse, ":ABCD")
		assert.Contains(t, msg, `invalid port "[REDACTED]"`)
	})
	t.Run("3-byte piece is kept", func(t *testing.T) {
		original := errors.New("x ABC y")
		assert.Same(t, original, scrub.TokenFromError(original, tg.SecretToken("ABCDEFGHIJ")))
	})
}

func TestTokenFromError_LongInputIsLinear(t *testing.T) {
	for _, tc := range []struct{ name, msg, tok string }{
		{"long token", "aaa" + strings.Repeat("b", 1<<16), strings.Repeat("a", 1<<18) + "Z"},
		{"long message", strings.Repeat("x", 1<<20), "123456789:" + secretPre + "STUVWXYZabcdefgh" + secretPost},
		{"long message and long token", strings.Repeat("b", 1<<20), strings.Repeat("a", 1<<20) + "Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			_ = scrub.TokenFromError(errors.New(tc.msg), tg.SecretToken(tc.tok))
			assert.Less(t, time.Since(start), 2*time.Second)
		})
	}
}

func TestTokenFromError_ShortToken(t *testing.T) {
	for _, tok := range []string{"A", "AB", "ABC"} {
		t.Run(tok, func(t *testing.T) {
			original := errors.New("x " + tok + " y")
			got := scrub.TokenFromError(original, tg.SecretToken(tok))
			msg := got.Error()
			assert.NotContains(t, strings.ReplaceAll(msg, "[REDACTED]", ""), tok)
			assert.Contains(t, msg, "[REDACTED]")
			assert.ErrorIs(t, got, original)
		})
	}
}

func TestTokenFromError_CanonicalRenderings(t *testing.T) {
	for _, tc := range []struct{ base, tok, secret string }{
		{goodBase, "123456789:" + strings.Repeat("%3f", 8) + " ", "%3F%3F"},
		{"http://h/bot#", "123456789:" + strings.Repeat("%3f", 8) + " ", "????"},
		{"http://h/bot#", shapedToken("\t?\xd0(\x7f!"), "?%D0("},
		{goodBase, shapedToken("#? ? ? ?"), "%20?%20"},
		{goodBase, shapedToken("#?\"?\"?\"?"), "?%22?"},
		{goodBase, "123456789:#?\x01?\x01?\x01?\x01?", "%01?%01?"},
		{"http://", "123456789" + strings.Repeat("%3a", 8) + "@h", "%3A%3A"},
		{goodBase, "123456789:" + strings.Repeat("@%3f", 4) + " ", "@%3F@"},
		{goodBase, "123456789:?x=%zz#%41%42%43%44 ", "ABCD"},
		{"http://h/bot ", "123456789:%4a%4b%4c%4d", "JKLM"},
	} {
		t.Run(tc.base+tc.tok, func(t *testing.T) {
			requireScrubbed(t, makeErr(t, tc.base, tc.tok, kindDo), tc.tok, kindDo, tc.secret)
		})
	}
}

func TestTokenFromError_DecodedHost(t *testing.T) {
	// A bracketed host is decoded before netip.ParseAddr, which prints the decoded text.
	for _, tc := range []struct{ tok, secret string }{
		{"123456789:%c3%a9%c3%a9]", "éé"},
		{"123456789:%ab%ab%ab%ab]", `\xab\xab\xab\xab`},
	} {
		for _, base := range []string{"http://[", "http://[::1]:"} {
			t.Run(base+tc.tok, func(t *testing.T) {
				requireScrubbed(t, makeErr(t, base, tc.tok, kindParse), tc.tok, kindParse, tc.secret)
			})
		}
	}
}

func TestTokenFromError_HostRendering(t *testing.T) {
	// URL.String escapes a host differently from a path, fragment or userinfo: "<" stays, 0xab becomes %AB.
	tok := "123456789:u@h<\xab<CD"
	msg := requireScrubbed(t, makeErr(t, "http://", tok, kindDo), tok, kindDo, "%AB<CD", "h<%AB")
	assert.NotContains(t, msg, "<CD")

	// The host reference must render the decoded token: %ab etc. are decoded, then escaped again as %AB.
	tok = "123456789:u@%ab<%cd<%ef<%ab<CD"
	requireScrubbed(t, makeErr(t, "http://", tok, kindDo), tok, kindDo, "<%CD<%EF")
}

func TestTokenFromError_TrailingPercent(t *testing.T) {
	// A token may end in a cut-off escape; decoding must not read past the end.
	for _, tok := range []string{"123456789:" + secretPre + "%4", "123456789:" + secretPre + "%"} {
		t.Run(tok, func(t *testing.T) {
			got := scrub.TokenFromError(errors.New("failure "+tok+" end"), tg.SecretToken(tok))
			assert.Equal(t, "failure [REDACTED] end", got.Error())
		})
	}
}

func TestTokenFromError_LastWindow(t *testing.T) {
	tok := tg.SecretToken("123456789:" + secretPre + "STUVWXYZabcdefgh" + secretPost)
	assert.Equal(t, "failure [REDACTED]", scrub.TokenFromError(errors.New("failure WXYZ"), tok).Error())
	assert.Equal(t, "[REDACTED]", scrub.TokenFromError(errors.New("WXYZ"), tok).Error())
}
