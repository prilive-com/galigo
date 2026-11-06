package tg_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.1 part a: links in poll options on the read side (B-25 10.1a, TEST-PLAN §2, rows T-g-1, T-g-2).
//
// Live fixtures: the "options" arrays of the sendPoll replies of 2026-11-06 (E10/lookups/probe101a/01, 02, 09 and
// probe101a-form/01). The only edit: every file_id / file_unique_id -> a label (PH90, PH100). Everything else
// (texts, URLs, sizes, key order) is byte-exact. The server adds a trailing "/" to a bare host and drops a fragment
// (probe 01 sent "https://example.com", probe 02 "https://example.com/a?b=1#c").
const (
	api101aProbe01 = `[{"persistent_id":"0","text":"l","media":{"link":{"url":"https://example.com/"}},"voter_count":0},{"persistent_id":"1","text":"b","voter_count":0}]`
	api101aProbe02 = `[{"persistent_id":"0","text":"l","media":{"link":{"url":"https://example.com/a?b=1"}},"voter_count":0},{"persistent_id":"1","text":"p","media":{"photo":[{"file_id":"PH90-file","file_unique_id":"PH90-uniq","file_size":341,"width":90,"height":90},{"file_id":"PH100-file","file_unique_id":"PH100-uniq","file_size":361,"width":100,"height":100}]},"voter_count":0}]`
	api101aProbe09 = `[{"persistent_id":"0","text":"1","media":{"link":{"url":"https://example.com/1"}},"voter_count":0},{"persistent_id":"1","text":"2","media":{"link":{"url":"https://example.com/2"}},"voter_count":0},{"persistent_id":"2","text":"3","voter_count":0}]`
	// probe101a-form/01 (multipart without files, the transport galigo uses): the same options as probe 09.
	api101aForm01 = `[{"persistent_id":"0","text":"1","media":{"link":{"url":"https://example.com/1"}},"voter_count":0},{"persistent_id":"1","text":"2","media":{"link":{"url":"https://example.com/2"}},"voter_count":0},{"persistent_id":"2","text":"3","voter_count":0}]`
)

// T-g-1: the probe replies decode strictly (no unknown key) into PollOption with Media.Link.URL.
func TestAPI101A_DecodeProbe(t *testing.T) {
	link := func(u string) *tg.PollMedia { return &tg.PollMedia{Link: &tg.Link{URL: u}} }
	twoLinks := []tg.PollOption{
		{PersistentID: "0", Text: "1", Media: link("https://example.com/1")},
		{PersistentID: "1", Text: "2", Media: link("https://example.com/2")},
		{PersistentID: "2", Text: "3"},
	}
	for _, tc := range []struct {
		name, raw string
		want      []tg.PollOption
	}{
		{"p01", api101aProbe01, []tg.PollOption{
			{PersistentID: "0", Text: "l", Media: link("https://example.com/")},
			{PersistentID: "1", Text: "b"},
		}},
		{"p02", api101aProbe02, []tg.PollOption{
			{PersistentID: "0", Text: "l", Media: link("https://example.com/a?b=1")},
			{PersistentID: "1", Text: "p", Media: &tg.PollMedia{Photo: []tg.PhotoSize{
				{FileID: "PH90-file", FileUniqueID: "PH90-uniq", FileSize: 341, Width: 90, Height: 90},
				{FileID: "PH100-file", FileUniqueID: "PH100-uniq", FileSize: 361, Width: 100, Height: 100},
			}}},
		}},
		{"p09", api101aProbe09, twoLinks},
		{"form01", api101aForm01, twoLinks},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dec := json.NewDecoder(bytes.NewReader([]byte(tc.raw)))
			dec.DisallowUnknownFields()
			var got []tg.PollOption
			require.NoError(t, dec.Decode(&got))
			assert.Equal(t, tc.want, got)
		})
	}
}

// T-g-2: PollMedia.Link and Link.URL encode with the spec's keys; an absent link is omitted, an empty URL is kept.
func TestAPI101A_EncodeKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want string
	}{
		{"link", tg.PollMedia{Link: &tg.Link{URL: "u"}}, `{"link":{"url":"u"}}`},
		{"no_link", tg.PollMedia{}, `{}`},
		{"empty_url", tg.Link{}, `{"url":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.v)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(b))
		})
	}
}

// T-g-3 (T1-09, T2-10 openai): tg.Link has exactly the spec's one field and no method (apidiff reports a new type as a whole, so an extra
// exported field would not show there).
func TestAPI101A_LinkShape(t *testing.T) {
	rt := reflect.TypeOf(tg.Link{})
	require.Equal(t, 1, rt.NumField())
	f := rt.Field(0)
	assert.Equal(t, "URL", f.Name)
	assert.Equal(t, reflect.TypeOf(""), f.Type)
	assert.Equal(t, reflect.StructTag(`json:"url"`), f.Tag)
	// T2-10 openai: and no method, on the value or the pointer (apidiff does not list a new type's methods either).
	assert.Equal(t, 0, rt.NumMethod())
	assert.Equal(t, 0, reflect.PointerTo(rt).NumMethod())
}
