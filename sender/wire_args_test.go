package sender_test

// B-45 wire tests: what the Bot API server reads from the request galigo sends.
// The server builds one argument list from a JSON body or from a
// multipart/form-data body; parseWire rebuilds that list, so a row can compare
// the whole argument map, not one field.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// wireFile is one file part of a multipart body.
type wireFile struct {
	FileName string
	Data     []byte
}

// jsonParams are the keys whose value the server parses as JSON.
var jsonParams = map[string]bool{
	"media":                true,
	"reply_markup":         true,
	"entities":             true,
	"caption_entities":     true,
	"link_preview_options": true,
	"reply_parameters":     true,
}

// parseWire returns the media type, the arguments and the file parts of a
// request, as the Bot API server reads them. A JSON string value becomes its
// decoded text; any other JSON value (null included) keeps its raw text.
func parseWire(r *http.Request) (string, map[string]string, map[string]wireFile, error) {
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return "", nil, nil, fmt.Errorf("content type: %w", err)
	}
	args := map[string]string{}
	files := map[string]wireFile{}

	switch mediaType {
	case "application/json":
		dec := json.NewDecoder(r.Body)
		tok, err := dec.Token()
		if err != nil {
			return mediaType, nil, nil, fmt.Errorf("json: %w", err)
		}
		if d, ok := tok.(json.Delim); !ok || d != '{' {
			return mediaType, nil, nil, errors.New("json: body is not an object")
		}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return mediaType, nil, nil, fmt.Errorf("json key: %w", err)
			}
			key, ok := kt.(string)
			if !ok {
				return mediaType, nil, nil, fmt.Errorf("json key: %v is not a string", kt)
			}
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return mediaType, nil, nil, fmt.Errorf("json value of %s: %w", key, err)
			}
			if _, dup := args[key]; dup {
				return mediaType, nil, nil, fmt.Errorf("json: duplicate key %s", key)
			}
			if len(raw) > 0 && raw[0] == '"' {
				var s string
				if err := json.Unmarshal(raw, &s); err != nil {
					return mediaType, nil, nil, fmt.Errorf("json string %s: %w", key, err)
				}
				args[key] = s
			} else {
				args[key] = string(raw)
			}
		}
		if _, err := dec.Token(); err != nil {
			return mediaType, nil, nil, fmt.Errorf("json end: %w", err)
		}
		if _, err := dec.Token(); !errors.Is(err, io.EOF) {
			return mediaType, nil, nil, errors.New("json: data after the object")
		}

	case "multipart/form-data":
		boundary := params["boundary"]
		if boundary == "" {
			return mediaType, nil, nil, errors.New("multipart: no boundary")
		}
		mr := multipart.NewReader(r.Body, boundary)
		for {
			p, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return mediaType, nil, nil, fmt.Errorf("multipart: %w", err)
			}
			name := p.FormName()
			data, err := io.ReadAll(p)
			if err != nil {
				return mediaType, nil, nil, fmt.Errorf("multipart part %s: %w", name, err)
			}
			if p.FileName() != "" {
				if _, dup := files[name]; dup {
					return mediaType, nil, nil, fmt.Errorf("multipart: duplicate file %s", name)
				}
				files[name] = wireFile{FileName: p.FileName(), Data: data}
				continue
			}
			if _, dup := args[name]; dup {
				return mediaType, nil, nil, fmt.Errorf("multipart: duplicate field %s", name)
			}
			args[name] = string(data)
		}
		for name := range files {
			if _, both := args[name]; both {
				return mediaType, nil, nil, fmt.Errorf("multipart: %s is both a file and a field", name)
			}
		}

	default:
		return mediaType, nil, nil, fmt.Errorf("unsupported media type %q", mediaType)
	}
	return mediaType, args, files, nil
}

// wireArgs is parseWire that stops the test on an error.
func wireArgs(t *testing.T, r *http.Request) (string, map[string]string, map[string]wireFile) {
	t.Helper()
	mediaType, args, files, err := parseWire(r)
	if err != nil {
		t.Fatalf("parseWire: %v", err)
	}
	return mediaType, args, files
}

// argsDiff lists every difference between two argument maps (empty: equal).
// Keys in jsonParams are compared as decoded JSON; every other key as text.
func argsDiff(want, got map[string]string) []string {
	keys := map[string]bool{}
	for k := range want {
		keys[k] = true
	}
	for k := range got {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	var diff []string
	for _, k := range sorted {
		w, inWant := want[k]
		g, inGot := got[k]
		switch {
		case !inGot:
			diff = append(diff, fmt.Sprintf("%s: missing (want %q)", k, w))
		case !inWant:
			diff = append(diff, fmt.Sprintf("%s: extra %q", k, g))
		case jsonParams[k]:
			var wv, gv any
			we := json.Unmarshal([]byte(w), &wv)
			ge := json.Unmarshal([]byte(g), &gv)
			if we != nil || ge != nil || !reflect.DeepEqual(wv, gv) {
				diff = append(diff, fmt.Sprintf("%s: got %q, want JSON %q", k, g, w))
			}
		case w != g:
			diff = append(diff, fmt.Sprintf("%s: got %q, want %q", k, g, w))
		}
	}
	return diff
}

func assertArgs(t *testing.T, want, got map[string]string) {
	t.Helper()
	for _, d := range argsDiff(want, got) {
		t.Errorf("args: %s", d)
	}
}

// filesDiff lists every difference between two file maps (empty: equal).
func filesDiff(want, got map[string]wireFile) []string {
	var diff []string
	for k, w := range want {
		g, ok := got[k]
		switch {
		case !ok:
			diff = append(diff, fmt.Sprintf("file %s: missing", k))
		case g.FileName != w.FileName || !bytes.Equal(g.Data, w.Data):
			diff = append(diff, fmt.Sprintf("file %s: got %q %q, want %q %q", k, g.FileName, g.Data, w.FileName, w.Data))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			diff = append(diff, fmt.Sprintf("file %s: extra", k))
		}
	}
	sort.Strings(diff)
	return diff
}

func assertFiles(t *testing.T, want, got map[string]wireFile) {
	t.Helper()
	for _, d := range filesDiff(want, got) {
		t.Errorf("files: %s", d)
	}
}

// capturedRequest turns a captured request back into an *http.Request.
func capturedRequest(c *testutil.Capture) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(c.Body))
	r.Header.Set("Content-Type", c.ContentType)
	return r
}

// wireCall runs call against a fresh mock server (reply answers the method)
// and returns the one request it sent, decoded. A call that sends no request,
// or more than one, fails the test.
func wireCall(t *testing.T, method string, reply http.HandlerFunc, call func(ctx context.Context, c *sender.Client) error) (string, map[string]string, map[string]wireFile, error) {
	t.Helper()
	server := testutil.NewMockServer(t)
	if reply != nil {
		server.On("/bot"+testutil.TestToken+"/"+method, reply)
	}
	client := testutil.NewTestClient(t, server.BaseURL())
	callErr := call(context.Background(), client)
	require.Equal(t, 1, server.CaptureCount(), "requests sent (call error: %v)", callErr)
	c := server.LastCapture()
	require.Equal(t, "/bot"+testutil.TestToken+"/"+method, c.Path)
	require.Empty(t, c.Query, "no argument in the URL query") // B-46: a URL argument is not in the body
	mediaType, args, files := wireArgs(t, capturedRequest(c))
	return mediaType, args, files, callErr
}

func replyMessage(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 42) }

func replyAlbum(w http.ResponseWriter, _ *http.Request) {
	testutil.ReplyOK(w, []*tg.Message{testutil.TestMessage(1, ""), testutil.TestMessage(2, "")})
}

// mpRequest builds a multipart request with the parts write adds.
func mpRequest(t *testing.T, write func(w *multipart.Writer)) (*http.Request, []byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	write(mw)
	require.NoError(t, mw.Close())
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(buf.Bytes()))
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r, buf.Bytes(), mw.Boundary()
}

func jsonRequest(body, contentType string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	return r
}

func writeFilePart(t *testing.T, mw *multipart.Writer, field, name, data string) {
	t.Helper()
	fw, err := mw.CreateFormFile(field, name)
	require.NoError(t, err)
	_, err = fw.Write([]byte(data))
	require.NoError(t, err)
}

// chatName is a named string type, a valid tg.ChatID value.
type chatName string

// escMarshaler marshals to a JSON string with an escape.
type escMarshaler struct{}

func (escMarshaler) MarshalJSON() ([]byte, error) { return []byte(`"a\"b"`), nil }

// ==================== helper controls (H1–H4) ====================

func TestWireHelper_H1_JSON(t *testing.T) {
	mediaType, args, files := wireArgs(t, jsonRequest(`{"a":"x","b":1,"c":{"k":"v"},"d":null}`, "application/json"))
	assert.Equal(t, "application/json", mediaType)
	assert.Equal(t, map[string]string{"a": "x", "b": "1", "c": `{"k":"v"}`, "d": "null"}, args)
	assert.Empty(t, files)
}

func TestWireHelper_H2_Multipart(t *testing.T) {
	r, _, _ := mpRequest(t, func(mw *multipart.Writer) {
		require.NoError(t, mw.WriteField("chat_id", "7"))
		writeFilePart(t, mw, "photo", "a.jpg", "bytes")
	})
	mediaType, args, files := wireArgs(t, r)
	assert.Equal(t, "multipart/form-data", mediaType)
	assert.Equal(t, map[string]string{"chat_id": "7"}, args)
	assert.Equal(t, map[string]wireFile{"photo": {FileName: "a.jpg", Data: []byte("bytes")}}, files)
}

func TestWireHelper_H2b_FileNotParam(t *testing.T) {
	r1, _, _ := mpRequest(t, func(mw *multipart.Writer) {
		require.NoError(t, mw.WriteField("photo", "FILE:photo:a.jpg"))
	})
	r2, _, _ := mpRequest(t, func(mw *multipart.Writer) {
		writeFilePart(t, mw, "photo", "a.jpg", "FILE:photo:a.jpg")
	})
	_, args1, files1 := wireArgs(t, r1)
	_, args2, files2 := wireArgs(t, r2)
	assert.NotEmpty(t, argsDiff(args1, args2))
	assert.NotEmpty(t, filesDiff(files1, files2))
}

func TestWireHelper_H3_Compare(t *testing.T) {
	rejects := []struct {
		name      string
		want, got map[string]string
	}{
		{"missing key", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1"}},
		{"extra key", map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}},
		{"media string vs object", map[string]string{"media": `["A"]`}, map[string]string{"media": `[{"media":"A"}]`}},
		{"quoted number", map[string]string{"chat_id": `"1"`}, map[string]string{"chat_id": "1"}},
		{"quoted chat name", map[string]string{"chat_id": `"@chan"`}, map[string]string{"chat_id": "@chan"}},
		{"media array order", map[string]string{"media": `[{"media":"A"},{"media":"B"}]`}, map[string]string{"media": `[{"media":"B"},{"media":"A"}]`}},
		{"caption is text", map[string]string{"caption": `{"a":1,"b":2}`}, map[string]string{"caption": `{"b":2,"a":1}`}},
	}
	for _, tt := range rejects {
		t.Run("rejects/"+tt.name, func(t *testing.T) {
			assert.NotEmpty(t, argsDiff(tt.want, tt.got))
		})
	}
	accepts := []struct {
		name      string
		want, got map[string]string
	}{
		{"reply_markup key order", map[string]string{"reply_markup": `{"b":1,"a":2}`}, map[string]string{"reply_markup": `{"a":2,"b":1}`}},
		{"media item key order", map[string]string{"media": `[{"type":"photo","media":"A"}]`}, map[string]string{"media": `[{"media":"A","type":"photo"}]`}},
	}
	for _, tt := range accepts {
		t.Run("accepts/"+tt.name, func(t *testing.T) {
			assert.Empty(t, argsDiff(tt.want, tt.got))
		})
	}
	t.Run("files/rejects other bytes", func(t *testing.T) {
		assert.NotEmpty(t, filesDiff(map[string]wireFile{"f": {"a", []byte("1")}}, map[string]wireFile{"f": {"a", []byte("2")}}))
	})
}

func TestWireHelper_H4_Errors(t *testing.T) {
	cases := map[string]func(t *testing.T) *http.Request{
		"json duplicate key": func(t *testing.T) *http.Request {
			return jsonRequest(`{"a":"1","a":"2"}`, "application/json")
		},
		"json malformed": func(t *testing.T) *http.Request {
			return jsonRequest(`{"a":`, "application/json")
		},
		"text/plain": func(t *testing.T) *http.Request {
			return jsonRequest(`a=1`, "text/plain")
		},
		"multipart same field twice": func(t *testing.T) *http.Request {
			r, _, _ := mpRequest(t, func(mw *multipart.Writer) {
				require.NoError(t, mw.WriteField("a", "1"))
				require.NoError(t, mw.WriteField("a", "2"))
			})
			return r
		},
		"multipart two files same field": func(t *testing.T) *http.Request {
			r, _, _ := mpRequest(t, func(mw *multipart.Writer) {
				writeFilePart(t, mw, "f", "a.txt", "1")
				writeFilePart(t, mw, "f", "b.txt", "2")
			})
			return r
		},
		"multipart file and field same name": func(t *testing.T) *http.Request {
			r, _, _ := mpRequest(t, func(mw *multipart.Writer) {
				writeFilePart(t, mw, "f", "a.txt", "1")
				require.NoError(t, mw.WriteField("f", "2"))
			})
			return r
		},
		"multipart cut off": func(t *testing.T) *http.Request {
			r, body, boundary := mpRequest(t, func(mw *multipart.Writer) {
				require.NoError(t, mw.WriteField("a", "1"))
			})
			closing := "\r\n--" + boundary + "--\r\n"
			require.True(t, bytes.HasSuffix(body, []byte(closing)))
			cut := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body[:len(body)-len(closing)]))
			cut.Header.Set("Content-Type", r.Header.Get("Content-Type"))
			return cut
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := parseWire(build(t))
			assert.Error(t, err)
		})
	}
}

// ==================== wire rows (W1–W14) ====================

var wireChatID = fmt.Sprint(testutil.TestChatID)

func TestWire_W1_MediaGroupFileID(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{
			ChatID: testutil.TestChatID,
			Media: []sender.InputFile{
				sender.FromFileID("A").WithMediaType("photo").WithCaption("cap").WithParseMode("HTML"),
				sender.FromFileID("B").WithMediaType("photo"),
			},
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id": wireChatID,
		"media":   `[{"type":"photo","media":"A","caption":"cap","parse_mode":"HTML"},{"type":"photo","media":"B"}]`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestWire_W2_MediaGroupURL(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{
			ChatID: testutil.TestChatID,
			Media: []sender.InputFile{
				sender.FromURL("https://e.example/a").WithMediaType("document"),
				sender.FromURL("https://e.example/b").WithMediaType("document"),
			},
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id": wireChatID,
		"media":   `[{"type":"document","media":"https://e.example/a"},{"type":"document","media":"https://e.example/b"}]`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestWire_W3_MediaGroupMixed(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{
			ChatID: testutil.TestChatID,
			Media: []sender.InputFile{
				{MediaType: "photo", FileID: "A", Caption: "one", ParseMode: "HTML"},
				{MediaType: "video", URL: "https://e.example/v.mp4", Caption: "two", ParseMode: "MarkdownV2"},
			},
			DisableNotification: true,
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id":              wireChatID,
		"disable_notification": "true",
		"media":                `[{"type":"photo","media":"A","caption":"one","parse_mode":"HTML"},{"type":"video","media":"https://e.example/v.mp4","caption":"two","parse_mode":"MarkdownV2"}]`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

// W4 is TestEditMessageMedia_FileID (sender/edit_media_test.go).

func TestWire_W5_EditInline(t *testing.T) {
	// The real inline reply is "result": true; EditMessageMedia returns
	// (nil, nil) for it (B-47, the result rows are in inline_edit_b47_test.go).
	mediaType, args, files, err := wireCall(t, "editMessageMedia", func(w http.ResponseWriter, _ *http.Request) {
		testutil.ReplyOK(w, true)
	}, func(ctx context.Context, c *sender.Client) error {
		_, err := c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{
			InlineMessageID: "inl-1",
			Media:           sender.NewInputMediaDocument(sender.FromURL("https://e.example/d")),
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"inline_message_id": "inl-1",
		"media":             `{"type":"document","media":"https://e.example/d"}`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestWire_W6_EditReplyMarkup(t *testing.T) {
	markup := testutil.TestInlineKeyboard([]tg.InlineKeyboardButton{testutil.TestInlineButton("a", "b")})
	markupJSON, marshalErr := json.Marshal(markup)
	require.NoError(t, marshalErr)
	mediaType, args, files, err := wireCall(t, "editMessageMedia", replyMessage, func(ctx context.Context, c *sender.Client) error {
		_, err := c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{
			ChatID:      testutil.TestChatID,
			MessageID:   42,
			Media:       sender.NewInputMediaPhoto(sender.FromFileID("A")),
			ReplyMarkup: markup,
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id":      wireChatID,
		"message_id":   "42",
		"media":        `{"type":"photo","media":"A"}`,
		"reply_markup": string(markupJSON),
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

// fileCall is one file-taking call with file_id input.
type fileCall struct {
	name, method string
	ref          any    // the request the call encodes; nil for builder-based methods
	wantType     string // the media type the request must have
	call         func(ctx context.Context, c *sender.Client) error
}

func fileCalls() []fileCall {
	const js, mp = "application/json", "multipart/form-data"
	id := testutil.TestChatID
	sticker := sender.InputSticker{Sticker: sender.FromFileID("A"), Format: "static", EmojiList: []string{"😀"}}
	group := sender.SendMediaGroupRequest{ChatID: id, Media: []sender.InputFile{
		sender.FromFileID("A").WithMediaType("photo"), sender.FromFileID("B").WithMediaType("photo"),
	}}
	edit := sender.EditMessageMediaRequest{ChatID: id, MessageID: 42, Media: sender.NewInputMediaPhoto(sender.FromFileID("A"))}
	eph := sender.EditEphemeralMessageMediaRequest{ChatID: id, ReceiverUserID: 5000000001, EphemeralMessageID: 4242,
		Media: sender.NewInputMediaPhoto(sender.FromFileID("A"))} // API102C T-w7
	photo := sender.SendPhotoRequest{ChatID: id, Photo: sender.FromFileID("A"), Caption: "c"}
	document := sender.SendDocumentRequest{ChatID: id, Document: sender.FromFileID("A"), Caption: "c"}
	video := sender.SendVideoRequest{ChatID: id, Video: sender.FromFileID("A"), Caption: "c"}
	audio := sender.SendAudioRequest{ChatID: id, Audio: sender.FromFileID("A"), Caption: "c"}
	animation := sender.SendAnimationRequest{ChatID: id, Animation: sender.FromFileID("A"), Caption: "c"}
	voice := sender.SendVoiceRequest{ChatID: id, Voice: sender.FromFileID("A"), Caption: "c"}
	videoNote := sender.SendVideoNoteRequest{ChatID: id, VideoNote: sender.FromFileID("A")}
	stickerSend := sender.SendStickerRequest{ChatID: id, Sticker: sender.FromFileID("A")}
	chatPhoto := sender.SetChatPhotoRequest{ChatID: id, Photo: sender.FromFileID("A")}
	upload := sender.UploadStickerFileRequest{UserID: 1, Sticker: sender.FromFileID("A"), StickerFormat: "static"}
	thumb := sender.SetStickerSetThumbnailRequest{Name: "n", UserID: 1, Thumbnail: sender.FromFileID("A"), Format: "static"}
	// API100B T-s-19: the three live-photo calls by file ids.
	live := sender.SendLivePhotoRequest{ChatID: id, LivePhoto: sender.FromFileID("A"), Photo: sender.FromFileID("B"), Caption: "c"}
	liveGroup := sender.SendMediaGroupRequest{ChatID: id, Media: []sender.InputFile{
		sender.FromFileID("A").WithMediaType("live_photo").WithPhoto(sender.FromFileID("B")),
		sender.FromFileID("C").WithMediaType("live_photo").WithPhoto(sender.FromFileID("D")),
	}}
	liveEdit := sender.EditMessageMediaRequest{ChatID: id, MessageID: 42,
		Media: sender.NewInputMediaLivePhoto(sender.FromFileID("A"), sender.FromFileID("B"))}
	// API100D T-s-14: sendPoll with description media and option media by file id.
	pollPhoto, optionPhoto := sender.NewInputMediaPhoto(sender.FromFileID("A")), sender.NewInputMediaPhoto(sender.FromFileID("B"))
	poll := sender.SendPollRequest{ChatID: id, Question: "Q", Media: &pollPhoto,
		Options: []sender.InputPollOption{{Text: "a", Media: &optionPhoto}, {Text: "b"}}}
	// B-48b2: sendPaidMedia by file ids (always a form).
	paid := sender.SendPaidMediaRequest{ChatID: id, StarCount: 1, Media: []sender.InputPaidMedia{
		sender.NewInputPaidMediaPhoto(sender.FromFileID("A")),
		sender.NewInputPaidMediaLivePhoto(sender.FromFileID("B"), sender.FromFileID("C")),
	}}

	msg := func(f func() (*tg.Message, error)) error { _, err := f(); return err }
	return []fileCall{
		{"sendPhoto", "sendPhoto", photo, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendPhoto(ctx, photo) })
		}},
		{"sendDocument", "sendDocument", document, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendDocument(ctx, document) })
		}},
		{"sendVideo", "sendVideo", video, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendVideo(ctx, video) })
		}},
		{"sendAudio", "sendAudio", audio, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendAudio(ctx, audio) })
		}},
		{"sendAnimation", "sendAnimation", animation, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendAnimation(ctx, animation) })
		}},
		{"sendVoice", "sendVoice", voice, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendVoice(ctx, voice) })
		}},
		{"sendVideoNote", "sendVideoNote", videoNote, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendVideoNote(ctx, videoNote) })
		}},
		{"sendSticker", "sendSticker", stickerSend, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendSticker(ctx, stickerSend) })
		}},
		{"sendMediaGroup", "sendMediaGroup", group, mp, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendMediaGroup(ctx, group)
			return err
		}},
		{"editMessageMedia", "editMessageMedia", edit, mp, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.EditMessageMedia(ctx, edit) })
		}},
		{"editEphemeralMessageMedia", "editEphemeralMessageMedia", eph, mp, func(ctx context.Context, c *sender.Client) error { // API102C T-w7
			return c.EditEphemeralMessageMedia(ctx, eph)
		}},
		{"sendPoll", "sendPoll", poll, mp, func(ctx context.Context, c *sender.Client) error { // API100D T-s-14
			return msg(func() (*tg.Message, error) { return c.SendPoll(ctx, poll) })
		}},
		{"sendPaidMedia", "sendPaidMedia", paid, mp, func(ctx context.Context, c *sender.Client) error { // B-48b2
			return msg(func() (*tg.Message, error) { return c.SendPaidMedia(ctx, paid) })
		}},
		{"sendLivePhoto", "sendLivePhoto", live, js, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.SendLivePhoto(ctx, live) })
		}},
		{"sendMediaGroup live", "sendMediaGroup", liveGroup, mp, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendMediaGroup(ctx, liveGroup)
			return err
		}},
		{"editMessageMedia live", "editMessageMedia", liveEdit, mp, func(ctx context.Context, c *sender.Client) error {
			return msg(func() (*tg.Message, error) { return c.EditMessageMedia(ctx, liveEdit) })
		}},
		{"setChatPhoto", "setChatPhoto", chatPhoto, js, func(ctx context.Context, c *sender.Client) error {
			return c.SetChatPhoto(ctx, id, sender.FromFileID("A"))
		}},
		{"setMyProfilePhoto", "setMyProfilePhoto", nil, js, func(ctx context.Context, c *sender.Client) error { // B-46: InputProfilePhoto
			return c.SetMyProfilePhoto(ctx, &sender.InputProfilePhotoStatic{Photo: sender.FromFileID("A")})
		}},
		{"uploadStickerFile", "uploadStickerFile", upload, js, func(ctx context.Context, c *sender.Client) error {
			_, err := c.UploadStickerFile(ctx, upload)
			return err
		}},
		{"setStickerSetThumbnail", "setStickerSetThumbnail", thumb, js, func(ctx context.Context, c *sender.Client) error {
			return c.SetStickerSetThumbnail(ctx, thumb)
		}},
		// Builder-based methods: the encoded struct is internal, so no ref.
		{"createNewStickerSet", "createNewStickerSet", nil, js, func(ctx context.Context, c *sender.Client) error {
			return c.CreateNewStickerSet(ctx, sender.CreateNewStickerSetRequest{
				UserID: 1, Name: "n_by_bot", Title: "t", Stickers: []sender.InputSticker{sticker},
			})
		}},
		{"addStickerToSet", "addStickerToSet", nil, js, func(ctx context.Context, c *sender.Client) error {
			return c.AddStickerToSet(ctx, sender.AddStickerToSetRequest{UserID: 1, Name: "n", Sticker: sticker})
		}},
		{"replaceStickerInSet", "replaceStickerInSet", nil, js, func(ctx context.Context, c *sender.Client) error {
			return c.ReplaceStickerInSet(ctx, sender.ReplaceStickerInSetRequest{UserID: 1, Name: "n", OldSticker: "old", Sticker: sticker})
		}},
		{"postStory", "postStory", nil, js, func(ctx context.Context, c *sender.Client) error {
			_, err := c.PostStory(ctx, sender.PostStoryRequest{
				BusinessConnectionID: "bc", Content: &sender.InputStoryContentPhoto{Photo: sender.FromFileID("A")},
			})
			return err
		}},
		{"editStory", "editStory", nil, js, func(ctx context.Context, c *sender.Client) error {
			_, err := c.EditStory(ctx, sender.EditStoryRequest{
				BusinessConnectionID: "bc", StoryID: 1, Content: &sender.InputStoryContentPhoto{Photo: sender.FromFileID("A")},
			})
			return err
		}},
		{"setBusinessAccountProfilePhoto", "setBusinessAccountProfilePhoto", nil, js, func(ctx context.Context, c *sender.Client) error {
			return c.SetBusinessAccountProfilePhoto(ctx, sender.SetBusinessAccountProfilePhotoRequest{
				BusinessConnectionID: "bc", Photo: &sender.InputProfilePhotoStatic{Photo: sender.FromFileID("A")},
			})
		}},
	}
}

// W7 is a routing check: both sides share the encoder, so the literal rows
// (W1–W6, W12) are the proof of the values.
func TestWire_W7_Routing(t *testing.T) {
	n := 0
	for _, fc := range fileCalls() {
		if fc.ref == nil {
			continue
		}
		n++
		t.Run(fc.name, func(t *testing.T) {
			ref, err := sender.BuildMultipartRequest(fc.ref)
			require.NoError(t, err)
			require.Empty(t, ref.Files, "file_id input must not upload")
			_, args, files, _ := wireCall(t, fc.method, nil, fc.call)
			assertArgs(t, ref.Params, args)
			assertFiles(t, map[string]wireFile{}, files)
		})
	}
	assert.Equal(t, 19, n, "W7 rows") // API100B T-s-19: 14 + 3 live-photo rows; API100D T-s-14: + sendPoll; API102C T-w7: + editEphemeralMessageMedia; B-46: - setMyProfilePhoto (builder payload); B-48b2: + sendPaidMedia
}

func TestWire_W8_ContentType(t *testing.T) {
	calls := fileCalls()
	require.Len(t, calls, 26) // API100B T-s-19: 20 + 3 live-photo rows; API100D T-s-14: + sendPoll; API102C T-w7: + editEphemeralMessageMedia; B-48b2: + sendPaidMedia
	for _, fc := range calls {
		t.Run(fc.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL())
			_ = fc.call(context.Background(), client)
			require.Equal(t, 1, server.CaptureCount())
			c := server.LastCapture()
			if fc.wantType == "application/json" {
				assert.Equal(t, "application/json", c.ContentType)
				return
			}
			mediaType, _, err := mime.ParseMediaType(c.ContentType)
			require.NoError(t, err)
			assert.Equal(t, fc.wantType, mediaType)
		})
	}
}

// W7 completeness guard: every file-taking type in sender/ is a W7 row (part
// A) or a listed builder request or nested value type (part B).
var guardPartA = []string{
	"SendPhotoRequest", "SendDocumentRequest", "SendVideoRequest", "SendAudioRequest",
	"SendAnimationRequest", "SendVoiceRequest", "SendVideoNoteRequest", "SendStickerRequest",
	"SendMediaGroupRequest", "EditMessageMediaRequest", "SetChatPhotoRequest",
	"UploadStickerFileRequest", "SetStickerSetThumbnailRequest",
	"SendLivePhotoRequest",             // API100B T-s-19
	"SendPollRequest",                  // API100D T-s-14
	"EditEphemeralMessageMediaRequest", // API102C T-w7
	"SendPaidMediaRequest",             // B-48b2
}

var guardPartB = []string{
	"CreateNewStickerSetRequest", "AddStickerToSetRequest", "ReplaceStickerInSetRequest",
	"PostStoryRequest", "EditStoryRequest", "SetBusinessAccountProfilePhotoRequest",
	"SetMyProfilePhotoRequest", // B-46: InputProfilePhoto, built by resolveProfilePhoto
	"InputMedia", "InputSticker", "InputProfilePhotoStatic", "InputProfilePhotoAnimated",
	"InputStoryContentPhoto", "InputStoryContentVideo",
	"InputFile",       // API100B T-s-19: InputFile.Photo is a nested *InputFile
	"InputPollOption", // API100D T-s-14: InputPollOption.Media is a nested *InputMedia
	"InputPaidMedia",  // B-48b2: sendPaidMedia's items, built by handlePaidMedia
}

// guardTypes returns the union of set 1 (exported structs with a field of a
// file type) and set 2 (exported *Request structs with a field whose type is
// an Input* interface of the package), sorted.
func guardTypes(t *testing.T, sources map[string]string) []string {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, sources[name], 0)
		require.NoError(t, err, name)
		files = append(files, f)
	}

	interfaces := map[string]bool{}
	structs := map[string]*ast.StructType{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			switch tt := ts.Type.(type) {
			case *ast.InterfaceType:
				interfaces[ts.Name.Name] = true
			case *ast.StructType:
				if ts.Name.IsExported() {
					structs[ts.Name.Name] = tt
				}
			}
			return true
		})
	}

	fileTypes := map[string]bool{"InputFile": true, "InputMedia": true, "InputSticker": true, "InputPaidMedia": true} // B-48b2: InputPaidMedia
	isFileType := func(e ast.Expr) bool {
		switch x := e.(type) {
		case *ast.Ident:
			return fileTypes[x.Name]
		case *ast.StarExpr:
			id, ok := x.X.(*ast.Ident)
			return ok && (id.Name == "InputFile" || id.Name == "InputMedia")
		case *ast.ArrayType:
			id, ok := x.Elt.(*ast.Ident)
			return ok && x.Len == nil && fileTypes[id.Name]
		}
		return false
	}

	union := map[string]bool{}
	for name, st := range structs {
		for _, field := range st.Fields.List {
			if isFileType(field.Type) {
				union[name] = true
			}
			if id, ok := field.Type.(*ast.Ident); ok && strings.HasSuffix(name, "Request") &&
				strings.HasPrefix(id.Name, "Input") && interfaces[id.Name] {
				union[name] = true
			}
		}
	}
	out := make([]string, 0, len(union))
	for name := range union {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func senderSources(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)
	sources := map[string]string{}
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		data, err := os.ReadFile(p)
		require.NoError(t, err)
		sources[p] = string(data)
	}
	require.NotEmpty(t, sources)
	return sources
}

func TestWire_W7_Guard(t *testing.T) {
	want := append(append([]string{}, guardPartA...), guardPartB...)
	sort.Strings(want)
	require.Len(t, want, 33) // API100B T-s-19: 26 + SendLivePhotoRequest + InputFile; API100D T-s-14: + SendPollRequest + InputPollOption; API102C T-w7: + EditEphemeralMessageMediaRequest; B-48b2: + SendPaidMediaRequest + InputPaidMedia

	assert.Equal(t, want, guardTypes(t, senderSources(t)))

	t.Run("control: a new file-taking request fails the guard", func(t *testing.T) {
		sources := senderSources(t)
		sources["zz_control.go"] = "package sender\n\ntype XRequest struct{ F InputFile }\n"
		assert.NotEqual(t, want, guardTypes(t, sources))
	})
}

func TestWire_W9_UploadsUnchanged(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{
			ChatID: testutil.TestChatID,
			Media: []sender.InputFile{
				sender.FromBytes([]byte("b45-w9"), "a.txt").WithMediaType("document"),
				sender.FromFileID("B").WithMediaType("document"),
			},
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id": wireChatID,
		"media":   `[{"type":"document","media":"attach://file0"},{"type":"document","media":"B"}]`,
	}, args)
	assertFiles(t, map[string]wireFile{"file0": {FileName: "a.txt", Data: []byte("b45-w9")}}, files)
}

func TestWire_W10_OtherArguments(t *testing.T) {
	const keyboard = `{"inline_keyboard":[]}`
	t.Run("sendMediaGroup/namedChat", func(t *testing.T) {
		_, args, _, err := wireCall(t, "sendMediaGroup", replyAlbum, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{
				ChatID: chatName("@chan"),
				Media: []sender.InputFile{
					sender.FromFileID("A").WithMediaType("photo"), sender.FromFileID("B").WithMediaType("photo"),
				},
			})
			return err
		})
		require.NoError(t, err)
		assertArgs(t, map[string]string{
			"chat_id": "@chan",
			"media":   `[{"type":"photo","media":"A"},{"type":"photo","media":"B"}]`,
		}, args)
	})
	edits := []struct {
		name   string
		chat   tg.ChatID
		markup any
		want   map[string]string
	}{
		{"namedChat", chatName("@chan"), nil, map[string]string{"chat_id": "@chan"}},
		{"rawStringMarkup", testutil.TestChatID, json.RawMessage(`"{\"inline_keyboard\":[]}"`), map[string]string{"chat_id": wireChatID, "reply_markup": keyboard}},
		{"stringMarkup", testutil.TestChatID, keyboard, map[string]string{"chat_id": wireChatID, "reply_markup": keyboard}},
		{"rawObjectMarkup", testutil.TestChatID, json.RawMessage(keyboard), map[string]string{"chat_id": wireChatID, "reply_markup": keyboard}},
	}
	for _, tt := range edits {
		t.Run("editMessageMedia/"+tt.name, func(t *testing.T) {
			_, args, _, err := wireCall(t, "editMessageMedia", replyMessage, func(ctx context.Context, c *sender.Client) error {
				_, err := c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{
					ChatID:      tt.chat,
					MessageID:   42,
					Media:       sender.NewInputMediaPhoto(sender.FromFileID("A")),
					ReplyMarkup: tt.markup,
				})
				return err
			})
			require.NoError(t, err)
			want := map[string]string{"message_id": "42", "media": `{"type":"photo","media":"A"}`}
			for k, v := range tt.want {
				want[k] = v
			}
			assertArgs(t, want, args)
		})
	}
}

func TestWire_W11_UploadRawStringMarkup(t *testing.T) {
	const keyboard = `{"inline_keyboard":[[{"text":"a","callback_data":"b"}]]}`
	inner, marshalErr := json.Marshal(keyboard)
	require.NoError(t, marshalErr)
	mediaType, args, files, err := wireCall(t, "sendPhoto", replyMessage, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendPhoto(ctx, sender.SendPhotoRequest{
			ChatID:      testutil.TestChatID,
			Photo:       sender.FromBytes([]byte("p"), "p.jpg"),
			ReplyMarkup: json.RawMessage(inner),
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{"chat_id": wireChatID, "reply_markup": keyboard}, args)
	assertFiles(t, map[string]wireFile{"photo": {FileName: "p.jpg", Data: []byte("p")}}, files)
}

func TestWire_W12_MediaGroupOptionalFields(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{
			ChatID: testutil.TestChatID,
			Media: []sender.InputFile{
				sender.FromFileID("A").WithMediaType("photo"), sender.FromFileID("B").WithMediaType("photo"),
			},
			DisableNotification: true,
			ProtectContent:      true,
			ReplyToMessageID:    77,
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id":              wireChatID,
		"media":                `[{"type":"photo","media":"A"},{"type":"photo","media":"B"}]`,
		"disable_notification": "true",
		"protect_content":      "true",
		"reply_to_message_id":  "77",
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestWire_W13_PlainStringStaysText(t *testing.T) {
	const caption = `"q\"x"`
	_, args, files, err := wireCall(t, "sendPhoto", replyMessage, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendPhoto(ctx, sender.SendPhotoRequest{
			ChatID:  testutil.TestChatID,
			Photo:   sender.FromBytes([]byte("p"), "p.jpg"),
			Caption: caption,
		})
		return err
	})
	require.NoError(t, err)
	assertArgs(t, map[string]string{"chat_id": wireChatID, "caption": caption}, args)
	assertFiles(t, map[string]wireFile{"photo": {FileName: "p.jpg", Data: []byte("p")}}, files)
}

func TestWire_W14_EditNullMarkup(t *testing.T) {
	mediaType, args, files, err := wireCall(t, "editMessageMedia", replyMessage, func(ctx context.Context, c *sender.Client) error {
		_, err := c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{
			ChatID:      testutil.TestChatID,
			MessageID:   42,
			Media:       sender.NewInputMediaPhoto(sender.FromFileID("A")),
			ReplyMarkup: json.RawMessage("null"),
		})
		return err
	})
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id":      wireChatID,
		"message_id":   "42",
		"media":        `{"type":"photo","media":"A"}`,
		"reply_markup": "null",
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

// ==================== encoder default branch (D1–D8) ====================
// D4 is TestBuildMultipartRequest_ComplexType (sender/multipart_test.go).

func TestBuildMultipartRequest_Default_D1_NamedStringInAny(t *testing.T) {
	got, err := sender.BuildMultipartRequest(struct {
		V any `json:"v"`
	}{chatName("@chan")})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"v": "@chan"}, got.Params)
}

func TestBuildMultipartRequest_Default_D2_RawString(t *testing.T) {
	got, err := sender.BuildMultipartRequest(struct {
		V any `json:"v"`
	}{json.RawMessage(`"x"`)})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"v": "x"}, got.Params)
}

func TestBuildMultipartRequest_Default_D3_RawObject(t *testing.T) {
	got, err := sender.BuildMultipartRequest(struct {
		V any `json:"v"`
	}{json.RawMessage(`{"k":"v"}`)})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"v": `{"k":"v"}`}, got.Params)
}

func TestBuildMultipartRequest_Default_D5_DefinedScalars(t *testing.T) {
	type num int64
	type flag bool
	type ratio float64
	got, err := sender.BuildMultipartRequest(struct {
		N any `json:"n"`
		F any `json:"f"`
		R any `json:"r"`
		L any `json:"l"`
	}{num(123), flag(true), ratio(1.5), []int{1, 2}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"n": "123", "f": "true", "r": "1.5", "l": "[1,2]"}, got.Params)
}

func TestBuildMultipartRequest_Default_D5b_BuiltinScalars(t *testing.T) {
	got, err := sender.BuildMultipartRequest(struct {
		N any `json:"n"`
		F any `json:"f"`
		R any `json:"r"`
	}{int64(123), true, 1.5})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"n": "123", "f": "true", "r": "1.5"}, got.Params)
}

func TestBuildMultipartRequest_Default_D6_MarshalJSONString(t *testing.T) {
	got, err := sender.BuildMultipartRequest(struct {
		V any `json:"v"`
	}{escMarshaler{}})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"v": `a"b`}, got.Params)
}

func TestBuildMultipartRequest_Default_D7_PlainStringUnchanged(t *testing.T) {
	got, err := sender.BuildMultipartRequest(struct {
		A string `json:"a"`
		B string `json:"b"`
	}{`"hello"`, `"a\"b"`})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": `"hello"`, "b": `"a\"b"`}, got.Params)
}

func TestBuildMultipartRequest_Default_D8_NullStaysNull(t *testing.T) {
	var nilKeyboard *tg.InlineKeyboardMarkup
	got, err := sender.BuildMultipartRequest(struct {
		R any `json:"r,omitempty"`
		P any `json:"p,omitempty"`
	}{json.RawMessage("null"), nilKeyboard})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"r": "null", "p": "null"}, got.Params)
}

// ================== Bot API 10.0 part b: live photos in albums and edits (B-25 10.0b, TEST-PLAN §3) ==================
// Rows T-s-9 … T-s-18, T-s-22 … T-s-24. Every expected media value is a literal.

var (
	lpV1 = []byte("video-one")
	lpP1 = []byte("photo-one")
	lpV2 = []byte("video-two")
	lpP2 = []byte("photo-two")
)

func liveItem(video, photo sender.InputFile) sender.InputFile {
	return video.WithMediaType("live_photo").WithPhoto(photo)
}

func sendGroup(media ...sender.InputFile) func(ctx context.Context, c *sender.Client) error {
	return func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendMediaGroup(ctx, sender.SendMediaGroupRequest{ChatID: testutil.TestChatID, Media: media})
		return err
	}
}

func editMedia(m sender.InputMedia) func(ctx context.Context, c *sender.Client) error {
	return func(ctx context.Context, c *sender.Client) error {
		_, err := c.EditMessageMedia(ctx, sender.EditMessageMediaRequest{ChatID: testutil.TestChatID, MessageID: 42, Media: m})
		return err
	}
}

// noRequest runs call on a fresh mock server and returns its error; the call must send nothing.
func noRequest(t *testing.T, call func(ctx context.Context, c *sender.Client) error) error {
	t.Helper()
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())
	err := call(context.Background(), client)
	assert.Equal(t, 0, server.CaptureCount(), "no request sent")
	return err
}

func TestWire_W15_LiveAlbumFileID(t *testing.T) { // API100B T-s-9
	mediaType, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(
		sender.FromFileID("V1").WithMediaType("live_photo").WithCaption("c").WithParseMode("HTML").WithPhoto(sender.FromFileID("P1")),
		liveItem(sender.FromFileID("V2"), sender.FromFileID("P2")),
	))
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	assertArgs(t, map[string]string{
		"chat_id": wireChatID,
		"media": `[{"type":"live_photo","media":"V1","photo":"P1","caption":"c","parse_mode":"HTML"},` +
			`{"type":"live_photo","media":"V2","photo":"P2"}]`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestWire_W16_LiveAlbumUploads(t *testing.T) { // API100B T-s-10
	_, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(
		liveItem(sender.FromBytes(lpV1, "v1.mp4"), sender.FromBytes(lpP1, "p1.jpg")),
		liveItem(sender.FromBytes(lpV2, "v2.mp4"), sender.FromBytes(lpP2, "p2.jpg")),
	))
	require.NoError(t, err)
	assertArgs(t, map[string]string{
		"chat_id": wireChatID,
		"media": `[{"type":"live_photo","media":"attach://file0","photo":"attach://file1"},` +
			`{"type":"live_photo","media":"attach://file2","photo":"attach://file3"}]`,
	}, args)
	assertFiles(t, map[string]wireFile{
		"file0": {FileName: "v1.mp4", Data: lpV1},
		"file1": {FileName: "p1.jpg", Data: lpP1},
		"file2": {FileName: "v2.mp4", Data: lpV2},
		"file3": {FileName: "p2.jpg", Data: lpP2},
	}, files)
}

func TestWire_W17_LiveAlbumMixed(t *testing.T) { // API100B T-s-11
	// Item 0 has no upload, so the photo upload of item 3 takes attach index 2, not its item index 3 (M14).
	_, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(
		liveItem(sender.FromFileID("V0"), sender.FromFileID("P0")),
		liveItem(sender.FromBytes(lpV1, "v1.mp4"), sender.FromFileID("P1")),
		sender.FromBytes(lpP1, "plain.jpg").WithMediaType("photo"),
		liveItem(sender.FromFileID("V3"), sender.FromBytes(lpP2, "p3.jpg")),
	))
	require.NoError(t, err)
	assertArgs(t, map[string]string{
		"chat_id": wireChatID,
		"media": `[{"type":"live_photo","media":"V0","photo":"P0"},` +
			`{"type":"live_photo","media":"attach://file0","photo":"P1"},` +
			`{"type":"photo","media":"attach://file1"},` +
			`{"type":"live_photo","media":"V3","photo":"attach://file2"}]`,
	}, args)
	assertFiles(t, map[string]wireFile{
		"file0": {FileName: "v1.mp4", Data: lpV1},
		"file1": {FileName: "plain.jpg", Data: lpP1},
		"file2": {FileName: "p3.jpg", Data: lpP2},
	}, files)
}

func TestWire_W18_LiveAlbumPhotoURL(t *testing.T) { // API100B T-s-12
	_, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(
		liveItem(sender.FromFileID("V1"), sender.FromURL("https://x/p.jpg")),
		liveItem(sender.FromFileID("V2"), sender.FromFileID("P2")),
	))
	require.NoError(t, err)
	assertArgs(t, map[string]string{
		"chat_id": wireChatID,
		"media": `[{"type":"live_photo","media":"V1","photo":"https://x/p.jpg"},` +
			`{"type":"live_photo","media":"V2","photo":"P2"}]`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestWire_W19_LiveAlbumErrors(t *testing.T) { // API100B T-s-13
	t.Run("live item without Photo", func(t *testing.T) {
		err := noRequest(t, sendGroup(
			liveItem(sender.FromFileID("V1"), sender.FromFileID("P1")),
			sender.FromFileID("V2").WithMediaType("live_photo"),
		))
		require.Error(t, err)
		assert.Equal(t, "failed to build request: field media: item 1: live_photo requires Photo", err.Error())
	})
	t.Run("live item with an empty Photo", func(t *testing.T) {
		err := noRequest(t, sendGroup(liveItem(sender.FromFileID("V1"), sender.InputFile{})))
		require.Error(t, err)
		assert.Equal(t, "failed to build request: field media: item 0: photo: InputFile must have FileID, URL, or Reader set", err.Error())
	})
}

func TestWire_W20_PhotoOnPlainItem(t *testing.T) {
	t.Run("album", func(t *testing.T) { // API100B T-s-14
		_, args, _, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(
			sender.FromFileID("A").WithMediaType("photo").WithPhoto(sender.FromFileID("B")),
		))
		require.NoError(t, err)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "media": `[{"type":"photo","media":"A","photo":"B"}]`}, args)
	})
	t.Run("edit", func(t *testing.T) { // API100B T-s-14e
		_, args, _, err := wireCall(t, "editMessageMedia", replyMessage, editMedia(sender.InputMedia{
			Type: "photo", Media: sender.FromFileID("A").WithPhoto(sender.FromFileID("B")),
		}))
		require.NoError(t, err)
		assertArgs(t, map[string]string{
			"chat_id": wireChatID, "message_id": "42", "media": `{"type":"photo","media":"A","photo":"B"}`,
		}, args)
	})
}

func TestWire_W21_EditLive(t *testing.T) { // API100B T-s-15
	t.Run("by file ids", func(t *testing.T) {
		mediaType, args, files, err := wireCall(t, "editMessageMedia", replyMessage, editMedia(
			sender.NewInputMediaLivePhoto(sender.FromFileID("V1"), sender.FromFileID("P1")).WithCaption("c").WithParseMode("HTML"),
		))
		require.NoError(t, err)
		assert.Equal(t, "multipart/form-data", mediaType)
		assertArgs(t, map[string]string{
			"chat_id": wireChatID, "message_id": "42",
			"media": `{"type":"live_photo","media":"V1","photo":"P1","caption":"c","parse_mode":"HTML"}`,
		}, args)
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("uploads", func(t *testing.T) {
		_, args, files, err := wireCall(t, "editMessageMedia", replyMessage, editMedia(
			sender.NewInputMediaLivePhoto(sender.FromBytes(lpV1, "v.mp4"), sender.FromBytes(lpP1, "p.jpg")),
		))
		require.NoError(t, err)
		assertArgs(t, map[string]string{
			"chat_id": wireChatID, "message_id": "42",
			"media": `{"type":"live_photo","media":"attach://file0","photo":"attach://file1"}`,
		}, args)
		assertFiles(t, map[string]wireFile{
			"file0": {FileName: "v.mp4", Data: lpV1},
			"file1": {FileName: "p.jpg", Data: lpP1},
		}, files)
	})
}

func TestWire_W22_EditLivePhotoURL(t *testing.T) { // API100B T-s-15u
	_, args, files, err := wireCall(t, "editMessageMedia", replyMessage, editMedia(
		sender.NewInputMediaLivePhoto(sender.FromFileID("V1"), sender.FromURL("https://x/p.jpg")),
	))
	require.NoError(t, err)
	assertArgs(t, map[string]string{
		"chat_id": wireChatID, "message_id": "42", "media": `{"type":"live_photo","media":"V1","photo":"https://x/p.jpg"}`,
	}, args)
	assertFiles(t, map[string]wireFile{}, files)
}

func TestWire_W23_EditLiveErrors(t *testing.T) { // API100B T-s-16
	t.Run("live_photo without Media.Photo", func(t *testing.T) {
		err := noRequest(t, editMedia(sender.InputMedia{Type: "live_photo", Media: sender.FromFileID("V1")}))
		require.Error(t, err)
		assert.Equal(t, "failed to build request: field media: InputMedia: live_photo requires Media.Photo", err.Error())
	})
	t.Run("live_photo with an empty Media.Photo", func(t *testing.T) {
		err := noRequest(t, editMedia(sender.NewInputMediaLivePhoto(sender.FromFileID("V1"), sender.InputFile{})))
		require.Error(t, err)
		assert.Equal(t, "failed to build request: field media: InputMedia.Media.Photo: InputFile must have FileID, URL, or Reader set", err.Error())
	})
}

func TestWire_W24_LiveConstructors(t *testing.T) { // API100B T-s-17
	p := sender.FromFileID("P1")
	m := sender.NewInputMediaLivePhoto(sender.FromFileID("V1"), p)
	assert.Equal(t, sender.InputMedia{Type: "live_photo", Media: sender.InputFile{FileID: "V1", Photo: &sender.InputFile{FileID: "P1"}}}, m)

	f := sender.FromFileID("V2")
	g := f.WithPhoto(p)
	assert.Nil(t, f.Photo, "WithPhoto returns a copy")
	assert.Equal(t, sender.InputFile{FileID: "V2", Photo: &sender.InputFile{FileID: "P1"}}, g)

	p.FileID = "CHANGED"
	require.NotNil(t, m.Media.Photo)
	require.NotNil(t, g.Photo)
	assert.Equal(t, "P1", m.Media.Photo.FileID, "a later change to photo does not reach the item")
	assert.Equal(t, "P1", g.Photo.FileID, "a later change to photo does not reach the copy")
}

func TestWire_W25_PhotoIgnoredElsewhere(t *testing.T) { // API100B T-s-18 (a control: it may be green on the stub)
	t.Run("sendPhoto by id", func(t *testing.T) {
		pa := sender.FromFileID("B")
		in := sender.FromFileID("A")
		in.Photo = &pa
		require.NotNil(t, in.Photo)
		mediaType, args, files, err := wireCall(t, "sendPhoto", replyMessage, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPhoto(ctx, sender.SendPhotoRequest{ChatID: testutil.TestChatID, Photo: in})
			return err
		})
		require.NoError(t, err)
		assert.Equal(t, "application/json", mediaType)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "photo": "A"}, args)
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("sendPhoto upload", func(t *testing.T) {
		pa := sender.FromBytes(lpP2, "inner.jpg")
		in := sender.FromBytes(lpP1, "outer.jpg")
		in.Photo = &pa
		require.NotNil(t, in.Photo)
		_, args, files, err := wireCall(t, "sendPhoto", replyMessage, func(ctx context.Context, c *sender.Client) error {
			_, err := c.SendPhoto(ctx, sender.SendPhotoRequest{ChatID: testutil.TestChatID, Photo: in})
			return err
		})
		require.NoError(t, err)
		assertArgs(t, map[string]string{"chat_id": wireChatID}, args)
		assertFiles(t, map[string]wireFile{"photo": {FileName: "outer.jpg", Data: lpP1}}, files)
	})
	t.Run("MarshalJSON", func(t *testing.T) {
		pa := sender.FromFileID("B")
		in := sender.FromFileID("A")
		in.Photo = &pa
		b, err := json.Marshal(in)
		require.NoError(t, err)
		assert.Equal(t, `"A"`, string(b))
	})
}

func TestWire_W26_SourcePhotoReread(t *testing.T) { // API100B T-s-22
	t.Run("album", func(t *testing.T) {
		req := sender.SendMediaGroupRequest{ChatID: testutil.TestChatID, Media: []sender.InputFile{
			liveItem(sender.FromFileID("V1"), sender.FromBytes(lpP1, "p.jpg")),
		}}
		for i := range 2 {
			_, _, files, err := wireCall(t, "sendMediaGroup", replyAlbum, func(ctx context.Context, c *sender.Client) error {
				_, err := c.SendMediaGroup(ctx, req)
				return err
			})
			require.NoError(t, err, "send %d", i)
			assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: lpP1}}, files)
		}
		require.NotNil(t, req.Media[0].Photo)
		assert.NotNil(t, req.Media[0].Photo.Source)
	})
	t.Run("edit", func(t *testing.T) {
		m := sender.NewInputMediaLivePhoto(sender.FromBytes(lpV1, "v.mp4"), sender.FromBytes(lpP1, "p.jpg"))
		for i := range 2 {
			_, _, files, err := wireCall(t, "editMessageMedia", replyMessage, editMedia(m))
			require.NoError(t, err, "send %d", i)
			assertFiles(t, map[string]wireFile{
				"file0": {FileName: "v.mp4", Data: lpV1},
				"file1": {FileName: "p.jpg", Data: lpP1},
			}, files)
		}
		require.NotNil(t, m.Media.Photo)
		assert.NotNil(t, m.Media.Photo.Source)
	})
}

func TestWire_W27_ReaderPhoto(t *testing.T) { // API100B T-s-23
	t.Run("album reader only", func(t *testing.T) {
		_, _, files, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(
			liveItem(sender.FromFileID("V1"), sender.FromReader(bytes.NewReader(lpP2), "r.jpg")),
		))
		require.NoError(t, err)
		assertFiles(t, map[string]wireFile{"file0": {FileName: "r.jpg", Data: lpP2}}, files)
	})
	t.Run("edit reader only", func(t *testing.T) {
		_, _, files, err := wireCall(t, "editMessageMedia", replyMessage, editMedia(
			sender.NewInputMediaLivePhoto(sender.FromFileID("V1"), sender.FromReader(bytes.NewReader(lpP2), "r.jpg")),
		))
		require.NoError(t, err)
		assertFiles(t, map[string]wireFile{"file0": {FileName: "r.jpg", Data: lpP2}}, files)
	})
	t.Run("Source wins over Reader", func(t *testing.T) {
		both := sender.FromBytes(lpP1, "s.jpg")
		both.Reader = bytes.NewReader(lpP2)
		_, _, files, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(liveItem(sender.FromFileID("V1"), both)))
		require.NoError(t, err)
		assertFiles(t, map[string]wireFile{"file0": {FileName: "s.jpg", Data: lpP1}}, files)
	})
}

func TestWire_W28_SecondLevelPhotoIgnored(t *testing.T) { // API100B T-s-24
	byID := func() sender.InputFile {
		p := sender.FromFileID("P1")
		n := sender.FromFileID("NESTED")
		p.Photo = &n
		return p
	}
	upload := func() sender.InputFile {
		p := sender.FromBytes(lpP1, "p.jpg")
		nf := sender.FromBytes(lpP2, "n.jpg")
		p.Photo = &nf
		return p
	}
	t.Run("album by id", func(t *testing.T) {
		_, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(liveItem(sender.FromFileID("V1"), byID())))
		require.NoError(t, err)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "media": `[{"type":"live_photo","media":"V1","photo":"P1"}]`}, args)
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("edit by id", func(t *testing.T) {
		_, args, files, err := wireCall(t, "editMessageMedia", replyMessage, editMedia(sender.NewInputMediaLivePhoto(sender.FromFileID("V1"), byID())))
		require.NoError(t, err)
		assertArgs(t, map[string]string{
			"chat_id": wireChatID, "message_id": "42", "media": `{"type":"live_photo","media":"V1","photo":"P1"}`,
		}, args)
		assertFiles(t, map[string]wireFile{}, files)
	})
	t.Run("album upload", func(t *testing.T) {
		_, args, files, err := wireCall(t, "sendMediaGroup", replyAlbum, sendGroup(liveItem(sender.FromFileID("V1"), upload())))
		require.NoError(t, err)
		assertArgs(t, map[string]string{"chat_id": wireChatID, "media": `[{"type":"live_photo","media":"V1","photo":"attach://file0"}]`}, args)
		assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: lpP1}}, files)
	})
	t.Run("edit upload", func(t *testing.T) {
		_, args, files, err := wireCall(t, "editMessageMedia", replyMessage, editMedia(sender.NewInputMediaLivePhoto(sender.FromFileID("V1"), upload())))
		require.NoError(t, err)
		assertArgs(t, map[string]string{
			"chat_id": wireChatID, "message_id": "42", "media": `{"type":"live_photo","media":"V1","photo":"attach://file0"}`,
		}, args)
		assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: lpP1}}, files)
	})
}
