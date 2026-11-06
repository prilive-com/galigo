package sender_test

// Bot API 10.1 part a (B-25 10.1a, TEST-PLAN §3, T-s-1 … T-s-6): a link as poll option media (InputMediaLink), on
// the wire as the Bot API server reads it (helpers of polls100d_test.go and wire_args_test.go).

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// T-s-1: the new API has the planned types (a wrong type does not compile).
func TestAPI101A_Shapes(t *testing.T) {
	hasType[func(string) sender.InputMedia](sender.NewInputMediaLink) // hasType: inputmedia100d_test.go
	var m sender.InputMedia
	hasType[string](m.URL)
	var p tg.PollMedia
	hasType[*tg.Link](p.Link)
	var l tg.Link
	hasType[string](l.URL)
}

// T-s-2: the constructor sets the type and the link URL, and nothing else.
func TestPolls101A_Constructor(t *testing.T) {
	assert.Equal(t, sender.InputMedia{Type: "link", URL: "https://e.x/l"}, sender.NewInputMediaLink("https://e.x/l"))
	assert.Equal(t, sender.InputMedia{Type: "link"}, sender.NewInputMediaLink(""))
	// T2-14, C1-04, C2-04 openai: no client normalisation (plan §4): query, fragment, letter case, percent escapes,
	// '+' and a missing scheme stay as given.
	assert.Equal(t, sender.InputMedia{Type: "link", URL: "https://e.x/a?b=1#c"}, sender.NewInputMediaLink("https://e.x/a?b=1#c"))
	assert.Equal(t, sender.InputMedia{Type: "link", URL: "https://E.x/Path?Key=Value#Frag"},
		sender.NewInputMediaLink("https://E.x/Path?Key=Value#Frag"))
	assert.Equal(t, sender.InputMedia{Type: "link", URL: "https://e.x/a%2Fb?key=A%2BB+Value"},
		sender.NewInputMediaLink("https://e.x/a%2Fb?key=A%2BB+Value"))
	assert.Equal(t, sender.InputMedia{Type: "link", URL: "example.com/x"}, sender.NewInputMediaLink("example.com/x"))
}

func link(u string) *sender.InputMedia { return mp(sender.NewInputMediaLink(u)) }

// T-s-3: link options between plain and upload options; the item is exactly {"type":"link","url":…} and takes no
// attach name.
func TestPolls101A_LinkOptions(t *testing.T) {
	full := sender.NewInputMediaLink("https://e.x/f")
	full.Caption, full.ParseMode, full.Emoji = "c", "HTML", "👍"
	full.Latitude, full.Longitude, full.HorizontalAccuracy = 1, 2, 3
	full.Title, full.Address, full.FoursquareID, full.GooglePlaceID = "T", "A", "f", "g"
	full.FoursquareType, full.GooglePlaceType = "ft", "gt" // T1-02 openai
	// T2-06 openai: the nested file metadata that sets no file (FileName, MediaType, Caption, ParseMode) is ignored too.
	full.Media.FileName, full.Media.MediaType, full.Media.Caption, full.Media.ParseMode = "n.bin", "photo", "mc", "MarkdownV2"
	rows := []struct {
		name    string
		options []sender.InputPollOption
		want    string
		files   map[string]wireFile
	}{
		{"links_only", []sender.InputPollOption{{Text: "l1", Media: link("https://e.x/1")}, {Text: "plain"},
			{Text: "l2", Media: link("https://e.x/2")}},
			`[{"text":"l1","media":{"type":"link","url":"https://e.x/1"}},{"text":"plain"},` +
				`{"text":"l2","media":{"type":"link","url":"https://e.x/2"}}]`, map[string]wireFile{}},
		{"link_then_upload", []sender.InputPollOption{{Text: "l", Media: link("https://e.x/l")},
			{Text: "u", Media: mp(sender.NewInputMediaPhoto(upload("d101-u", "u.jpg")))}},
			`[{"text":"l","media":{"type":"link","url":"https://e.x/l"}},` +
				`{"text":"u","media":{"type":"photo","media":"attach://file0"}}]`,
			map[string]wireFile{"file0": {FileName: "u.jpg", Data: []byte("d101-u")}}},
		{"upload_link_upload", []sender.InputPollOption{
			{Text: "u1", Media: mp(sender.NewInputMediaPhoto(upload("d101-1", "1.jpg")))},
			{Text: "l", Media: link("https://e.x/l")},
			{Text: "u2", Media: mp(sender.NewInputMediaPhoto(upload("d101-2", "2.jpg")))}},
			`[{"text":"u1","media":{"type":"photo","media":"attach://file0"}},` +
				`{"text":"l","media":{"type":"link","url":"https://e.x/l"}},` +
				`{"text":"u2","media":{"type":"photo","media":"attach://file1"}}]`,
			map[string]wireFile{"file0": {FileName: "1.jpg", Data: []byte("d101-1")},
				"file1": {FileName: "2.jpg", Data: []byte("d101-2")}}},
		{"empty_url", []sender.InputPollOption{{Text: "l", Media: link("")}, {Text: "b"}},
			`[{"text":"l","media":{"type":"link","url":""}},{"text":"b"}]`, map[string]wireFile{}},
		// T2-14, C1-04, C2-04 openai: the URL is sent byte for byte (no fragment or query cut, no scheme added, no
		// case change, no percent or '+' decoding; plan §4).
		{"url_as_given", []sender.InputPollOption{{Text: "f", Media: link("https://e.x/a?b=1#c")},
			{Text: "s", Media: link("example.com/x")}, {Text: "m", Media: link("https://E.x/Path?Key=Value#Frag")},
			{Text: "p", Media: link("https://e.x/a%2Fb?key=A%2BB+Value")}},
			`[{"text":"f","media":{"type":"link","url":"https://e.x/a?b=1#c"}},` +
				`{"text":"s","media":{"type":"link","url":"example.com/x"}},` +
				`{"text":"m","media":{"type":"link","url":"https://E.x/Path?Key=Value#Frag"}},` +
				`{"text":"p","media":{"type":"link","url":"https://e.x/a%2Fb?key=A%2BB+Value"}}]`, map[string]wireFile{}},
		{"other_fields_set", []sender.InputPollOption{{Text: "l", Media: &full}, {Text: "b"}},
			`[{"text":"l","media":{"type":"link","url":"https://e.x/f"}},{"text":"b"}]`, map[string]wireFile{}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{Options: r.options})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q", "options": pj(t, r.want)}, args)
			assertFiles(t, r.files, files)
		})
	}
	t.Run("upload_link_upload/part_order", func(t *testing.T) {
		assert.Equal(t, []string{"file0", "file1"}, polls100dPartOrder(t, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPoll(ctx, sender.SendPollRequest{ChatID: testutil.TestChatID, Question: "Q",
				Options: rows[2].options})
			return err
		}))
	})
}

// T-s-4: a link with any file set is an error before any request, in an option and as description media.
func TestPolls101A_LinkNoFile(t *testing.T) {
	photo := sender.FromFileID("Y")
	files := []struct {
		name string
		in   sender.InputFile
	}{
		{"file_id", sender.FromFileID("X")},
		{"url", sender.FromURL("https://e.x/f")},
		{"reader", sender.FromReader(strings.NewReader("r"), "r.bin")},
		{"source", sender.FromBytes([]byte("s"), "s.bin")},
		{"photo_only", sender.InputFile{Photo: &photo}},
	}
	send := func(t *testing.T, req sender.SendPollRequest, prefix string) {
		t.Helper()
		server := testutil.NewMockServer(t)
		client := testutil.NewTestClient(t, server.BaseURL())
		req.ChatID, req.Question = testutil.TestChatID, "Q"
		_, err := client.SendPoll(context.Background(), req)
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), prefix), err.Error())
		assert.True(t, strings.HasSuffix(err.Error(), "InputMedia: link has no media file"), err.Error())
		assert.Equal(t, 0, server.CaptureCount())
	}
	for _, f := range files {
		t.Run(f.name, func(t *testing.T) {
			m := sender.NewInputMediaLink("https://e.x/l")
			m.Media = f.in
			send(t, sender.SendPollRequest{Options: []sender.InputPollOption{{Text: "l", Media: &m}, {Text: "b"}}},
				"failed to build request: field options: ")
		})
	}
	t.Run("place_media", func(t *testing.T) {
		m := sender.NewInputMediaLink("https://e.x/l")
		m.Media = sender.FromFileID("X")
		send(t, sender.SendPollRequest{Options: plainOptions(), Media: &m}, "failed to build request: field media: ")
	})
}

// T-s-5: no client check of where a link may go (plan §4): as description or explanation media it is sent, and
// the server decides (probe 03, 04).
func TestPolls101A_LinkInOtherPlaces(t *testing.T) {
	item := `{"type":"link","url":"https://e.x/m"}`
	t.Run("media", func(t *testing.T) {
		mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{Options: plainOptions(),
			Media: link("https://e.x/m")})
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q", "options": pj(t, plainOptionsJSON),
			"media": pj(t, item)}, args)
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("explanation_media", func(t *testing.T) {
		mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{Options: plainOptions(),
			Type: "quiz", CorrectOptionIDs: []int{0}, Explanation: "e", ExplanationMedia: link("https://e.x/m")})
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q", "options": pj(t, plainOptionsJSON),
			"type": "quiz", "correct_option_ids": "[0]", "explanation": "e", "explanation_media": pj(t, item)}, args)
		assertFiles(t, map[string]wireFile{}, files)
	})
}

// T-s-6 (T1-01, T1-03 openai): every non-link type with every field it ignores set, URL included, gives exactly the
// item it gives without them (the 10.0d items): nothing leaks across the restructured no-file branch.
func TestPolls101A_IgnoredFields(t *testing.T) {
	file := func(m sender.InputMedia) *sender.InputMedia { // a file item ignores coordinates, venue fields and URL
		m.Latitude, m.Longitude, m.HorizontalAccuracy = 1, 2, 3
		m.Title, m.Address = "T", "A"
		m.FoursquareID, m.FoursquareType, m.GooglePlaceID, m.GooglePlaceType = "f", "ft", "g", "gt"
		m.URL = "https://e.x/x"
		// T3-06 openai: the nested file metadata is ignored too (the file name only where nothing is uploaded).
		for _, f := range []*sender.InputFile{&m.Media, m.Media.Photo} {
			if f == nil {
				continue
			}
			f.Caption, f.ParseMode, f.MediaType = "mc", "MarkdownV2", "photo"
			if f.Reader == nil && f.Source == nil {
				f.FileName = "n.bin"
			}
		}
		return &m
	}
	noFile := func(m sender.InputMedia, venue bool) *sender.InputMedia { // location/venue ignore caption, emoji, URL
		m.Caption, m.ParseMode, m.Emoji, m.URL = "c", "HTML", "👍", "https://e.x/x"
		// T3-06 openai: and the nested file metadata (no file is set, so the no-file guard does not fire).
		m.Media.FileName, m.Media.MediaType, m.Media.Caption, m.Media.ParseMode = "n.bin", "photo", "mc", "MarkdownV2"
		if !venue {
			m.Title, m.Address = "T", "A"
			m.FoursquareID, m.FoursquareType, m.GooglePlaceID, m.GooglePlaceType = "f", "ft", "g", "gt"
		}
		return &m
	}
	rows := []struct {
		name  string
		media *sender.InputMedia
		want  string
		files map[string]wireFile
	}{
		{"photo_id", file(sender.NewInputMediaPhoto(sender.FromFileID("P"))), `{"type":"photo","media":"P"}`, map[string]wireFile{}},
		// T2-05 openai: a file by URL with InputMedia.URL also set: the file URL is the media, the link URL is ignored.
		{"photo_url", file(sender.NewInputMediaPhoto(sender.FromURL("https://e.x/photo.jpg"))),
			`{"type":"photo","media":"https://e.x/photo.jpg"}`, map[string]wireFile{}},
		{"photo_upload", file(sender.NewInputMediaPhoto(upload("d101-p", "p.jpg"))),
			`{"type":"photo","media":"attach://file0"}`, map[string]wireFile{"file0": {FileName: "p.jpg", Data: []byte("d101-p")}}},
		{"video", file(sender.NewInputMediaVideo(sender.FromFileID("V"))), `{"type":"video","media":"V"}`, map[string]wireFile{}},
		{"document", file(sender.NewInputMediaDocument(sender.FromFileID("D"))), `{"type":"document","media":"D"}`, map[string]wireFile{}},
		{"audio", file(sender.NewInputMediaAudio(sender.FromFileID("A"))), `{"type":"audio","media":"A"}`, map[string]wireFile{}},
		{"animation", file(sender.NewInputMediaAnimation(sender.FromFileID("N"))), `{"type":"animation","media":"N"}`, map[string]wireFile{}},
		{"live_photo", file(sender.NewInputMediaLivePhoto(sender.FromFileID("LV"), sender.FromFileID("LP"))),
			`{"type":"live_photo","media":"LV","photo":"LP"}`, map[string]wireFile{}},
		{"sticker", file(sender.NewInputMediaSticker(sender.FromFileID("S"))), `{"type":"sticker","media":"S"}`, map[string]wireFile{}},
		{"location", noFile(sender.NewInputMediaLocation(52.52, 13.405), false),
			`{"type":"location","latitude":52.52,"longitude":13.405}`, map[string]wireFile{}},
		{"venue", noFile(sender.NewInputMediaVenue(48.8584, 2.2945, "T", "A"), true),
			`{"type":"venue","latitude":48.8584,"longitude":2.2945,"title":"T","address":"A"}`, map[string]wireFile{}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{
				Options: []sender.InputPollOption{{Text: "m", Media: r.media}, {Text: "b"}}})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q",
				"options": pj(t, `[{"text":"m","media":`+r.want+`},{"text":"b"}]`)}, args)
			assertFiles(t, r.files, files)
		})
	}
}
