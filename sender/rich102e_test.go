package sender_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 rich message media (B-25 10.2e, TEST-PLAN §3, rows T-mj-* and T-rm-*).

// api102eUploadErr is the error text of an InputMedia upload that only the multipart builder of the media list can send.
const api102eUploadErr = "sender: an InputMedia upload in a rich message is supported only in InputRichMessage.Media of " +
	"a request sent by sender.Client; use a file_id or an HTTP URL"

// api102eCounter counts how often an upload's Source is called and its Reader is read.
type api102eCounter struct{ sources, reads atomic.Int32 }

func (c *api102eCounter) source(data string) sender.InputFile {
	return sender.InputFile{FileName: "s.bin", Source: func() io.Reader {
		c.sources.Add(1)
		return strings.NewReader(data)
	}}
}

type api102eReader struct {
	c *api102eCounter
	r io.Reader
}

func (r api102eReader) Read(p []byte) (int, error) { r.c.reads.Add(1); return r.r.Read(p) }

func (c *api102eCounter) reader(data string) sender.InputFile {
	return sender.InputFile{FileName: "r.bin", Reader: api102eReader{c, strings.NewReader(data)}}
}

func (c *api102eCounter) assertUnused(t *testing.T) {
	t.Helper()
	assert.Equal(t, int32(0), c.sources.Load(), "Source calls")
	assert.Equal(t, int32(0), c.reads.Load(), "Reader reads")
}

// api102eEditItem is the item the multipart builder writes for m (editMessageMedia's media field).
func api102eEditItem(t *testing.T, m sender.InputMedia) string {
	t.Helper()
	req, err := sender.BuildMultipartRequest(sender.EditMessageMediaRequest{ChatID: api101dChat, MessageID: 1, Media: m})
	require.NoError(t, err)
	return req.Params["media"]
}

// T-mj-1: InputMedia.MarshalJSON gives exactly the item the builder writes, for every kind, for a value and a pointer.
func TestAPI102E_InputMediaJSON(t *testing.T) {
	live := sender.NewInputMediaLivePhoto(sender.FromFileID("LV"), sender.FromFileID("LP"))
	liveFull := live.WithCaption("lc").WithParseMode("HTML")
	loc := sender.NewInputMediaLocation(1.5, 2.5)
	loc.HorizontalAccuracy = 3.5
	venue := sender.NewInputMediaVenue(1.5, 2.5, "T", "A")
	venue.FoursquareID = "F"
	venueFull := venue // every existing venue field set, each with its own value (t1 O-T1-04)
	venueFull.HorizontalAccuracy = 4.5
	venueFull.FoursquareType = "FT"
	venueFull.GooglePlaceID = "G"
	venueFull.GooglePlaceType = "GT"
	sticker := sender.NewInputMediaSticker(sender.FromFileID("S")).WithCaption("sc").WithParseMode("HTML") // t3 O-T3-03
	sticker.Emoji = "e"
	rows := []struct {
		name string
		in   sender.InputMedia
		want string
	}{
		{"photo", sender.NewInputMediaPhoto(sender.FromFileID("P")).WithCaption("c").WithParseMode("HTML"),
			`{"caption":"c","media":"P","parse_mode":"HTML","type":"photo"}`},
		// each file kind with its own caption and parse mode (t2 O-T2-03)
		{"video-url", sender.NewInputMediaVideo(sender.FromURL("https://x/v.mp4")).WithCaption("vdc").WithParseMode("HTML"),
			`{"caption":"vdc","media":"https://x/v.mp4","parse_mode":"HTML","type":"video"}`},
		{"audio", sender.NewInputMediaAudio(sender.FromFileID("A")).WithCaption("auc").WithParseMode("Markdown"),
			`{"caption":"auc","media":"A","parse_mode":"Markdown","type":"audio"}`},
		{"animation", sender.NewInputMediaAnimation(sender.FromFileID("G")).WithCaption("anc").WithParseMode("MarkdownV2"),
			`{"caption":"anc","media":"G","parse_mode":"MarkdownV2","type":"animation"}`},
		{"document", sender.NewInputMediaDocument(sender.FromFileID("D")).WithCaption("dc").WithParseMode("HTML"),
			`{"caption":"dc","media":"D","parse_mode":"HTML","type":"document"}`},
		{"voice_note", sender.NewInputMediaVoiceNote(sender.FromFileID("V")).WithCaption("vc").WithParseMode("MarkdownV2"),
			`{"caption":"vc","media":"V","parse_mode":"MarkdownV2","type":"voice_note"}`},
		{"as-given", sender.NewInputMediaPhoto(sender.FromFileID(" P\t")).WithCaption("\tc "), `{"caption":"\tc ","media":" P\t","type":"photo"}`},
		{"live_photo", live, `{"media":"LV","photo":"LP","type":"live_photo"}`},
		{"live_photo-caption", liveFull, `{"caption":"lc","media":"LV","parse_mode":"HTML","photo":"LP","type":"live_photo"}`},
		{"sticker", sticker, `{"caption":"sc","emoji":"e","media":"S","parse_mode":"HTML","type":"sticker"}`},
		{"location", sender.NewInputMediaLocation(1.5, 2.5), `{"latitude":1.5,"longitude":2.5,"type":"location"}`},
		{"location-full", loc, `{"horizontal_accuracy":3.5,"latitude":1.5,"longitude":2.5,"type":"location"}`},
		{"venue", venue, `{"address":"A","foursquare_id":"F","latitude":1.5,"longitude":2.5,"title":"T","type":"venue"}`},
		{"venue-full", venueFull, `{"address":"A","foursquare_id":"F","foursquare_type":"FT","google_place_id":"G",` +
			`"google_place_type":"GT","horizontal_accuracy":4.5,"latitude":1.5,"longitude":2.5,"title":"T","type":"venue"}`},
		{"link", sender.NewInputMediaLink("https://x"), `{"type":"link","url":"https://x"}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
			outp, err := json.Marshal(&r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(outp), "pointer")
			assert.Equal(t, r.want, api102eEditItem(t, r.in), "the builder's item")
			assert.Equal(t, r.in.Type, r.in.InputRichMediaType())
		})
	}
}

// T-vn: the voice note constructor.
func TestAPI102E_VoiceNote(t *testing.T) {
	f := sender.FromFileID("V")
	assert.Equal(t, sender.InputMedia{Type: "voice_note", Media: f}, sender.NewInputMediaVoiceNote(f))
}

// T-mj-2: an upload is refused by MarshalJSON before its reader is opened: no Source call, no Read.
func TestAPI102E_InputMediaUploadRefused(t *testing.T) {
	rows := []struct {
		name string
		make func(c *api102eCounter) sender.InputMedia
	}{
		{"source", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaPhoto(c.source("x")) }},
		{"reader", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaVideo(c.reader("x")) }},
		{"from-bytes", func(*api102eCounter) sender.InputMedia {
			return sender.NewInputMediaVoiceNote(sender.FromBytes([]byte("x"), "v.ogg"))
		}},
		{"live/video", func(c *api102eCounter) sender.InputMedia {
			return sender.NewInputMediaLivePhoto(c.source("v"), sender.FromFileID("LP"))
		}},
		{"live/photo", func(c *api102eCounter) sender.InputMedia {
			return sender.NewInputMediaLivePhoto(sender.FromFileID("LV"), c.source("p"))
		}},
		{"live/photo-reader", func(c *api102eCounter) sender.InputMedia {
			return sender.NewInputMediaLivePhoto(sender.FromURL("https://x/v"), c.reader("p"))
		}},
		{"live/both", func(c *api102eCounter) sender.InputMedia {
			return sender.NewInputMediaLivePhoto(c.reader("v"), c.source("p"))
		}},
		{"photo-on-photo", func(c *api102eCounter) sender.InputMedia {
			return sender.NewInputMediaPhoto(sender.FromFileID("P").WithPhoto(c.source("p")))
		}},
		// each file kind with its own constructor (t1 O-T1-02: a kind exempted from the check must fail)
		{"video-source", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaVideo(c.source("x")) }},
		{"audio-reader", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaAudio(c.reader("x")) }},
		{"animation-source", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaAnimation(c.source("x")) }},
		{"animation-reader", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaAnimation(c.reader("x")) }},
		{"document-source", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaDocument(c.source("x")) }},
		{"voice_note-reader", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaVoiceNote(c.reader("x")) }},
		{"sticker-source", func(c *api102eCounter) sender.InputMedia { return sender.NewInputMediaSticker(c.source("x")) }},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			var c api102eCounter
			m := r.make(&c)
			for _, v := range []any{m, &m, tg.InputRichMessageMedia{ID: "x", Media: m}, tg.InputRichBlockPhoto{Photo: &m}} {
				_, err := json.Marshal(v)
				require.Error(t, err)
				assert.Contains(t, err.Error(), api102eUploadErr)
			}
			c.assertUnused(t)
		})
	}
}

// T-mj-3: a file_id or URL comes before an upload, as in the builder: not an upload, the reader is not opened.
func TestAPI102E_InputMediaNotUpload(t *testing.T) {
	var c api102eCounter
	withID := c.source("x")
	withID.FileID = "F"
	withURL := c.reader("x")
	withURL.URL = "https://x/u"
	photoID := c.source("p")
	photoID.FileID = "PF"
	rows := []struct {
		name string
		in   sender.InputMedia
		want string
	}{
		{"file_id+source", sender.NewInputMediaPhoto(withID), `{"media":"F","type":"photo"}`},
		{"url+reader", sender.NewInputMediaAudio(withURL), `{"media":"https://x/u","type":"audio"}`},
		{"live/photo-id+source", sender.NewInputMediaLivePhoto(sender.FromFileID("LV"), photoID),
			`{"media":"LV","photo":"PF","type":"live_photo"}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
		})
	}
	c.assertUnused(t)
}

// T-mj-4: the builder's existing checks apply to MarshalJSON; a location, venue or link with a file keeps its own error
// (not the upload error), and its reader is not opened.
func TestAPI102E_InputMediaErrors(t *testing.T) {
	var c api102eCounter
	loc := sender.NewInputMediaLocation(1, 2)
	loc.Media = c.source("x")
	venue := sender.NewInputMediaVenue(1, 2, "T", "A")
	venue.Media = sender.FromFileID("F")
	link := sender.NewInputMediaLink("https://x")
	link.Media = c.reader("x")
	rows := []struct {
		name string
		in   sender.InputMedia
		want string
	}{
		{"zero", sender.InputMedia{}, "InputMedia.Media must have FileID, URL, or Reader set"},
		{"photo-empty", sender.NewInputMediaPhoto(sender.InputFile{}), "InputMedia.Media must have FileID, URL, or Reader set"},
		{"live-no-photo", sender.InputMedia{Type: "live_photo", Media: sender.FromFileID("LV")}, "InputMedia: live_photo requires Media.Photo"},
		{"location-file", loc, "InputMedia: location has no media file"},
		{"venue-file", venue, "InputMedia: venue has no media file"},
		{"link-file", link, "InputMedia: link has no media file"},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			_, err := json.Marshal(r.in)
			require.Error(t, err)
			assert.Contains(t, err.Error(), r.want)
			assert.NotContains(t, err.Error(), api102eUploadErr)
		})
	}
	c.assertUnused(t)
}

// T-mj-5: a nil *InputMedia is JSON null in the media list, in a block and in inline content, and never panics.
func TestAPI102E_TypedNil(t *testing.T) {
	var p *sender.InputMedia
	rows := []struct {
		name string
		in   any
		want string
	}{
		{"item", tg.InputRichMessageMedia{ID: "n", Media: p}, `{"id":"n","media":null}`},
		{"block", tg.InputRichBlockVideo{Video: p}, `{"type":"video","video":null}`},
		{"message", tg.InputRichMessage{Markdown: "m", Media: []tg.InputRichMessageMedia{{ID: "n", Media: p}},
			Blocks: []tg.InputRichBlock{tg.InputRichBlockAudio{Audio: p}}},
			`{"markdown":"m","blocks":[{"type":"audio","audio":null}],"media":[{"id":"n","media":null}]}`},
		{"inline", tg.InputRichMessageContent{RichMessage: tg.InputRichMessage{Markdown: "m", Media: []tg.InputRichMessageMedia{{ID: "n", Media: p}}}},
			`{"rich_message":{"markdown":"m","media":[{"id":"n","media":null}]}}`},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			out, err := json.Marshal(r.in)
			require.NoError(t, err)
			assert.Equal(t, r.want, string(out))
		})
	}
}

// api102eArgs are the multipart text fields each caller sends besides rich_message.
var api102eArgs = map[string]map[string]string{
	"send":  {"chat_id": "123456789"},
	"draft": {"chat_id": "123456789", "draft_id": "4"},
	"edit":  {"chat_id": "123456789", "message_id": "9"},
}

// api102eCall runs one caller against a fresh mock server and returns the one request it sent, decoded.
func api102eCall(t *testing.T, c api102dCaller, rm tg.InputRichMessage) (string, map[string]string, map[string]wireFile) {
	t.Helper()
	server, client := api102dServer(t)
	require.NoError(t, c.call(context.Background(), client, rm))
	require.Equal(t, 1, server.CaptureCount())
	got := server.LastCapture()
	got.AssertPath(t, api101dPath(c.method))
	return wireArgs(t, capturedRequest(got))
}

// T-rm-1: uploads in the media list go multipart for each caller: the rich_message field holds the list with
// attach://fileN in list order (the ids are not sorted: an id sort would move the parts), every file is its part;
// file_id, URL, nil and absent media stay; the caller's value is unchanged (the slice, the pointed-to media and the
// live photo's photo, field by field).
func TestAPI102E_SendMediaUploads(t *testing.T) {
	for _, c := range api102dCallers {
		t.Run(c.name, func(t *testing.T) {
			audio := sender.NewInputMediaAudio(sender.FromReader(bytes.NewReader([]byte("aud")), "a.mp3")).
				WithCaption("ac").WithParseMode("HTML")
			var nilMedia *sender.InputMedia
			media := []tg.InputRichMessageMedia{
				{ID: "q", Media: sender.NewInputMediaPhoto(sender.FromFileID("P"))},
				{ID: "z", Media: sender.NewInputMediaVideo(sender.FromBytes([]byte("vid"), "v.mp4")).WithCaption("vc")},
				{ID: "a", Media: &audio},
				{ID: "m", Media: sender.NewInputMediaLivePhoto(sender.FromFileID("LV"), sender.FromBytes([]byte("ph"), "p.jpg"))},
				{ID: "e", Media: sender.NewInputMediaAnimation(sender.FromURL("https://x/g.mp4"))},
				{ID: "f", Media: nilMedia},
				{ID: "b", Media: sender.NewInputMediaVoiceNote(sender.FromBytes([]byte("vo"), "v.ogg"))},
				{ID: "h"}, // no media at all: left as it is (null)
			}
			before := make([]tg.InputRichMessageMedia, len(media))
			copy(before, media)
			reader := audio.Media.Reader // the caller's reader object (t3 O-T3-01): the image holds only "set"
			imgs := make([]api102eImg, len(media))
			for i := range media {
				imgs[i] = api102eImage(media[i].Media)
			}
			mt, args, files := api102eCall(t, c, tg.InputRichMessage{Markdown: "m", Media: media})
			assert.Equal(t, "multipart/form-data", mt)
			want := map[string]string{"rich_message": `{"markdown":"m","media":[` +
				`{"id":"q","media":{"media":"P","type":"photo"}},` +
				`{"id":"z","media":{"caption":"vc","media":"attach://file0","type":"video"}},` +
				`{"id":"a","media":{"caption":"ac","media":"attach://file1","parse_mode":"HTML","type":"audio"}},` +
				`{"id":"m","media":{"media":"LV","photo":"attach://file2","type":"live_photo"}},` +
				`{"id":"e","media":{"media":"https://x/g.mp4","type":"animation"}},` +
				`{"id":"f","media":null},` +
				`{"id":"b","media":{"media":"attach://file3","type":"voice_note"}},` +
				`{"id":"h","media":null}]}`}
			for k, v := range api102eArgs[c.name] {
				want[k] = v
			}
			assertArgs(t, want, args)
			assertFiles(t, map[string]wireFile{
				"file0": {FileName: "v.mp4", Data: []byte("vid")},
				"file1": {FileName: "a.mp3", Data: []byte("aud")},
				"file2": {FileName: "p.jpg", Data: []byte("ph")},
				"file3": {FileName: "v.ogg", Data: []byte("vo")},
			}, files)
			// The caller's slice, every item (a value or the pointed-to media) and the live photo's photo are not
			// changed, field by field (t2 O-T2-02, X-T2-01); the images are taken before the call.
			require.Len(t, media, len(before))
			for i := range media {
				assert.Equal(t, before[i].ID, media[i].ID)
				assert.IsType(t, before[i].Media, media[i].Media, "item %d", i)
				assert.Equal(t, imgs[i], api102eImage(media[i].Media), "item %d", i)
			}
			assert.Same(t, &audio, media[2].Media)
			assert.Same(t, reader, audio.Media.Reader, "the caller's reader object")
			// the images hold the fields (a check of the helper itself)
			assert.Equal(t, "ac", imgs[2].M.Caption)
			assert.True(t, imgs[2].Reader)
			require.NotNil(t, imgs[3].Photo)
			assert.Equal(t, "p.jpg", imgs[3].Photo.FileName)
			assert.True(t, imgs[3].PhotoSource)
			assert.Equal(t, "P", imgs[0].M.Media.FileID)
		})
	}
}

// api102eImg is the comparable form of a rich message media item for T-rm-1's caller check: every InputMedia field,
// the file's Reader and Source as set or not set (a func is not comparable, and the builder reads the reader), and
// the live photo's photo as its own copy (a shallow copy would share it).
type api102eImg struct {
	Kind                     string
	M                        sender.InputMedia
	Reader, Source           bool
	Photo                    *sender.InputFile
	PhotoReader, PhotoSource bool
}

func api102eImage(media any) api102eImg {
	var m sender.InputMedia
	switch v := media.(type) {
	case sender.InputMedia:
		m = v
	case *sender.InputMedia:
		if v == nil {
			return api102eImg{Kind: "nil *InputMedia"}
		}
		m = *v
	default:
		return api102eImg{Kind: fmt.Sprintf("%T", media)}
	}
	img := api102eImg{Kind: fmt.Sprintf("%T", media), Reader: m.Media.Reader != nil, Source: m.Media.Source != nil}
	m.Media.Reader, m.Media.Source = nil, nil
	if ph := m.Media.Photo; ph != nil {
		cp := *ph
		img.PhotoReader, img.PhotoSource = cp.Reader != nil, cp.Source != nil
		cp.Reader, cp.Source = nil, nil
		img.Photo = &cp
		m.Media.Photo = nil
	}
	img.M = m
	return img
}

// T-rm-7 (t2 O-T2-01, t3 O-T3-02 X-T3-01): the message flags survive the multipart path, each alone with each text
// form, both together, and with blocks, for each caller.
func TestAPI102E_SendMediaFlags(t *testing.T) {
	list := `"media":[{"id":"a","media":{"media":"attach://file0","type":"photo"}}]}`
	for _, c := range api102dCallers {
		for _, f := range []struct {
			name string
			rm   tg.InputRichMessage
			want string
		}{
			{"is_rtl", tg.InputRichMessage{Markdown: "m", IsRTL: true}, `{"markdown":"m","is_rtl":true,` + list},
			{"skip_entity_detection", tg.InputRichMessage{HTML: "h", SkipEntityDetection: true},
				`{"html":"h","skip_entity_detection":true,` + list},
			{"is_rtl-html", tg.InputRichMessage{HTML: "h", IsRTL: true}, `{"html":"h","is_rtl":true,` + list},
			{"skip_entity_detection-markdown", tg.InputRichMessage{Markdown: "m", SkipEntityDetection: true},
				`{"markdown":"m","skip_entity_detection":true,` + list},
			{"both", tg.InputRichMessage{Markdown: "m", IsRTL: true, SkipEntityDetection: true},
				`{"markdown":"m","is_rtl":true,"skip_entity_detection":true,` + list},
			{"is_rtl-blocks", tg.InputRichMessage{Blocks: []tg.InputRichBlock{tg.InputRichBlockDivider{}}, IsRTL: true},
				`{"is_rtl":true,"blocks":[{"type":"divider"}],` + list},
		} {
			t.Run(c.name+"/"+f.name, func(t *testing.T) {
				f.rm.Media = []tg.InputRichMessageMedia{{ID: "a", Media: sender.NewInputMediaPhoto(sender.FromBytes([]byte("p"), "p.jpg"))}}
				mt, args, files := api102eCall(t, c, f.rm)
				assert.Equal(t, "multipart/form-data", mt)
				want := map[string]string{"rich_message": f.want}
				for k, v := range api102eArgs[c.name] {
					want[k] = v
				}
				assertArgs(t, want, args)
				assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: []byte("p")}}, files)
			})
		}
	}
}

// T-rm-2: a FromBytes upload can be sent twice (a fresh reader per build); a photo-only live photo is an upload.
func TestAPI102E_SendMediaTwice(t *testing.T) {
	for _, c := range api102dCallers {
		t.Run(c.name, func(t *testing.T) {
			rm := tg.InputRichMessage{HTML: "h", Media: []tg.InputRichMessageMedia{
				{ID: "l", Media: sender.NewInputMediaLivePhoto(sender.FromFileID("LV"), sender.FromBytes([]byte("ph"), "p.jpg"))}}}
			for range 2 {
				mt, args, files := api102eCall(t, c, rm)
				assert.Equal(t, "multipart/form-data", mt)
				assert.Equal(t, `{"html":"h","media":[{"id":"l","media":{"media":"LV","photo":"attach://file0","type":"live_photo"}}]}`,
					args["rich_message"])
				assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: []byte("ph")}}, files)
			}
		})
	}
}

// T-rm-3: without an upload the request stays JSON: a list of file_ids and URLs, and a message without media (the
// 10.2d body).
func TestAPI102E_SendMediaNoUpload(t *testing.T) {
	for _, c := range api102dCallers {
		t.Run(c.name, func(t *testing.T) {
			var nilMedia *sender.InputMedia
			for _, r := range []struct {
				name string
				rm   tg.InputRichMessage
				rich string
			}{
				{"file-ids", tg.InputRichMessage{Markdown: "m", Media: []tg.InputRichMessageMedia{
					{ID: "a", Media: sender.NewInputMediaPhoto(sender.FromFileID("P"))},
					{ID: "b", Media: sender.NewInputMediaVideo(sender.FromURL("https://x/v.mp4"))},
					{ID: "n", Media: nilMedia}}},
					`{"markdown":"m","media":[{"id":"a","media":{"media":"P","type":"photo"}},{"id":"b","media":{"media":"https://x/v.mp4","type":"video"}},{"id":"n","media":null}]}`},
				{"no-media", tg.InputRichMessage{Blocks: api102dBlocks}, `{"blocks":` + api102dBlocksJSON + `}`},
				{"empty-media", tg.InputRichMessage{HTML: "<b>&</b>", Media: []tg.InputRichMessageMedia{}}, `{"html":"\u003cb\u003e\u0026\u003c/b\u003e"}`},
			} {
				t.Run(r.name, func(t *testing.T) {
					server, client := api102dServer(t)
					require.NoError(t, c.call(context.Background(), client, r.rm))
					require.Equal(t, 1, server.CaptureCount())
					got := server.LastCapture()
					got.AssertContentType(t, "application/json")
					assert.Equal(t, c.body(r.rich), got.BodyString())
				})
			}
		})
	}
}

// T-rm-4 (AC6): an upload anywhere but the media list is refused before any request, and its reader is not opened.
func TestAPI102E_UploadElsewhereRefused(t *testing.T) {
	for _, c := range api102dCallers {
		for _, k := range []string{"animation", "audio", "photo", "video", "voice_note", "in-collage", "beside-list-upload"} {
			t.Run(c.name+"/"+k, func(t *testing.T) {
				var cnt api102eCounter
				var b tg.InputRichBlock // each block holds an upload of its own kind (t1 O-T1-02)
				switch k {
				case "animation":
					b = tg.InputRichBlockAnimation{Animation: sender.NewInputMediaAnimation(cnt.source("x"))}
				case "audio":
					b = tg.InputRichBlockAudio{Audio: sender.NewInputMediaAudio(cnt.reader("x"))}
				case "photo", "beside-list-upload":
					b = tg.InputRichBlockPhoto{Photo: sender.NewInputMediaPhoto(cnt.source("x"))}
				case "video":
					v := sender.NewInputMediaVideo(cnt.source("x"))
					b = tg.InputRichBlockVideo{Video: &v}
				case "voice_note":
					b = tg.InputRichBlockVoiceNote{VoiceNote: sender.NewInputMediaVoiceNote(cnt.reader("x"))}
				case "in-collage":
					b = tg.InputRichBlockCollage{Blocks: []tg.InputRichBlock{
						tg.InputRichBlockAnimation{Animation: sender.NewInputMediaAnimation(cnt.reader("x"))}}}
				}
				rm := tg.InputRichMessage{Blocks: []tg.InputRichBlock{b}}
				if k == "beside-list-upload" {
					rm.Media = []tg.InputRichMessageMedia{{ID: "u", Media: sender.NewInputMediaPhoto(sender.FromBytes([]byte("p"), "p.jpg"))}}
				}
				server, client := api102dServer(t)
				err := c.call(context.Background(), client, rm)
				require.Error(t, err)
				assert.Contains(t, err.Error(), api102eUploadErr)
				assert.Equal(t, 0, server.CaptureCount())
				cnt.assertUnused(t)
			})
		}
	}
	for _, k := range []string{"list", "block", "list-voice_note", "block-animation"} {
		t.Run("inline/"+k, func(t *testing.T) {
			var cnt api102eCounter
			rm := tg.InputRichMessage{Markdown: "m"}
			switch k {
			case "list":
				rm.Media = []tg.InputRichMessageMedia{{ID: "u", Media: sender.NewInputMediaPhoto(cnt.source("x"))}}
			case "block":
				rm.Blocks = []tg.InputRichBlock{tg.InputRichBlockPhoto{Photo: sender.NewInputMediaPhoto(cnt.reader("x"))}}
			case "list-voice_note":
				rm.Media = []tg.InputRichMessageMedia{{ID: "u", Media: sender.NewInputMediaVoiceNote(cnt.reader("x"))}}
			case "block-animation":
				rm.Blocks = []tg.InputRichBlock{tg.InputRichBlockAnimation{Animation: sender.NewInputMediaAnimation(cnt.source("x"))}}
			}
			server := testutil.NewMockServer(t)
			api101dOnResult(server, "answerInlineQuery", "true")
			client := testutil.NewTestClient(t, server.BaseURL())
			err := client.AnswerInlineQuery(context.Background(), sender.AnswerInlineQueryRequest{InlineQueryID: "q",
				Results: []tg.InlineQueryResult{&tg.InlineQueryResultArticle{Type: "article", ID: "1", Title: "t",
					InputMessageContent: tg.InputRichMessageContent{RichMessage: rm}}}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), api102eUploadErr)
			assert.Equal(t, 0, server.CaptureCount())
			cnt.assertUnused(t)
		})
	}
}

// T-rm-5: a bad item in the media list is refused with its index before any request.
func TestAPI102E_ListItemError(t *testing.T) {
	for _, c := range api102dCallers {
		t.Run(c.name, func(t *testing.T) {
			server, client := api102dServer(t)
			err := c.call(context.Background(), client, tg.InputRichMessage{Markdown: "m", Media: []tg.InputRichMessageMedia{
				{ID: "a", Media: sender.NewInputMediaPhoto(sender.FromBytes([]byte("p"), "p.jpg"))},
				{ID: "b", Media: sender.InputMedia{Type: "video"}}}})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "media 1: InputMedia.Media must have FileID, URL, or Reader set")
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}

// T-rm-6 (AC7): an upload request keeps the send path: one per-chat limiter, and it waits for the per-chat and the
// global limit (a second upload with a short deadline is refused before it is sent), no retry, the breaker counts it.
func TestAPI102E_PathUploads(t *testing.T) {
	rm := tg.InputRichMessage{Markdown: "m", Media: []tg.InputRichMessageMedia{
		{ID: "a", Media: sender.NewInputMediaPhoto(sender.FromBytes([]byte("p"), "p.jpg"))}}}
	reply := map[string]func(http.ResponseWriter){
		"sendRichMessage":      func(w http.ResponseWriter) { testutil.ReplyMessage(w, 4242) },
		"sendRichMessageDraft": func(w http.ResponseWriter) { testutil.ReplyBool(w, true) },
		"editMessageText":      func(w http.ResponseWriter) { testutil.ReplyMessage(w, 4242) },
	}
	for _, c := range api102dCallers {
		t.Run(c.name+"/limiter", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) { reply[c.method](w) })
			client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()),
				sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			require.Equal(t, 0, client.ChatLimiterCount())
			require.NoError(t, c.call(context.Background(), client, rm))
			assert.Equal(t, 1, client.ChatLimiterCount())
			assert.Equal(t, 1, server.CaptureCount())
			server.LastCapture().AssertContentType(t, "multipart/form-data")
		})
		t.Run(c.name+"/shares", func(t *testing.T) { // t2 O-T2-04: the chat's own limiter, not another key
			server := testutil.NewMockServer(t)
			server.On(api101dPath("sendMessage"), func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyMessage(w, 4241) })
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) { reply[c.method](w) })
			client, err := sender.New(testutil.TestToken, sender.WithBaseURL(server.BaseURL()),
				sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0),
				sender.WithPerChatRateLimit(0.01, 1))
			require.NoError(t, err)
			t.Cleanup(func() { _ = client.Close() })
			_, err = client.SendMessage(context.Background(), sender.SendMessageRequest{ChatID: api101dChat, Text: "x"})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err = c.call(ctx, client, rm)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "would exceed context deadline")
			assert.Equal(t, 1, server.CaptureCount(), "only the SendMessage request")
		})
		for _, w := range []struct {
			name string
			opts []sender.Option
		}{
			{"per-chat-wait", []sender.Option{sender.WithPerChatRateLimit(0.001, 1), sender.WithRateLimit(1000, 100)}},
			{"global-wait", []sender.Option{sender.WithRateLimit(0.001, 1), sender.WithPerChatRateLimit(1000, 100)}},
		} {
			t.Run(c.name+"/"+w.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) { reply[c.method](w) })
				client, err := sender.New(testutil.TestToken, append([]sender.Option{sender.WithBaseURL(server.BaseURL()),
					sender.WithCircuitBreakerSettings(testutil.CircuitBreakerNeverTrip()), sender.WithRetries(0)}, w.opts...)...)
				require.NoError(t, err)
				t.Cleanup(func() { _ = client.Close() })
				require.NoError(t, c.call(context.Background(), client, rm))
				ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
				defer cancel()
				require.Error(t, c.call(ctx, client, rm), "the second upload must wait for the limit")
				assert.Equal(t, 1, server.CaptureCount(), "the second upload is not sent")
			})
		}
		t.Run(c.name+"/no-retry", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			var n atomic.Int32
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) {
				if n.Add(1) == 1 {
					testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
					return
				}
				reply[c.method](w)
			})
			sleeper := &testutil.FakeSleeper{}
			client := testutil.NewRetryTestClient(t, server.BaseURL(), sleeper, sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
			err := c.call(context.Background(), client, rm)
			var apiErr *tg.APIError
			require.True(t, errors.As(err, &apiErr), "error %v", err)
			assert.Equal(t, 502, apiErr.Code)
			assert.Equal(t, 1, server.CaptureCount())
			assert.Equal(t, 0, sleeper.CallCount())
		})
		t.Run(c.name+"/opens", func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On(api101dPath(c.method), func(w http.ResponseWriter, _ *http.Request) {
				testutil.ReplyServerError(w, http.StatusBadGateway, "Bad Gateway")
			})
			client := testutil.NewBreakerTestClient(t, server.BaseURL(), sender.WithRateLimit(1000, 100),
				sender.WithGroupRateLimit(1000, 100), sender.WithPerChatRateLimit(1000, 100))
			for i := 1; i <= 2; i++ {
				require.Error(t, c.call(context.Background(), client, rm))
				assert.Equal(t, i, server.CaptureCount())
			}
			err := client.DeleteMessage(context.Background(), sender.DeleteMessageRequest{ChatID: api101dChat, MessageID: 1})
			assert.True(t, errors.Is(err, gobreaker.ErrOpenState), "the failed upload calls opened the breaker: %v", err)
		})
	}
}

// T-val: the media list is not a form: media alone is refused, media with a form is sent.
func TestAPI102E_MediaNotAForm(t *testing.T) {
	media := []tg.InputRichMessageMedia{{ID: "a", Media: sender.NewInputMediaPhoto(sender.FromFileID("P"))}}
	for _, c := range api102dCallers {
		t.Run(c.name, func(t *testing.T) {
			server, client := api102dServer(t)
			err := c.call(context.Background(), client, tg.InputRichMessage{Media: media})
			var ve *tg.ValidationError
			require.True(t, errors.As(err, &ve), "error %v", err)
			assert.Equal(t, "rich_message", ve.Field)
			assert.Equal(t, "html, markdown or blocks required", ve.Message)
			assert.Equal(t, 0, server.CaptureCount())
			require.NoError(t, c.call(context.Background(), client, tg.InputRichMessage{Blocks: api102dBlocks, Media: media}))
			assert.Equal(t, 1, server.CaptureCount())
		})
	}
}

// T-zero (plan D2): an edit with a zero InputMedia now fails locally with the builder's error; nothing is sent.
func TestAPI102E_ZeroEditMedia(t *testing.T) {
	server := testutil.NewMockServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())
	_, err := client.EditMessageMedia(context.Background(), sender.EditMessageMediaRequest{ChatID: api101dChat, MessageID: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InputMedia.Media must have FileID, URL, or Reader set")
	err = client.EditEphemeralMessageMedia(context.Background(), sender.EditEphemeralMessageMediaRequest{ChatID: api101dChat,
		ReceiverUserID: 7, EphemeralMessageID: 3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InputMedia.Media must have FileID, URL, or Reader set")
	assert.Equal(t, 0, server.CaptureCount())
}
