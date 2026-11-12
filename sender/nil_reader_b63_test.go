package sender_test

// B-63: an upload reader that is nil (an InputFile.Source that returns nil, or
// a typed-nil pointer) must give a *tg.ValidationError and send nothing,
// instead of a panic in the encoder goroutine.
//
// On a build without the fix the client rows panic in a goroutine the library
// starts, which stops the whole test binary: run them one by one with -run.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

const b63Msg = "upload reader is nil (InputFile.Source returned nil, or the reader is a nil pointer)"

// b63EscA is the JSON escape of the letter "a" (backslash, u0061). It is built
// from the rune so that no tool can turn the escape into its letter in this
// source file (that happened once: test round t1, Claude A-T1-01).
var b63EscA = string(rune(0x5c)) + "u0061"

func b63NilSource(name string) sender.InputFile {
	return sender.InputFile{Source: func() io.Reader { return nil }, FileName: name}
}

func b63Good(name string) sender.InputFile { return sender.FromBytes([]byte("x"), name) }

func b63Media(f sender.InputFile) *sender.InputMedia {
	m := sender.InputMedia{Type: "photo", Media: f}
	return &m
}

// b63NilSafe is a pointer reader that works on a nil receiver. The fix refuses
// it anyway (PLAN AC2: every typed-nil pointer).
type b63NilSafe struct{}

func (*b63NilSafe) Read([]byte) (int, error) { return 0, io.EOF }

// b63Value is a value-kind reader; b63Slice is a slice reader that is nil.
type b63Value struct{}

func (b63Value) Read([]byte) (int, error) { return 0, io.EOF }

type b63Slice []byte

func (b63Slice) Read([]byte) (int, error) { return 0, io.EOF }

// b63Scalar is a pointer reader whose element is not a struct: a typed nil of
// it is refused like any other (AC2; test round t3, A-T3-02 = O-T3-02).
type b63Scalar int

func (*b63Scalar) Read([]byte) (int, error) { return 0, io.EOF }

// requireB63 checks the error of one nil part.
func requireB63(t *testing.T, err error, field, part string) {
	t.Helper()
	var ve *tg.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, field, ve.Field)
	assert.Equal(t, "part "+part+": "+b63Msg, ve.Message)
}

// b63RT counts RoundTrip calls (deterministic, unlike a server capture of a
// body that fails mid-stream) and answers every call with a message.
type b63RT struct{ calls atomic.Int32 }

func (r *b63RT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.calls.Add(1)
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":1,"date":0,"chat":{"id":1,"type":"private"}}}`)), Request: req}, nil
}

// b63Server answers every method with a message, so a request that should not
// be sent is visible as a capture.
func b63Server(t *testing.T) *testutil.MockTelegramServer {
	t.Helper()
	srv := testutil.NewMockServer(t)
	srv.On("/bot"+testutil.TestToken+"/", func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 1) })
	return srv
}

type b63Row struct {
	name, field, part string
	call              func(ctx context.Context, c *sender.Client) error
}

// b63PathRows: one row per upload path (PLAN §6, sites 1-7), plus the typed-nil
// rows and the nil part first, last and alone (E-0044).
func b63PathRows() []b63Row {
	cid := testutil.TestChatID
	var osFile *os.File
	opts2 := []sender.InputPollOption{{Text: "a"}, {Text: "b"}}
	return []b63Row{
		// site 1: single-file fields
		{"document", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: b63NilSource("a")})
			return err
		}},
		{"photo", "photo", "photo", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPhoto(ctx, sender.SendPhotoRequest{ChatID: cid, Photo: b63NilSource("a")})
			return err
		}},
		{"video_thumbnail_last", "thumbnail", "thumbnail", func(ctx context.Context, c *sender.Client) error {
			th := b63NilSource("t")
			_, err := c.SendVideo(ctx, sender.SendVideoRequest{ChatID: cid, Video: b63Good("v"), Thumbnail: &th})
			return err
		}},
		{"upload_sticker_file", "sticker", "sticker", func(ctx context.Context, c *sender.Client) error {
			_, err := c.UploadStickerFile(ctx, sender.UploadStickerFileRequest{UserID: 1, Sticker: b63NilSource("s"), StickerFormat: "static"})
			return err
		}},
		// site 2: []InputFile (media group): first, middle, last
		{"media_group_first", "media", "file0", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{ChatID: cid, Media: []sender.InputFile{
				b63NilSource("a").WithMediaType("photo"), b63Good("b").WithMediaType("photo"), b63Good("c").WithMediaType("photo")}})
			return err
		}},
		{"media_group_middle", "media", "file1", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{ChatID: cid, Media: []sender.InputFile{
				b63Good("a").WithMediaType("photo"), b63NilSource("b").WithMediaType("photo"), b63Good("c").WithMediaType("photo")}})
			return err
		}},
		{"media_group_last", "media", "file2", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{ChatID: cid, Media: []sender.InputFile{
				b63Good("a").WithMediaType("photo"), b63Good("b").WithMediaType("photo"), b63NilSource("c").WithMediaType("photo")}})
			return err
		}},
		// site 4 through site 2: a live photo's photo
		{"media_group_live_photo", "media", "file1", func(ctx context.Context, c *sender.Client) error {
			p := b63NilSource("p")
			f := b63Good("v").WithMediaType("live_photo")
			f.Photo = &p
			_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{ChatID: cid, Media: []sender.InputFile{f, b63Good("b").WithMediaType("photo")}})
			return err
		}},
		// site 3: InputMedia (edit media, poll media, rich message media)
		{"edit_message_media", "media", "file0", func(ctx context.Context, c *sender.Client) error {
			_, err := c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{ChatID: cid, MessageID: 1, Media: sender.InputMedia{Type: "photo", Media: b63NilSource("a")}})
			return err
		}},
		{"poll_media", "media", "file0", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPoll(ctx, sender.SendPollRequest{ChatID: cid, Question: "q", Options: opts2, Media: b63Media(b63NilSource("a"))})
			return err
		}},
		{"poll_explanation_media", "explanation_media", "file0", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPoll(ctx, sender.SendPollRequest{ChatID: cid, Question: "q", Type: "quiz", Options: opts2,
				ExplanationMedia: b63Media(b63NilSource("a")), Media: b63Media(b63Good("b"))})
			return err
		}},
		{"poll_option_media", "options", "file1", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPoll(ctx, sender.SendPollRequest{ChatID: cid, Question: "q", Options: []sender.InputPollOption{
				{Text: "a", Media: b63Media(b63Good("x"))}, {Text: "b", Media: b63Media(b63NilSource("y"))}}})
			return err
		}},
		{"rich_message_media", "rich_message", "file0", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendRichMessage(ctx, sender.SendRichMessageRequest{ChatID: cid, RichMessage: tg.InputRichMessage{HTML: "x",
				Media: []tg.InputRichMessageMedia{{ID: "m", Media: sender.InputMedia{Type: "photo", Media: b63NilSource("a")}}}}})
			return err
		}},
		// site 4: paid media (alone)
		{"paid_media_alone", "media", "file0", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPaidMedia(ctx, sender.SendPaidMediaRequest{ChatID: cid, StarCount: 1,
				Media: []sender.InputPaidMedia{sender.NewInputPaidMediaPhoto(b63NilSource("a"))}})
			return err
		}},
		// site 5: []FilePart payloads (stories, profile photos)
		{"post_story", "content", "story_photo", func(ctx context.Context, c *sender.Client) error {
			_, err := c.PostStory(ctx, sender.PostStoryRequest{BusinessConnectionID: "b",
				Content: &sender.InputStoryContentPhoto{Photo: b63NilSource("a")}, ActivePeriod: 86400})
			return err
		}},
		{"business_profile_photo", "photo", "profile_photo", func(ctx context.Context, c *sender.Client) error {
			return c.SetBusinessAccountProfilePhoto(ctx, sender.SetBusinessAccountProfilePhotoRequest{BusinessConnectionID: "b",
				Photo: &sender.InputProfilePhotoStatic{Photo: b63NilSource("a")}})
		}},
		{"my_profile_photo", "photo", "profile_photo", func(ctx context.Context, c *sender.Client) error {
			return c.SetMyProfilePhoto(ctx, &sender.InputProfilePhotoStatic{Photo: b63NilSource("a")})
		}},
		// site 6: sticker set
		{"new_sticker_set", "stickers", "sticker_file_0", func(ctx context.Context, c *sender.Client) error {
			return c.CreateNewStickerSet(ctx, sender.CreateNewStickerSetRequest{UserID: 1, Name: "n", Title: "t",
				Stickers: []sender.InputSticker{{Sticker: b63NilSource("s"), Format: "static", EmojiList: []string{"x"}}}})
		}},
		{"add_sticker_to_set", "sticker", "sticker_file_0", func(ctx context.Context, c *sender.Client) error {
			return c.AddStickerToSet(ctx, sender.AddStickerToSetRequest{UserID: 1, Name: "n",
				Sticker: sender.InputSticker{Sticker: b63NilSource("s"), Format: "static", EmojiList: []string{"x"}}})
		}},
		// typed nil (AC2)
		{"typed_nil_source", "document", "document", func(ctx context.Context, c *sender.Client) error {
			f := sender.InputFile{FileName: "a", Source: func() io.Reader { var r *bytes.Reader; return r }}
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: f})
			return err
		}},
		{"typed_nil_reader", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: sender.FromReader((*bytes.Reader)(nil), "a")})
			return err
		}},
		// No file name: the check does not depend on FileName (test round t2,
		// A-T2-01 = O-T2-01).
		{"nameless_nil_source", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: sender.InputFile{Source: func() io.Reader { return nil }}})
			return err
		}},
		{"typed_nil_scalar", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: sender.FromReader((*b63Scalar)(nil), "a")})
			return err
		}},
		{"typed_nil_os_file", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: sender.FromReader(osFile, "a")})
			return err
		}},
		{"typed_nil_nil_safe", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: sender.FromReader((*b63NilSafe)(nil), "a")})
			return err
		}},
		// the Field rule (AC3): user text never takes the field
		{"caption_equals_ref", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: b63NilSource("a"), Caption: "attach://document"})
			return err
		}},
		{"caption_quotes_ref", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: b63NilSource("a"), Caption: `Example: "attach://document"`})
			return err
		}},
		{"caption_is_json_string", "document", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: b63NilSource("a"), Caption: `"attach://document"`})
			return err
		}},
		{"paid_media_caption_quotes_ref", "media", "file0", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPaidMedia(ctx, sender.SendPaidMediaRequest{ChatID: cid, StarCount: 1, Caption: `Example: "attach://file0"`,
				Media: []sender.InputPaidMedia{sender.NewInputPaidMediaPhoto(b63NilSource("a"))}})
			return err
		}},
		// the documented limit (AC3): another value that decodes to a JSON object or
		// array holding the ref wins; for a single-file part whatever its key
		{"limit_reply_markup_button", "reply_markup", "document", func(ctx context.Context, c *sender.Client) error {
			kb := map[string]any{"inline_keyboard": [][]map[string]string{{{"text": "attach://document", "callback_data": "x"}}}}
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: b63NilSource("a"), ReplyMarkup: kb})
			return err
		}},
		{"limit_other_media_caption", "explanation_media", "file0", func(ctx context.Context, c *sender.Client) error {
			expl := sender.InputMedia{Type: "photo", Media: sender.FromFileID("x"), Caption: "attach://file0"}
			_, err := c.SendPoll(ctx, sender.SendPollRequest{ChatID: cid, Question: "q", Type: "quiz", Options: opts2,
				ExplanationMedia: &expl, Media: b63Media(b63NilSource("a"))})
			return err
		}},
		{"limit_caption_entities_url", "caption_entities", "file0", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPaidMedia(ctx, sender.SendPaidMediaRequest{ChatID: cid, StarCount: 1, Caption: "x",
				CaptionEntities: []tg.MessageEntity{{Type: "text_link", Offset: 0, Length: 1, URL: "attach://file0"}},
				Media:           []sender.InputPaidMedia{sender.NewInputPaidMediaPhoto(b63NilSource("a"))}})
			return err
		}},
		{"limit_caption_is_json_array", "caption", "document", func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendDocument(ctx, sender.SendDocumentRequest{ChatID: cid, Document: b63NilSource("a"), Caption: `["attach://document"]`})
			return err
		}},
	}
}

func TestB63_ClientPaths(t *testing.T) {
	for _, row := range b63PathRows() {
		t.Run(row.name, func(t *testing.T) {
			rt := &b63RT{}
			c := testutil.NewTestClient(t, "http://galigo.invalid", sender.WithHTTPClient(&http.Client{Transport: rt}))
			err := row.call(context.Background(), c)
			requireB63(t, err, row.field, row.part)
			assert.True(t, strings.HasPrefix(err.Error(), "failed to build request: field "+row.field+": "), err.Error())
			assert.Equal(t, int32(0), rt.calls.Load(), "no request may be sent")
		})
	}
}

// TestB63_Builder: BuildMultipartRequest with a caller's own struct, and the
// Field rule on caller-built values (PLAN AC3, §10).
func TestB63_Builder(t *testing.T) {
	type payload struct {
		Parts []sender.FilePart `json:"uploads"`
		A     any               `json:"a,omitempty"`
		Media any               `json:"media,omitempty"`
		Z     any               `json:"z,omitempty"`
	}
	nilPart := func(name string) []sender.FilePart { return []sender.FilePart{{FieldName: name, FileName: "x"}} }
	rows := []struct {
		name, part, field string
		p                 payload
	}{
		{"holder_first", "file1", "media", payload{Media: `{"media":"attach://file1"}`}},
		{"prefix_file10_ignored", "file1", "media", payload{A: `{"media":"attach://file10"}`, Media: `{"media":"attach://file1"}`}},
		{"plain_text_ignored", "file1", "media", payload{A: `Example attach://file1 suffix`, Media: `{"media":"attach://file1"}`}},
		{"quoted_text_ignored", "file1", "media", payload{A: `Example "attach://file1" suffix`, Media: `{"media":"attach://file1"}`}},
		{"json_key_ignored", "file1", "media", payload{A: json.RawMessage(`{"attach://file1":"text"}`), Media: `{"media":"attach://file1"}`}},
		{"top_level_json_string_ignored", "file1", "media", payload{A: `"attach://file1"`, Media: `{"media":"attach://file1"}`}},
		{"escaped_slashes", "file1", "media", payload{Media: json.RawMessage(`{"media":"attach:\/\/file1"}`)}},
		{"escaped_unicode", "file1", "media", payload{Media: json.RawMessage(`{"media":"` + b63EscA + `ttach://file1"}`)}},
		{"escaped_part_name", `f"x`, "media", payload{Media: map[string]string{"media": `attach://f"x`}}},
		{"nested_array", "file1", "media", payload{Media: json.RawMessage(`[{"x":[{"media":"attach://file1"}]}]`)}},
		{"two_fields_first_key_wins", "file1", "a", payload{A: `["attach://file1"]`, Media: `{"media":"attach://file1"}`}},
		{"reference_last", "file1", "z", payload{Z: `{"media":"attach://file1"}`}},
		{"bare_ref_value_falls_back", "file1", "file1", payload{Media: "attach://file1"}},
		{"invalid_json_falls_back", "file1", "file1", payload{Media: `{"media":attach://file1}`}},
		{"no_reference_falls_back", "file1", "file1", payload{}},
		{"empty_field_name", "", "", payload{}},
		// A value that does not decode (a number out of float64 range) never
		// counts, even though most of it would decode (test round t1, Codex O-T1-01).
		{"partial_decode_error", "file1", "file1", payload{A: json.RawMessage(`{"bad":1e1000,"media":"attach://file1"}`)}},
		// Only an exact nested value counts, not a text that ends with the
		// reference (test round t1, Codex O-T1-02), and not a value that is
		// equal only after trimming or case folding (test round t2, A-T2-03 =
		// O-T2-02).
		{"nested_prefix_text_ignored", "file1", "media", payload{A: `{"text":"prefixattach://file1"}`, Media: `{"media":"attach://file1"}`}},
		{"spaced_ref_ignored", "file1", "media", payload{A: `{"text":" attach://file1 "}`, Media: `{"media":"attach://file1"}`}},
		{"case_ref_ignored", "file1", "media", payload{A: `{"text":"ATTACH://FILE1"}`, Media: `{"media":"attach://file1"}`}},
		// The whole value must be JSON: an object followed by text does not
		// count (test round t2, Claude A-T2-03).
		{"trailing_text_ignored", "file1", "media", payload{A: `{"media":"attach://file1"} x`, Media: `{"media":"attach://file1"}`}},
		// A nested value that is not the reference string never counts: null,
		// a bool, a number, or an object with no reference (change round c1,
		// Codex O-C1-01).
		{"nested_null_ignored", "file1", "media", payload{A: `[null]`, Media: `{"media":"attach://file1"}`}},
		{"nested_bools_ignored", "file1", "media", payload{A: `[true,false]`, Media: `{"media":"attach://file1"}`}},
		{"nested_number_ignored", "file1", "media", payload{A: `{"n":0}`, Media: `{"media":"attach://file1"}`}},
		{"object_without_ref_ignored", "file1", "media", payload{A: `[{"n":"x"}]`, Media: `{"media":"attach://file1"}`}},
		// Empty containers and nested strings shorter than the reference
		// never count (change round c2, Codex O-C2-02 = Claude A-C2-03, and
		// Claude A-C2-01).
		{"empty_object_ignored", "file1", "media", payload{A: `{}`, Media: `{"media":"attach://file1"}`}},
		{"empty_containers_ignored", "file1", "media", payload{A: `[[],{}]`, Media: `{"media":"attach://file1"}`}},
		{"nested_ref_prefix_ignored", "file1", "media", payload{A: `["","attach://","attach://file"]`, Media: `{"media":"attach://file1"}`}},
		// JSON allows any of its four white space bytes before the value
		// (change round c1, Claude A-C1-01; all four since round c2, Codex
		// O-C2-01 = Claude A-C2-02).
		{"leading_whitespace_value", "file1", "media", payload{Media: " \t\r\n" + `[{"type":"photo","media":"attach://file1"}]`}},
	}
	require.Equal(t, []byte{0x5c, 'u', '0', '0', '6', '1'}, []byte(b63EscA), "the escape must be bytes, not the letter")
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			row.p.Parts = nilPart(row.part)
			// 32 times: params is a map, so a lookup that forgets to sort its
			// keys passes one run by chance (test round t1, Claude A-T1-03).
			for range 32 {
				_, err := sender.BuildMultipartRequest(row.p)
				requireB63(t, err, row.field, row.part)
				require.Equal(t, "field "+row.field+": galigo: validation: "+row.field+" - part "+row.part+": "+b63Msg, err.Error())
			}
		})
	}

	t.Run("position", func(t *testing.T) {
		for pos, want := range []string{"p0", "p1", "p2"} {
			parts := []sender.FilePart{
				{FieldName: "p0", FileName: "x", Reader: strings.NewReader("0")},
				{FieldName: "p1", FileName: "x", Reader: strings.NewReader("1")},
				{FieldName: "p2", FileName: "x", Reader: strings.NewReader("2")},
			}
			parts[pos].Reader = nil
			_, err := sender.BuildMultipartRequest(payload{Parts: parts, Z: "text"})
			requireB63(t, err, want, want)
		}
	})

	// Two nil parts: the first in order is named (test round t1, Codex O-T1-06).
	t.Run("two_nil_parts_first_wins", func(t *testing.T) {
		parts := []sender.FilePart{{FieldName: "a", FileName: "x"}, {FieldName: "b", FileName: "x"}}
		_, err := sender.BuildMultipartRequest(payload{Parts: parts})
		requireB63(t, err, "a", "a")
	})

	t.Run("public_request", func(t *testing.T) {
		_, err := sender.BuildMultipartRequest(sender.SendDocumentRequest{ChatID: int64(1), Document: b63NilSource("a")})
		requireB63(t, err, "document", "document")
	})
}

// TestB63_Encode: the exported encoder refuses a nil part before writing
// anything, with the same Field rule (PLAN AC4).
func TestB63_Encode(t *testing.T) {
	good := func(name string) sender.FilePart {
		return sender.FilePart{FieldName: name, FileName: "x", Reader: strings.NewReader("ok")}
	}
	nilPart := func(name string) sender.FilePart { return sender.FilePart{FieldName: name, FileName: "x"} }
	var typedNil *bytes.Reader
	rows := []struct {
		name, field, part string
		req               sender.MultipartRequest
	}{
		{"first", "a", "a", sender.MultipartRequest{Files: []sender.FilePart{nilPart("a"), good("b"), good("c")}}},
		{"middle", "b", "b", sender.MultipartRequest{Files: []sender.FilePart{good("a"), nilPart("b"), good("c")}}},
		{"last", "c", "c", sender.MultipartRequest{Files: []sender.FilePart{good("a"), good("b"), nilPart("c")}}},
		{"alone", "a", "a", sender.MultipartRequest{Files: []sender.FilePart{nilPart("a")}}},
		{"nameless", "a", "a", sender.MultipartRequest{Files: []sender.FilePart{{FieldName: "a"}}}},
		// An empty field name, with a file name, so the two name fields are
		// tested apart (test round t3, A-T3-01 = O-T3-01 = LT3-01).
		{"empty_field_name", "", "", sender.MultipartRequest{Files: []sender.FilePart{{FileName: "x"}}}},
		{"typed_nil_scalar", "a", "a", sender.MultipartRequest{Files: []sender.FilePart{{FieldName: "a", FileName: "x", Reader: (*b63Scalar)(nil)}}}},
		{"typed_nil", "a", "a", sender.MultipartRequest{Files: []sender.FilePart{{FieldName: "a", FileName: "x", Reader: typedNil}}}},
		{"referring_param", "media", "file1", sender.MultipartRequest{
			Files:  []sender.FilePart{good("file0"), nilPart("file1")},
			Params: map[string]string{"caption": `x "attach://file1"`, "media": `[{"type":"photo","media":"attach://file1"}]`},
		}},
		{"referring_param_escaped", "media", "file1", sender.MultipartRequest{
			Files:  []sender.FilePart{nilPart("file1")},
			Params: map[string]string{"media": `[{"type":"photo","media":"` + b63EscA + `ttach://file1"}]`},
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := sender.NewMultipartEncoder(&buf).Encode(row.req)
			requireB63(t, err, row.field, row.part)
			assert.Equal(t, "field "+row.field+": galigo: validation: "+row.field+" - part "+row.part+": "+b63Msg, err.Error(),
				"Encode returns the checker's error unwrapped (AC4)")
			assert.Equal(t, 0, buf.Len(), "nothing may be written")
		})
	}

	t.Run("valid_unchanged", func(t *testing.T) {
		var buf bytes.Buffer
		enc := sender.NewMultipartEncoder(&buf)
		require.NoError(t, enc.Encode(sender.MultipartRequest{Files: []sender.FilePart{good("a")}, Params: map[string]string{"k": "v"}}))
		require.NoError(t, enc.Close())
		assert.Contains(t, buf.String(), `name="a"; filename="x"`)
		assert.Contains(t, buf.String(), "ok")
	})
}

// TestB63_Retry: the error is not retried, and a retry re-checks the new
// reader (PLAN AC1, AC3; E-0118).
func TestB63_Retry(t *testing.T) {
	t.Run("nil_on_first_attempt", func(t *testing.T) {
		srv := b63Server(t)
		sl := &testutil.FakeSleeper{}
		c := testutil.NewRetryTestClient(t, srv.BaseURL(), sl, sender.WithRetries(3))
		var calls atomic.Int32
		f := sender.InputFile{FileName: "p", Source: func() io.Reader { calls.Add(1); return nil }}
		_, err := c.SendPhoto(context.Background(), sender.SendPhotoRequest{ChatID: testutil.TestChatID, Photo: f})
		requireB63(t, err, "photo", "photo")
		assert.NotErrorIs(t, err, sender.ErrMaxRetries)
		assert.Equal(t, int32(1), calls.Load())
		assert.Equal(t, 0, srv.CaptureCount())
		assert.Equal(t, 0, sl.CallCount())
	})
	t.Run("nil_after_a_5xx", func(t *testing.T) {
		srv := testutil.NewMockServer(t)
		srv.On("/bot"+testutil.TestToken+"/sendPhoto", func(w http.ResponseWriter, _ *http.Request) {
			testutil.ReplyServerError(w, 500, "Internal Server Error")
		})
		sl := &testutil.FakeSleeper{}
		c := testutil.NewRetryTestClient(t, srv.BaseURL(), sl, sender.WithRetries(3))
		var calls atomic.Int32
		f := sender.InputFile{FileName: "p", Source: func() io.Reader {
			if calls.Add(1) == 1 {
				return strings.NewReader("img")
			}
			return nil
		}}
		_, err := c.SendPhoto(context.Background(), sender.SendPhotoRequest{ChatID: testutil.TestChatID, Photo: f})
		requireB63(t, err, "photo", "photo")
		assert.NotErrorIs(t, err, sender.ErrMaxRetries)
		assert.Equal(t, int32(2), calls.Load())
		assert.Equal(t, 1, srv.CaptureCount())
		assert.Equal(t, 1, sl.CallCount())
	})
}

// TestB63_Controls: valid uploads and the old results stay (PLAN AC1, AC5).
func TestB63_Controls(t *testing.T) {
	var typedNil *bytes.Reader
	sends := map[string]sender.InputFile{
		"from_bytes":       b63Good("a"),
		"value_reader":     sender.FromReader(b63Value{}, "a"),
		"nil_slice_reader": sender.FromReader(b63Slice(nil), "a"),
		"non_nil_pointer":  sender.FromReader(bytes.NewReader([]byte("x")), "a"),
		// A non-nil pointer to a zero value is not nil (AC2, AC5; test round
		// t2, Claude A-T2-02).
		"zero_value_pointer": sender.FromReader(new(bytes.Buffer), "a"),
		// The same non-struct pointer type as typed_nil_scalar, not nil.
		"non_nil_scalar_pointer":  sender.FromReader(new(b63Scalar), "a"),
		"source_over_nil_reader":  {FileName: "a", Reader: typedNil, Source: func() io.Reader { return strings.NewReader("x") }},
		"file_id_over_nil_source": {FileID: "id", Source: func() io.Reader { return nil }},
	}
	for name, f := range sends {
		t.Run(name, func(t *testing.T) {
			srv := b63Server(t)
			c := testutil.NewTestClient(t, srv.BaseURL())
			_, err := c.SendDocument(context.Background(), sender.SendDocumentRequest{ChatID: testutil.TestChatID, Document: f})
			require.NoError(t, err)
			assert.Equal(t, 1, srv.CaptureCount())
		})
	}

	t.Run("from_reader_nil_keeps_old_error", func(t *testing.T) {
		srv := b63Server(t)
		c := testutil.NewTestClient(t, srv.BaseURL())
		_, err := c.SendDocument(context.Background(), sender.SendDocumentRequest{ChatID: testutil.TestChatID, Document: sender.FromReader(nil, "a")})
		require.EqualError(t, err, "failed to build request: field document: InputFile must have FileID, URL, or Reader set")
		var ve *tg.ValidationError
		assert.False(t, errors.As(err, &ve), "not a ValidationError")
		assert.Equal(t, 0, srv.CaptureCount())
	})
}
