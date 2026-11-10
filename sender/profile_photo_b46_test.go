package sender_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-46: setMyProfilePhoto takes one InputProfilePhoto (Bot API), and the
// shared resolver writes main_frame_timestamp.

func replyTrue(w http.ResponseWriter, _ *http.Request) { testutil.ReplyOK(w, true) }

// b46Upload returns an upload InputFile whose Source counts its calls.
func b46Upload(data []byte, name string, opens *int) sender.InputFile {
	return sender.InputFile{FileName: name, Source: func() io.Reader {
		*opens++
		return bytes.NewReader(data)
	}}
}

type b46Caller struct {
	name   string
	method string
	extra  map[string]string // arguments the caller adds beside photo
	call   func(ctx context.Context, c *sender.Client, p sender.InputProfilePhoto) error
}

func b46Callers() []b46Caller {
	return []b46Caller{
		{"my", "setMyProfilePhoto", map[string]string{}, func(ctx context.Context, c *sender.Client, p sender.InputProfilePhoto) error {
			return c.SetMyProfilePhoto(ctx, p)
		}},
		{"business", "setBusinessAccountProfilePhoto", map[string]string{"business_connection_id": "bc"}, func(ctx context.Context, c *sender.Client, p sender.InputProfilePhoto) error {
			return c.SetBusinessAccountProfilePhoto(ctx, sender.SetBusinessAccountProfilePhotoRequest{BusinessConnectionID: "bc", Photo: p})
		}},
	}
}

func b46With(base map[string]string, k, v string) map[string]string {
	out := map[string]string{k: v}
	for bk, bv := range base {
		out[bk] = bv
	}
	return out
}

// T-1: uploads are multipart; photo is the InputProfilePhoto JSON and
// the file is its own part, named by the attach reference.
func TestB46_Upload(t *testing.T) {
	data := []byte("\xff\xd8 jpeg bytes")
	video := []byte("mp4 bytes")
	rows := []struct {
		name      string
		photo     func(opens *int) sender.InputProfilePhoto
		wantPhoto string
		wantFiles map[string]wireFile
		wantOpens int // Source calls; 0 for a FromReader upload (no Source)
	}{
		{"static", func(o *int) sender.InputProfilePhoto {
			return &sender.InputProfilePhotoStatic{Photo: b46Upload(data, "pic 1.jpg", o)}
		}, `{"photo":"attach://profile_photo","type":"static"}`, map[string]wireFile{"profile_photo": {"pic 1.jpg", data}}, 1},
		{"static/reader", func(*int) sender.InputProfilePhoto {
			return &sender.InputProfilePhotoStatic{Photo: sender.FromReader(bytes.NewReader(data), "r.jpg")}
		}, `{"photo":"attach://profile_photo","type":"static"}`, map[string]wireFile{"profile_photo": {"r.jpg", data}}, 0},
		{"animated/reader", func(*int) sender.InputProfilePhoto {
			return &sender.InputProfilePhotoAnimated{Animation: sender.FromReader(bytes.NewReader(video), "r.mp4"), MainFrameTime: 0.5}
		}, `{"animation":"attach://profile_animation","main_frame_timestamp":0.5,"type":"animated"}`, map[string]wireFile{"profile_animation": {"r.mp4", video}}, 0},
		{"animated/frame", func(o *int) sender.InputProfilePhoto {
			return &sender.InputProfilePhotoAnimated{Animation: b46Upload(video, "a.mp4", o), MainFrameTime: 0.5}
		}, `{"animation":"attach://profile_animation","main_frame_timestamp":0.5,"type":"animated"}`, map[string]wireFile{"profile_animation": {"a.mp4", video}}, 1},
		{"animated/small", func(o *int) sender.InputProfilePhoto {
			return &sender.InputProfilePhotoAnimated{Animation: b46Upload(video, "a.mp4", o), MainFrameTime: 0.001}
		}, `{"animation":"attach://profile_animation","main_frame_timestamp":0.001,"type":"animated"}`, map[string]wireFile{"profile_animation": {"a.mp4", video}}, 1},
		{"animated/zero", func(o *int) sender.InputProfilePhoto {
			return &sender.InputProfilePhotoAnimated{Animation: b46Upload(video, "a.mp4", o)}
		}, `{"animation":"attach://profile_animation","type":"animated"}`, map[string]wireFile{"profile_animation": {"a.mp4", video}}, 1},
		{"animated/negative", func(o *int) sender.InputProfilePhoto {
			return &sender.InputProfilePhotoAnimated{Animation: b46Upload(video, "a.mp4", o), MainFrameTime: -1}
		}, `{"animation":"attach://profile_animation","type":"animated"}`, map[string]wireFile{"profile_animation": {"a.mp4", video}}, 1},
	}
	for _, c := range b46Callers() {
		for _, row := range rows {
			t.Run(c.name+"/"+row.name, func(t *testing.T) {
				opens := 0
				p := row.photo(&opens)
				mediaType, args, files, err := wireCall(t, c.method, replyTrue, func(ctx context.Context, cl *sender.Client) error {
					return c.call(ctx, cl, p)
				})
				require.NoError(t, err)
				assert.Equal(t, "multipart/form-data", mediaType)
				assertArgs(t, b46With(c.extra, "photo", row.wantPhoto), args)
				assertFiles(t, row.wantFiles, files)
				assert.Equal(t, row.wantOpens, opens, "the upload Source is opened once")
			})
		}
	}
}

// T-2: a file_id or URL is passed as given (the server refuses it: profile
// photos can only be uploaded); no upload, so the body is JSON.
func TestB46_ReferenceAsGiven(t *testing.T) {
	rows := []struct {
		name      string
		photo     sender.InputProfilePhoto
		wantPhoto string
	}{
		{"static/file_id", &sender.InputProfilePhotoStatic{Photo: sender.FromFileID(" A ")}, `{"photo":" A ","type":"static"}`},
		{"static/url", &sender.InputProfilePhotoStatic{Photo: sender.FromURL("https://x.test/p.jpg?q=1")}, `{"photo":"https://x.test/p.jpg?q=1","type":"static"}`},
		{"animated/file_id", &sender.InputProfilePhotoAnimated{Animation: sender.FromFileID("B"), MainFrameTime: 2}, `{"animation":"B","main_frame_timestamp":2,"type":"animated"}`},
		{"animated/file_id/zero", &sender.InputProfilePhotoAnimated{Animation: sender.FromFileID("B")}, `{"animation":"B","type":"animated"}`},
		{"animated/file_id/negative", &sender.InputProfilePhotoAnimated{Animation: sender.FromFileID("B"), MainFrameTime: -1}, `{"animation":"B","type":"animated"}`},
		{"animated/url", &sender.InputProfilePhotoAnimated{Animation: sender.FromURL("https://x.test/a.mp4?q=1"), MainFrameTime: 0.5}, `{"animation":"https://x.test/a.mp4?q=1","main_frame_timestamp":0.5,"type":"animated"}`},
	}
	for _, c := range b46Callers() {
		for _, row := range rows {
			t.Run(c.name+"/"+row.name, func(t *testing.T) {
				mediaType, args, files, err := wireCall(t, c.method, replyTrue, func(ctx context.Context, cl *sender.Client) error {
					return c.call(ctx, cl, row.photo)
				})
				require.NoError(t, err)
				assert.Equal(t, "application/json", mediaType)
				want := b46With(c.extra, "photo", row.wantPhoto)
				want["_file_parts"] = "null" // the existing payload shape (base business sends it too)
				assertArgs(t, want, args)
				assertFiles(t, map[string]wireFile{}, files)
			})
		}
	}
}

// T-3: a nil photo, also a typed nil pointer in the interface, is a
// validation error before any request, never a panic (E-0133).
func TestB46_NilPhoto(t *testing.T) {
	rows := []struct {
		name  string
		photo sender.InputProfilePhoto
	}{
		{"nil", nil},
		{"static/typed-nil", (*sender.InputProfilePhotoStatic)(nil)},
		{"animated/typed-nil", (*sender.InputProfilePhotoAnimated)(nil)},
	}
	for _, c := range b46Callers() {
		for _, row := range rows {
			t.Run(c.name+"/"+row.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				client := testutil.NewTestClient(t, server.BaseURL())
				var err error
				require.NotPanics(t, func() { err = c.call(context.Background(), client, row.photo) })
				var ve *tg.ValidationError
				require.True(t, errors.As(err, &ve), "error %v", err)
				assert.Equal(t, "photo", ve.Field)
				assert.Equal(t, "required", ve.Message)
				assert.Equal(t, 0, server.CaptureCount())
			})
		}
	}
}

// T-4: an empty InputFile and a value (not pointer) form keep their errors.
func TestB46_BadPhoto(t *testing.T) {
	rows := []struct {
		name  string
		photo sender.InputProfilePhoto
		want  string
	}{
		{"static/empty", &sender.InputProfilePhotoStatic{}, "photo: InputFile must have FileID, URL, or Reader set"},
		{"animated/empty", &sender.InputProfilePhotoAnimated{MainFrameTime: 1}, "animation: InputFile must have FileID, URL, or Reader set"},
		{"static/value", sender.InputProfilePhotoStatic{Photo: sender.FromFileID("A")}, "unsupported InputProfilePhoto type: sender.InputProfilePhotoStatic"},
		{"animated/value", sender.InputProfilePhotoAnimated{Animation: sender.FromFileID("A")}, "unsupported InputProfilePhoto type: sender.InputProfilePhotoAnimated"},
	}
	for _, c := range b46Callers() {
		for _, row := range rows {
			t.Run(c.name+"/"+row.name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				client := testutil.NewTestClient(t, server.BaseURL())
				err := c.call(context.Background(), client, row.photo)
				require.Error(t, err)
				assert.Equal(t, row.want, err.Error())
				assert.Equal(t, 0, server.CaptureCount())
			})
		}
	}
}

// T-5: removeMyProfilePhoto takes no parameters.
func TestB46_Remove(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/removeMyProfilePhoto", replyTrue)
	client := testutil.NewTestClient(t, server.BaseURL())
	require.NoError(t, client.RemoveMyProfilePhoto(context.Background()))
	require.Equal(t, 1, server.CaptureCount())
	c := server.LastCapture()
	assert.Equal(t, "/bot"+testutil.TestToken+"/removeMyProfilePhoto", c.Path)
	assert.Empty(t, c.Query, "no argument in the URL query")
	assert.Equal(t, "application/json", c.ContentType)
	assert.Equal(t, "{}", string(c.Body))
}

// T-6: business is_public stays beside the photo, for both forms and for a
// JSON reference; the JSON body bytes are the base bytes (AC7).
func TestB46_BusinessIsPublic(t *testing.T) {
	data := []byte("jpg")
	rows := []struct {
		name      string
		photo     sender.InputProfilePhoto
		wantPhoto string
		wantFiles map[string]wireFile
	}{
		{"static", &sender.InputProfilePhotoStatic{Photo: sender.FromBytes(data, "p.jpg")},
			`{"photo":"attach://profile_photo","type":"static"}`, map[string]wireFile{"profile_photo": {"p.jpg", data}}},
		{"animated", &sender.InputProfilePhotoAnimated{Animation: sender.FromBytes(data, "a.mp4"), MainFrameTime: 0.5},
			`{"animation":"attach://profile_animation","main_frame_timestamp":0.5,"type":"animated"}`, map[string]wireFile{"profile_animation": {"a.mp4", data}}},
		{"animated/frame-0", &sender.InputProfilePhotoAnimated{Animation: sender.FromBytes(data, "a.mp4")},
			`{"animation":"attach://profile_animation","type":"animated"}`, map[string]wireFile{"profile_animation": {"a.mp4", data}}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			mediaType, args, files, err := wireCall(t, "setBusinessAccountProfilePhoto", replyTrue, func(ctx context.Context, c *sender.Client) error {
				return c.SetBusinessAccountProfilePhoto(ctx, sender.SetBusinessAccountProfilePhotoRequest{BusinessConnectionID: "bc", IsPublic: true, Photo: row.photo})
			})
			require.NoError(t, err)
			assert.Equal(t, "multipart/form-data", mediaType)
			assertArgs(t, map[string]string{"business_connection_id": "bc", "is_public": "true", "photo": row.wantPhoto}, args)
			assertFiles(t, row.wantFiles, files)
		})
	}
	raw := []struct {
		name     string
		isPublic bool
		photo    sender.InputProfilePhoto
		want     string
	}{
		{"json/public", true, &sender.InputProfilePhotoStatic{Photo: sender.FromFileID("A")},
			`{"business_connection_id":"bc","photo":"{\"photo\":\"A\",\"type\":\"static\"}","is_public":true,"_file_parts":null}`},
		{"json/not-public", false, &sender.InputProfilePhotoStatic{Photo: sender.FromFileID("A")},
			`{"business_connection_id":"bc","photo":"{\"photo\":\"A\",\"type\":\"static\"}","_file_parts":null}`},
		{"json/animated-public", true, &sender.InputProfilePhotoAnimated{Animation: sender.FromFileID("B")},
			`{"business_connection_id":"bc","photo":"{\"animation\":\"B\",\"type\":\"animated\"}","is_public":true,"_file_parts":null}`},
		{"json/animated-frame-public", true, &sender.InputProfilePhotoAnimated{Animation: sender.FromFileID("B"), MainFrameTime: 0.5},
			`{"business_connection_id":"bc","photo":"{\"animation\":\"B\",\"main_frame_timestamp\":0.5,\"type\":\"animated\"}","is_public":true,"_file_parts":null}`},
	}
	for _, row := range raw {
		t.Run(row.name, func(t *testing.T) {
			server := testutil.NewMockServer(t)
			server.On("/bot"+testutil.TestToken+"/setBusinessAccountProfilePhoto", replyTrue)
			client := testutil.NewTestClient(t, server.BaseURL())
			require.NoError(t, client.SetBusinessAccountProfilePhoto(context.Background(), sender.SetBusinessAccountProfilePhotoRequest{
				BusinessConnectionID: "bc", IsPublic: row.isPublic, Photo: row.photo,
			}))
			require.Equal(t, 1, server.CaptureCount())
			c := server.LastCapture()
			assert.Empty(t, c.Query, "no argument in the URL query")
			assert.Equal(t, "application/json", c.ContentType)
			assert.Equal(t, row.want, string(c.Body), "the JSON body bytes (same as base 907c151)")
		})
	}
}

// T-7: one request per call; no retry on 429 or 5xx (callJSON), for the
// three calls this slice touches (Codex T3-03).
func TestB46_NoRetry(t *testing.T) {
	replies := map[string]http.HandlerFunc{
		"429": func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyRateLimit(w, 1) },
		"500": func(w http.ResponseWriter, _ *http.Request) { testutil.ReplyServerError(w, 500, "Internal") },
	}
	photo := func() sender.InputProfilePhoto {
		return &sender.InputProfilePhotoStatic{Photo: sender.FromBytes([]byte("x"), "p.jpg")}
	}
	calls := []struct {
		name, method string
		call         func(ctx context.Context, c *sender.Client) error
	}{
		{"my", "setMyProfilePhoto", func(ctx context.Context, c *sender.Client) error { return c.SetMyProfilePhoto(ctx, photo()) }},
		{"business", "setBusinessAccountProfilePhoto", func(ctx context.Context, c *sender.Client) error {
			return c.SetBusinessAccountProfilePhoto(ctx, sender.SetBusinessAccountProfilePhotoRequest{BusinessConnectionID: "bc", Photo: photo()})
		}},
		{"remove", "removeMyProfilePhoto", func(ctx context.Context, c *sender.Client) error { return c.RemoveMyProfilePhoto(ctx) }},
	}
	for _, call := range calls {
		for name, reply := range replies {
			t.Run(call.name+"/"+name, func(t *testing.T) {
				server := testutil.NewMockServer(t)
				server.On("/bot"+testutil.TestToken+"/"+call.method, reply)
				// Retries enabled: a retry wrapper would send a second request.
				client := testutil.NewTestClient(t, server.BaseURL(), sender.WithRetries(1), sender.WithSleeper(&testutil.FakeSleeper{}))
				require.Error(t, call.call(context.Background(), client))
				assert.Equal(t, 1, server.CaptureCount())
			})
		}
	}
}

// T-8: the Go shape of the changed types.
func TestB46_Shape(t *testing.T) {
	type field struct{ name, typ, tag string }
	shape := func(v any) []field {
		rt := reflect.TypeOf(v)
		out := []field{}
		for i := range rt.NumField() {
			f := rt.Field(i)
			require.False(t, f.Anonymous, "%s.%s embedded", rt.Name(), f.Name)
			out = append(out, field{f.Name, f.Type.String(), f.Tag.Get("json")})
		}
		return out
	}
	assert.Equal(t, []field{{"Photo", "sender.InputProfilePhoto", "-"}}, shape(sender.SetMyProfilePhotoRequest{}))
	assert.Equal(t, []field{}, shape(sender.RemoveMyProfilePhotoRequest{}))
	assert.Equal(t, []field{{"Animation", "sender.InputFile", "-"}, {"MainFrameTime", "float64", "main_frame_timestamp,omitempty"}},
		shape(sender.InputProfilePhotoAnimated{}))
	assert.Equal(t, []field{{"Photo", "sender.InputFile", "-"}}, shape(sender.InputProfilePhotoStatic{}))
}
