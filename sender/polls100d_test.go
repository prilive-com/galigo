package sender_test

// Bot API 10.0 part d (B-25 slice 2d, TEST-PLAN §3, T-s-3 … T-s-16): media in sendPoll (description, explanation,
// options), on the wire as the Bot API server reads it (B-45 decoder, wire_args_test.go).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// pollJSONKeys are the arguments of a poll request whose value is JSON and is compared decoded.
var pollJSONKeys = []string{"options", "media", "explanation_media"}

// pollJSON decodes one JSON value strictly: numbers keep their text (json.Number, so 1 and 1.0 differ), a
// duplicate key in any object at any depth is an error (the Bot API reads the first of two equal keys; a Go map
// keeps the last), and trailing data is an error (T1-05, T2-01 openai).
func pollJSON(raw string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	v, err := pollJSONValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trailing data after the JSON value")
	}
	return v, nil
}

func pollJSONValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch d := tok.(type) {
	case json.Delim:
		switch d {
		case '{':
			obj := map[string]any{}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key := kt.(string)
				if _, dup := obj[key]; dup {
					return nil, fmt.Errorf("duplicate key %q", key)
				}
				v, err := pollJSONValue(dec)
				if err != nil {
					return nil, err
				}
				obj[key] = v
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return obj, nil
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := pollJSONValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return arr, nil
		}
		return nil, fmt.Errorf("unexpected delimiter %v", d)
	default:
		return tok, nil // string, json.Number, bool or nil
	}
}

// pj decodes an expected JSON literal with pollJSON.
func pj(t *testing.T, raw string) any {
	t.Helper()
	v, err := pollJSON(raw)
	require.NoError(t, err, "expected literal %s", raw)
	return v
}

// pollArgs returns the arguments with each pollJSONKeys value decoded by pollJSON (an absent key stays absent;
// null stays a present key with a nil value).
func pollArgs(t *testing.T, args map[string]string) map[string]any {
	t.Helper()
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = v
	}
	for _, k := range pollJSONKeys {
		raw, ok := args[k]
		if !ok {
			continue
		}
		v, err := pollJSON(raw)
		require.NoError(t, err, "argument %s = %s", k, raw)
		out[k] = v
	}
	return out
}

// polls100dSend sends req with SendPoll to the mock server and returns the media type, the decoded arguments, the
// file parts and the call error.
func polls100dSend(t *testing.T, req sender.SendPollRequest) (string, map[string]any, map[string]wireFile, error) {
	t.Helper()
	if req.ChatID == nil {
		req.ChatID = testutil.TestChatID
	}
	if req.Question == "" {
		req.Question = "Q"
	}
	mediaType, args, files, err := wireCall(t, "sendPoll", replyMessage, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendPoll(ctx, req)
		return err
	})
	return mediaType, pollArgs(t, args), files, err
}

func plainOptions() []sender.InputPollOption {
	return []sender.InputPollOption{{Text: "a"}, {Text: "b"}}
}

func mp(m sender.InputMedia) *sender.InputMedia { return &m }

const plainOptionsJSON = `[{"text":"a"},{"text":"b"}]`

// T-s-3: description media by file id and by URL, with caption and parse mode.
func TestPolls100D_DescriptionByID(t *testing.T) {
	rows := []struct {
		name  string
		media sender.InputMedia
		want  string
	}{
		{"photo_id", sender.NewInputMediaPhoto(sender.FromFileID("P")), `{"type":"photo","media":"P"}`},
		{"photo_url", sender.NewInputMediaPhoto(sender.FromURL("https://e.x/p.jpg")), `{"type":"photo","media":"https://e.x/p.jpg"}`},
		{"video_id_caption", sender.NewInputMediaVideo(sender.FromFileID("V")).WithCaption("c").WithParseMode("HTML"),
			`{"type":"video","media":"V","caption":"c","parse_mode":"HTML"}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{Options: plainOptions(), Media: mp(r.media)})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q",
				"options": pj(t, plainOptionsJSON), "media": pj(t, r.want)}, args)
			assertFiles(t, map[string]wireFile{}, files)
		})
	}
}

// T-s-4: description media by upload.
func TestPolls100D_DescriptionUpload(t *testing.T) {
	mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{Options: plainOptions(),
		Media: mp(sender.NewInputMediaPhoto(sender.FromBytes([]byte("d100-photo"), "p.jpg")))})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q", "options": pj(t, plainOptionsJSON),
		"media": pj(t, `{"type":"photo","media":"attach://file0"}`)}, args)
	assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: []byte("d100-photo")}}, files)
}

func withAccuracy(m sender.InputMedia, acc float64) sender.InputMedia {
	m.HorizontalAccuracy = acc
	return m
}

// placeRow is one location or venue item and its whole expected JSON.
type placeRow struct {
	name  string
	media sender.InputMedia
	want  string
}

func checkPlaceRows(t *testing.T, rows []placeRow) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{Options: plainOptions(), Media: mp(r.media)})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q",
				"options": pj(t, plainOptionsJSON), "media": pj(t, r.want)}, args)
			assertFiles(t, map[string]wireFile{}, files)
		})
	}
}

// T-s-5: a location as description media, with coordinates always sent (also 0/0).
func TestPolls100D_Location(t *testing.T) {
	checkPlaceRows(t, []placeRow{
		{"plain", sender.NewInputMediaLocation(52.52, 13.405), `{"type":"location","latitude":52.52,"longitude":13.405}`},
		{"accuracy", withAccuracy(sender.NewInputMediaLocation(52.52, 13.405), 50.5),
			`{"type":"location","latitude":52.52,"longitude":13.405,"horizontal_accuracy":50.5}`},
		{"zero", sender.NewInputMediaLocation(0, 0), `{"type":"location","latitude":0,"longitude":0}`},
		// T3-02 openai: one zero coordinate is sent as well.
		{"zero_latitude", sender.NewInputMediaLocation(0, 13.405), `{"type":"location","latitude":0,"longitude":13.405}`},
		{"zero_longitude", sender.NewInputMediaLocation(52.52, 0), `{"type":"location","latitude":52.52,"longitude":0}`},
	})
}

func venueAll() sender.InputMedia {
	v := withAccuracy(sender.NewInputMediaVenue(48.8584, 2.2945, "T", "A"), 10)
	v.FoursquareID, v.FoursquareType, v.GooglePlaceID, v.GooglePlaceType = "fid", "ft", "gid", "gt"
	return v
}

// T-s-6: a venue as description media: required fields always, each optional field when set (alone and together).
func TestPolls100D_Venue(t *testing.T) {
	req := sender.NewInputMediaVenue(48.8584, 2.2945, "T", "A")
	const base = `"type":"venue","latitude":48.8584,"longitude":2.2945,"title":"T","address":"A"`
	only := func(set func(*sender.InputMedia)) sender.InputMedia {
		v := req
		set(&v)
		return v
	}
	checkPlaceRows(t, []placeRow{
		{"required", req, `{` + base + `}`},
		{"all", venueAll(), `{` + base + `,"foursquare_id":"fid","foursquare_type":"ft","google_place_id":"gid","google_place_type":"gt","horizontal_accuracy":10}`},
		{"zero", sender.NewInputMediaVenue(0, 0, "", ""), `{"type":"venue","latitude":0,"longitude":0,"title":"","address":""}`},
		{"only_foursquare_id", only(func(v *sender.InputMedia) { v.FoursquareID = "fid" }), `{` + base + `,"foursquare_id":"fid"}`},
		{"only_foursquare_type", only(func(v *sender.InputMedia) { v.FoursquareType = "ft" }), `{` + base + `,"foursquare_type":"ft"}`},
		{"only_google_place_id", only(func(v *sender.InputMedia) { v.GooglePlaceID = "gid" }), `{` + base + `,"google_place_id":"gid"}`},
		{"only_google_place_type", only(func(v *sender.InputMedia) { v.GooglePlaceType = "gt" }), `{` + base + `,"google_place_type":"gt"}`},
		// T3-02, T3-03 openai: one zero coordinate; accuracy without any venue ID or type.
		{"zero_latitude", sender.NewInputMediaVenue(0, 2.2945, "T", "A"),
			`{"type":"venue","latitude":0,"longitude":2.2945,"title":"T","address":"A"}`},
		{"zero_longitude", sender.NewInputMediaVenue(48.8584, 0, "T", "A"),
			`{"type":"venue","latitude":48.8584,"longitude":0,"title":"T","address":"A"}`},
		{"accuracy_only", withAccuracy(req, 10), `{` + base + `,"horizontal_accuracy":10}`},
	})
}

// T-s-7: the exact text of a place item (key order of a Go map, numbers in shortest form).
func TestPolls100D_NumberText(t *testing.T) {
	rows := []struct {
		name  string
		media sender.InputMedia
		want  string
	}{
		{"location", withAccuracy(sender.NewInputMediaLocation(52.52, 13.405), 50.5),
			`{"horizontal_accuracy":50.5,"latitude":52.52,"longitude":13.405,"type":"location"}`},
		{"venue", sender.NewInputMediaVenue(48.8584, 2.2945, "T", "A"),
			`{"address":"A","latitude":48.8584,"longitude":2.2945,"title":"T","type":"venue"}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			_, args, _, err := wireCall(t, "sendPoll", replyMessage, func(ctx context.Context, c *sender.Client) error {
				_, err := c.SendPoll(ctx, sender.SendPollRequest{ChatID: testutil.TestChatID, Question: "Q",
					Options: plainOptions(), Media: mp(r.media)})
				return err
			})
			require.NoError(t, err)
			assert.Equal(t, r.want, args["media"])
		})
	}
}

// T-s-8: a location or venue with a file is refused before anything is sent (P1-03 openai, P1-01 google).
func TestPolls100D_NoFileOnPlace(t *testing.T) {
	photo := sender.FromFileID("Y")
	files := []struct {
		name string
		in   sender.InputFile
	}{
		{"file_id", sender.FromFileID("X")},
		{"url", sender.FromURL("https://e.x")},
		{"reader", sender.FromReader(strings.NewReader("r"), "r.bin")},
		{"source", sender.FromBytes([]byte("s"), "s.bin")},
		{"photo_only", sender.InputFile{Photo: &photo}},
	}
	places := []struct {
		name string
		mk   func() sender.InputMedia
	}{
		{"location", func() sender.InputMedia { return sender.NewInputMediaLocation(1, 2) }},
		{"venue", func() sender.InputMedia { return sender.NewInputMediaVenue(1, 2, "T", "A") }},
	}
	for _, p := range places {
		for _, f := range files {
			t.Run(p.name+"/"+f.name, func(t *testing.T) {
				m := p.mk()
				m.Media = f.in
				server := testutil.NewMockServer(t)
				client := testutil.NewTestClient(t, server.BaseURL())
				_, err := client.SendPoll(context.Background(), sender.SendPollRequest{ChatID: testutil.TestChatID,
					Question: "Q", Options: plainOptions(), Media: &m})
				require.Error(t, err)
				assert.True(t, strings.HasPrefix(err.Error(), "failed to build request: field media: "), err.Error())
				assert.True(t, strings.HasSuffix(err.Error(), "InputMedia: "+p.name+" has no media file"), err.Error())
				assert.Equal(t, 0, server.CaptureCount())
			})
		}
	}
	t.Run("option/file_id", func(t *testing.T) {
		m := sender.NewInputMediaLocation(1, 2)
		m.Media = sender.FromFileID("X")
		server := testutil.NewMockServer(t)
		client := testutil.NewTestClient(t, server.BaseURL())
		_, err := client.SendPoll(context.Background(), sender.SendPollRequest{ChatID: testutil.TestChatID,
			Question: "Q", Options: []sender.InputPollOption{{Text: "a", Media: &m}, {Text: "b"}}})
		require.Error(t, err)
		assert.True(t, strings.HasPrefix(err.Error(), "failed to build request: field options: "), err.Error())
		assert.True(t, strings.HasSuffix(err.Error(), "InputMedia: location has no media file"), err.Error())
		assert.Equal(t, 0, server.CaptureCount())
	})
}

func stickerWithEmoji(in sender.InputFile, emoji string) sender.InputMedia {
	m := sender.NewInputMediaSticker(in)
	m.Emoji = emoji
	return m
}

var (
	bold1       = []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}}
	customEmoji = []tg.MessageEntity{{Type: "custom_emoji", Offset: 0, Length: 1, CustomEmojiID: "5368324170671202286"}}
)

// T-s-9: option media by reference, every option field kept, plain options before media options, entities
// without a parse mode (T1-01, T1-02, T2-03 openai).
func TestPolls100D_OptionMedia(t *testing.T) {
	rows := []struct {
		name    string
		options []sender.InputPollOption
		want    string
		files   map[string]wireFile
	}{
		{"ids", []sender.InputPollOption{
			{Text: "p", Media: mp(sender.NewInputMediaPhoto(sender.FromFileID("P")))},
			{Text: "plain"},
			{Text: "l", Media: mp(withAccuracy(sender.NewInputMediaLocation(52.52, 13.405), 50))},
			{Text: "v", Media: mp(sender.NewInputMediaVenue(48.8584, 2.2945, "T", "A"))},
			{Text: "s", Media: mp(stickerWithEmoji(sender.FromFileID("S"), "👍"))},
			{Text: "lp", Media: mp(sender.NewInputMediaLivePhoto(sender.FromFileID("LV"), sender.FromFileID("LP")))},
		}, `[{"text":"p","media":{"type":"photo","media":"P"}},{"text":"plain"},` +
			`{"text":"l","media":{"type":"location","latitude":52.52,"longitude":13.405,"horizontal_accuracy":50}},` +
			`{"text":"v","media":{"type":"venue","latitude":48.8584,"longitude":2.2945,"title":"T","address":"A"}},` +
			`{"text":"s","media":{"type":"sticker","media":"S","emoji":"👍"}},` +
			`{"text":"lp","media":{"type":"live_photo","media":"LV","photo":"LP"}}]`, map[string]wireFile{}},
		{"all_fields", []sender.InputPollOption{
			{Text: "x", TextParseMode: tg.ParseModeHTML, TextEntities: bold1, Media: mp(sender.NewInputMediaPhoto(sender.FromFileID("P")))},
		}, `[{"text":"x","text_parse_mode":"HTML","text_entities":[{"type":"bold","offset":0,"length":1}],"media":{"type":"photo","media":"P"}}]`,
			map[string]wireFile{}},
		{"plain_first", []sender.InputPollOption{
			{Text: "plain", TextParseMode: tg.ParseModeHTML, TextEntities: bold1},
			{Text: "p", Media: mp(sender.NewInputMediaPhoto(sender.FromFileID("P")))},
		}, `[{"text":"plain","text_parse_mode":"HTML","text_entities":[{"type":"bold","offset":0,"length":1}]},` +
			`{"text":"p","media":{"type":"photo","media":"P"}}]`, map[string]wireFile{}},
		{"plain_first_upload", []sender.InputPollOption{
			{Text: "plain"},
			{Text: "u", Media: mp(sender.NewInputMediaPhoto(sender.FromBytes([]byte("d100-u"), "u.jpg")))},
		}, `[{"text":"plain"},{"text":"u","media":{"type":"photo","media":"attach://file0"}}]`,
			map[string]wireFile{"file0": {FileName: "u.jpg", Data: []byte("d100-u")}}},
		{"entities_only", []sender.InputPollOption{
			{Text: "e", TextEntities: customEmoji},
			{Text: "m", TextEntities: customEmoji, Media: mp(sender.NewInputMediaPhoto(sender.FromFileID("P")))},
		}, `[{"text":"e","text_entities":[{"type":"custom_emoji","offset":0,"length":1,"custom_emoji_id":"5368324170671202286"}]},` +
			`{"text":"m","text_entities":[{"type":"custom_emoji","offset":0,"length":1,"custom_emoji_id":"5368324170671202286"}],` +
			`"media":{"type":"photo","media":"P"}}]`, map[string]wireFile{}},
		// T3-01 openai: a parse mode without entities, on a plain option and on a media option.
		{"parse_mode_only", []sender.InputPollOption{
			{Text: "h", TextParseMode: tg.ParseModeHTML},
			{Text: "m", TextParseMode: tg.ParseModeHTML, Media: mp(sender.NewInputMediaPhoto(sender.FromFileID("P")))},
		}, `[{"text":"h","text_parse_mode":"HTML"},{"text":"m","text_parse_mode":"HTML","media":{"type":"photo","media":"P"}}]`,
			map[string]wireFile{}},
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
}

func upload(data, name string) sender.InputFile { return sender.FromBytes([]byte(data), name) }

// T-s-10: one attach counter for the whole request, in field order: options, then explanation_media, then media;
// a live photo's photo right after its video.
func TestPolls100D_AttachOrder(t *testing.T) {
	mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{
		Options: []sender.InputPollOption{
			{Text: "o", Media: mp(sender.NewInputMediaPhoto(upload("d100-o", "o.jpg")))},
			{Text: "plain"},
			{Text: "lp", Media: mp(sender.NewInputMediaLivePhoto(upload("d100-v", "v.mp4"), upload("d100-s", "s.jpg")))},
		},
		Type: "quiz", CorrectOptionIDs: []int{0}, Explanation: "e",
		ExplanationMedia: mp(sender.NewInputMediaDocument(upload("d100-e", "e.txt"))),
		Media:            mp(sender.NewInputMediaPhoto(upload("d100-d", "d.jpg"))),
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q", "type": "quiz", "correct_option_ids": "[0]",
		"explanation": "e",
		"options": pj(t, `[{"text":"o","media":{"type":"photo","media":"attach://file0"}},{"text":"plain"},`+
			`{"text":"lp","media":{"type":"live_photo","media":"attach://file1","photo":"attach://file2"}}]`),
		"explanation_media": pj(t, `{"type":"document","media":"attach://file3"}`),
		"media":             pj(t, `{"type":"photo","media":"attach://file4"}`)}, args)
	assertFiles(t, map[string]wireFile{
		"file0": {FileName: "o.jpg", Data: []byte("d100-o")}, "file1": {FileName: "v.mp4", Data: []byte("d100-v")},
		"file2": {FileName: "s.jpg", Data: []byte("d100-s")}, "file3": {FileName: "e.txt", Data: []byte("d100-e")},
		"file4": {FileName: "d.jpg", Data: []byte("d100-d")},
	}, files)
	assert.Equal(t, []string{"file0", "file1", "file2", "file3", "file4"}, polls100dPartOrder(t, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendPoll(ctx, sender.SendPollRequest{ChatID: testutil.TestChatID, Question: "Q",
			Options: []sender.InputPollOption{
				{Text: "o", Media: mp(sender.NewInputMediaPhoto(upload("d100-o", "o.jpg")))},
				{Text: "plain"},
				{Text: "lp", Media: mp(sender.NewInputMediaLivePhoto(upload("d100-v", "v.mp4"), upload("d100-s", "s.jpg")))},
			},
			Type: "quiz", CorrectOptionIDs: []int{0}, Explanation: "e",
			ExplanationMedia: mp(sender.NewInputMediaDocument(upload("d100-e", "e.txt"))),
			Media:            mp(sender.NewInputMediaPhoto(upload("d100-d", "d.jpg"))),
		})
		return err
	}))
}

// polls100dPartOrder returns the field names of the file parts of the captured multipart body, in body order.
func polls100dPartOrder(t *testing.T, call func(ctx context.Context, c *sender.Client) error) []string {
	t.Helper()
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPoll", replyMessage)
	client := testutil.NewTestClient(t, server.BaseURL())
	require.NoError(t, call(context.Background(), client))
	require.Equal(t, 1, server.CaptureCount())
	r := capturedRequest(server.LastCapture())
	mr, err := r.MultipartReader()
	require.NoError(t, err)
	var names []string
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if p.FileName() != "" {
			names = append(names, p.FormName())
		}
	}
	return names
}

// T-s-11: explanation media by reference and by upload alone (T1-04 google).
func TestPolls100D_Explanation(t *testing.T) {
	rows := []struct {
		name  string
		media sender.InputMedia
		want  string
		files map[string]wireFile
	}{
		{"by_id", sender.NewInputMediaLocation(52.52, 13.405), `{"type":"location","latitude":52.52,"longitude":13.405}`, map[string]wireFile{}},
		{"upload", sender.NewInputMediaDocument(upload("d100-x", "x.txt")), `{"type":"document","media":"attach://file0"}`,
			map[string]wireFile{"file0": {FileName: "x.txt", Data: []byte("d100-x")}}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{Options: plainOptions(),
				Type: "quiz", CorrectOptionIDs: []int{0}, Explanation: "e", ExplanationMedia: mp(r.media)})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q", "options": pj(t, plainOptionsJSON),
				"type": "quiz", "correct_option_ids": "[0]", "explanation": "e", "explanation_media": pj(t, r.want)}, args)
			assertFiles(t, r.files, files)
		})
	}
}

// T-s-12: a poll without media is sent byte for byte as at the baseline (literals copied from the baseline record
// E10/red/baseline-bytes.txt, recorded at ab5a1b8 before the change).
func TestPolls100D_NoMediaBytes(t *testing.T) {
	anon, f, zero := false, false, 0
	rows := []struct {
		name string
		call func(ctx context.Context, c *sender.Client) error
		want string
	}{
		{"sendPoll", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPoll(ctx, sender.SendPollRequest{
				ChatID: int64(-100123), Question: "Q?", Options: []sender.InputPollOption{{Text: "A"}, {Text: "B"}},
				IsAnonymous: &anon, Type: "quiz", AllowsMultipleAnswers: true, CorrectOptionID: &zero, Explanation: "e",
				ExplanationParseMode: tg.ParseModeHTML, OpenPeriod: 60, CloseDate: 1793928546, IsClosed: true,
				DisableNotification: true, ReplyToMessageID: 7, ReplyMarkup: map[string]any{"k": 1},
				CorrectOptionIDs: []int{0}, AllowsRevoting: &f, ShuffleOptions: true, AllowAddingOptions: true,
				HideResultsUntilCloses: true, Description: "d", DescriptionParseMode: tg.ParseModeHTML,
				DescriptionEntities: []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}}, MembersOnly: true,
				CountryCodes: []string{"DE", "FT"}, ProtectContent: true,
			})
			return err
		}, `{"chat_id":-100123,"question":"Q?","options":[{"text":"A"},{"text":"B"}],"is_anonymous":false,"type":"quiz",` +
			`"allows_multiple_answers":true,"correct_option_id":0,"explanation":"e","explanation_parse_mode":"HTML",` +
			`"open_period":60,"close_date":1793928546,"is_closed":true,"disable_notification":true,"protect_content":true,` +
			`"reply_to_message_id":7,"reply_markup":{"k":1},"correct_option_ids":[0],"allows_revoting":false,` +
			`"shuffle_options":true,"allow_adding_options":true,"hide_results_until_closes":true,"members_only":true,` +
			`"country_codes":["DE","FT"],"description":"d","description_parse_mode":"HTML",` +
			`"description_entities":[{"type":"bold","offset":0,"length":1}]}`},
		{"simple", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPollSimple(ctx, testutil.TestChatID, "Q", []string{"a", "b"})
			return err
		}, `{"chat_id":123456789,"question":"Q","options":[{"text":"a"},{"text":"b"}]}`},
		{"quiz", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendQuiz(ctx, testutil.TestChatID, "Q", []string{"a", "b"}, 1)
			return err
		}, `{"chat_id":123456789,"question":"Q","options":[{"text":"a"},{"text":"b"}],"type":"quiz","correct_option_ids":[1]}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/sendPoll", replyMessage)
			client := testutil.NewTestClient(t, server.BaseURL())
			require.NoError(t, r.call(context.Background(), client))
			require.Equal(t, 1, server.CaptureCount())
			c := server.LastCapture()
			assert.Equal(t, "application/json", c.ContentType)
			assert.Equal(t, r.want, strings.TrimSuffix(c.BodyString(), "\n"))
		})
	}
}

// T-s-13: editMessageMedia items of the baseline constructors keep their keys, values and file parts (AC4;
// literals copied from the baseline record E10/red/baseline-bytes.txt). It pins the decoded item and the file
// bytes, not the raw item text or the multipart body bytes.
func TestPolls100D_EditItemsUnchanged(t *testing.T) {
	kinds := []struct {
		name string
		mk   func(sender.InputFile) sender.InputMedia
	}{
		{"photo", sender.NewInputMediaPhoto}, {"video", sender.NewInputMediaVideo}, {"document", sender.NewInputMediaDocument},
		{"audio", sender.NewInputMediaAudio}, {"animation", sender.NewInputMediaAnimation},
	}
	type row struct {
		name  string
		media sender.InputMedia
		want  string
		files map[string]wireFile
	}
	var rows []row
	for _, k := range kinds {
		rows = append(rows,
			row{k.name + "/id", k.mk(sender.FromFileID("ID-" + k.name)),
				`{"caption":"c","media":"ID-` + k.name + `","parse_mode":"HTML","type":"` + k.name + `"}`, map[string]wireFile{}},
			row{k.name + "/url", k.mk(sender.FromURL("https://e.x/" + k.name)),
				`{"caption":"c","media":"https://e.x/` + k.name + `","parse_mode":"HTML","type":"` + k.name + `"}`, map[string]wireFile{}},
			row{k.name + "/upload", k.mk(upload("d100-"+k.name, k.name+".bin")),
				`{"caption":"c","media":"attach://file0","parse_mode":"HTML","type":"` + k.name + `"}`,
				map[string]wireFile{"file0": {FileName: k.name + ".bin", Data: []byte("d100-" + k.name)}}},
		)
	}
	rows = append(rows,
		row{"live_photo/id", sender.NewInputMediaLivePhoto(sender.FromFileID("ID-lv"), sender.FromFileID("ID-lp")),
			`{"caption":"c","media":"ID-lv","parse_mode":"HTML","photo":"ID-lp","type":"live_photo"}`, map[string]wireFile{}},
		row{"live_photo/url", sender.NewInputMediaLivePhoto(sender.FromURL("https://e.x/lv"), sender.FromURL("https://e.x/lp")),
			`{"caption":"c","media":"https://e.x/lv","parse_mode":"HTML","photo":"https://e.x/lp","type":"live_photo"}`, map[string]wireFile{}},
		row{"live_photo/upload", sender.NewInputMediaLivePhoto(upload("d100-lv", "lv.bin"), upload("d100-lp", "lp.bin")),
			`{"caption":"c","media":"attach://file0","parse_mode":"HTML","photo":"attach://file1","type":"live_photo"}`,
			map[string]wireFile{"file0": {FileName: "lv.bin", Data: []byte("d100-lv")}, "file1": {FileName: "lp.bin", Data: []byte("d100-lp")}}},
	)
	require.Len(t, rows, 18)
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			m := r.media.WithCaption("c").WithParseMode("HTML")
			mediaType, args, files, err := wireCall(t, "editMessageMedia", replyMessage, func(ctx context.Context, c *sender.Client) error {
				_, err := c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{ChatID: testutil.TestChatID, MessageID: 42, Media: m})
				return err
			})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assert.Equal(t, map[string]any{"chat_id": wireChatID, "message_id": "42", "media": pj(t, r.want)}, pollArgs(t, args))
			assertFiles(t, r.files, files)
		})
	}
}

// T-s-15: the pollJSON helper itself (T1-05, T2-01 openai).
func TestPolls100D_PollArgsHelper(t *testing.T) {
	t.Run("duplicate option media", func(t *testing.T) {
		_, err := pollJSON(`[{"text":"p","media":null,"media":{"type":"photo","media":"P"}}]`)
		assert.EqualError(t, err, `duplicate key "media"`)
	})
	t.Run("duplicate inner key", func(t *testing.T) {
		_, err := pollJSON(`{"type":"photo","type":"video","media":"P"}`)
		assert.EqualError(t, err, `duplicate key "type"`)
	})
	t.Run("null is not absent", func(t *testing.T) {
		assert.NotEqual(t, pj(t, `[{"text":"p"}]`), pj(t, `[{"text":"p","media":null}]`))
	})
	t.Run("1.0 is not 1", func(t *testing.T) {
		assert.NotEqual(t, pj(t, `[{"text":"p","text_entities":[{"type":"bold","offset":0,"length":1}]}]`),
			pj(t, `[{"text":"p","text_entities":[{"type":"bold","offset":0,"length":1.0}]}]`))
	})
	t.Run("equal values are equal", func(t *testing.T) {
		assert.Equal(t, pj(t, `{"a":1,"b":[true,null,"x"]}`), pj(t, `{"b":[true,null,"x"],"a":1}`))
	})
	t.Run("trailing data", func(t *testing.T) {
		_, err := pollJSON(`{"a":1}{"b":2}`)
		assert.Error(t, err)
	})
}

// T-s-16: ids, places and URLs before uploads in the options, then explanation and description uploads; a sticker
// upload keeps its emoji (T1-03, T1-04 openai).
func TestPolls100D_AttachOrderMixed(t *testing.T) {
	mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{
		Options: []sender.InputPollOption{
			{Text: "i", Media: mp(sender.NewInputMediaPhoto(sender.FromFileID("P")))},
			{Text: "l", Media: mp(sender.NewInputMediaLocation(52.52, 13.405))},
			{Text: "r", Media: mp(sender.NewInputMediaPhoto(sender.FromURL("https://e.x/r.jpg")))},
			{Text: "k", Media: mp(stickerWithEmoji(upload("d100-k", "k.webp"), "👍"))},
			{Text: "u", Media: mp(sender.NewInputMediaPhoto(upload("d100-u", "u.jpg")))},
		},
		Type: "quiz", CorrectOptionIDs: []int{0}, Explanation: "e",
		ExplanationMedia: mp(sender.NewInputMediaDocument(upload("d100-e", "e.txt"))),
		Media:            mp(sender.NewInputMediaPhoto(upload("d100-d", "d.jpg"))),
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assert.Equal(t, map[string]any{"chat_id": wireChatID, "question": "Q", "type": "quiz", "correct_option_ids": "[0]",
		"explanation": "e",
		"options": pj(t, `[{"text":"i","media":{"type":"photo","media":"P"}},`+
			`{"text":"l","media":{"type":"location","latitude":52.52,"longitude":13.405}},`+
			`{"text":"r","media":{"type":"photo","media":"https://e.x/r.jpg"}},`+
			`{"text":"k","media":{"type":"sticker","media":"attach://file0","emoji":"👍"}},`+
			`{"text":"u","media":{"type":"photo","media":"attach://file1"}}]`),
		"explanation_media": pj(t, `{"type":"document","media":"attach://file2"}`),
		"media":             pj(t, `{"type":"photo","media":"attach://file3"}`)}, args)
	assertFiles(t, map[string]wireFile{
		"file0": {FileName: "k.webp", Data: []byte("d100-k")}, "file1": {FileName: "u.jpg", Data: []byte("d100-u")},
		"file2": {FileName: "e.txt", Data: []byte("d100-e")}, "file3": {FileName: "d.jpg", Data: []byte("d100-d")},
	}, files)
}

// T-s-17 (C1-01 openai, round c1): a nil *InputMedia held in an `any` field is not removed by the zero-value skip
// (the interface is not nil). It is written like a nil *InputFile: no argument, no file, no multipart, no panic.
func TestPolls100D_TypedNilInAny(t *testing.T) {
	var nilMedia *sender.InputMedia
	t.Run("builder", func(t *testing.T) {
		got, err := sender.BuildMultipartRequest(struct {
			Media any `json:"media,omitempty"`
		}{Media: nilMedia})
		require.NoError(t, err)
		assert.Empty(t, got.Params)
		assert.Empty(t, got.Files)
	})
	t.Run("send_poll_reply_markup", func(t *testing.T) {
		mediaType, args, files, err := polls100dSend(t, sender.SendPollRequest{Options: plainOptions(), ReplyMarkup: nilMedia})
		require.NoError(t, err)
		assert.Equal(t, "application/json", mediaType)
		assert.Empty(t, files)
		assert.NotContains(t, args, "media")
		assert.NotContains(t, args, "explanation_media")
	})
}
