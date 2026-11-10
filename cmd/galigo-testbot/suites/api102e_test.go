package suites

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/engine"
	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.2 rich message media, S69 (B-25 10.2e, TEST-PLAN §4, rows T-tb-5 … T-tb-8).

// view102eMedia is a media list with each InputMedia's file replaced by its view (its bytes read).
func view102eMedia(media []tg.InputRichMessageMedia) []any {
	out := make([]any, len(media))
	for i, m := range media {
		im, ok := m.Media.(sender.InputMedia)
		if !ok {
			out[i] = m
			continue
		}
		out[i] = struct {
			ID    string
			Media any
		}{m.ID, viewPollMedia(&im)}
	}
	return out
}

// view102eSteps is viewSteps with the media lists of the rich steps viewed.
func view102eSteps(steps []engine.Step) []any {
	out := make([]any, len(steps))
	for i, s := range steps {
		switch st := s.(type) {
		case *engine.SendRichMessageStep:
			rm := st.RichMessage
			media := view102eMedia(rm.Media)
			rm.Media = nil
			out[i] = struct {
				Kind  string
				RM    tg.InputRichMessage
				Media []any
				Want  []string
			}{"SendRichMessageStep", rm, media, st.WantBlocks}
		case *engine.EditMessageRichStep:
			rm := st.RichMessage
			media := view102eMedia(rm.Media)
			rm.Media = nil
			out[i] = struct {
				Kind  string
				RM    tg.InputRichMessage
				Media []any
				Want  []string
			}{"EditMessageRichStep", rm, media, st.WantBlocks}
		default:
			out[i] = viewStep(s)
		}
	}
	return out
}

var (
	s69Kinds1 = []string{"paragraph", "photo", "video", "audio", "animation", "voice_note"}
	s69Kinds2 = []string{"photo", "video", "audio", "animation", "voice_note", "collage", "slideshow"}
	s69Kinds3 = []string{"paragraph", "photo"}
)

// T-tb-5: S69's name, covered methods and exact steps; the files are the fixtures (the animation is the MP4).
func TestAPI102E_S69Steps(t *testing.T) {
	got := AllAPI102Scenarios()
	require.Len(t, got, 2)
	s := got[1]
	assert.Equal(t, "S69-RichMedia", s.Name())
	assert.Equal(t, []string{"sendRichMessage", "editMessageText"}, s.Covers())
	media := func(id string, m sender.InputMedia) tg.InputRichMessageMedia {
		return tg.InputRichMessageMedia{ID: id, Media: m}
	}
	want := []engine.Step{
		&engine.SendRichMessageStep{RichMessage: tg.InputRichMessage{
			Markdown: "galigo 10.2 media\n\n![](tg://photo?id=p)\n\n![](tg://video?id=v)\n\n![](tg://audio?id=a)\n\n" +
				"![](tg://video?id=g)\n\n![](tg://audio?id=n)",
			Media: []tg.InputRichMessageMedia{
				media("p", sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg"))),
				media("v", sender.NewInputMediaVideo(sender.FromBytes(fixtures.VideoBytes(), "video.mp4"))),
				media("a", sender.NewInputMediaAudio(sender.FromBytes(fixtures.AudioBytes(), "audio.mp3"))),
				media("g", sender.NewInputMediaAnimation(sender.FromBytes(fixtures.VideoBytes(), "animation.mp4"))),
				media("n", sender.NewInputMediaVoiceNote(sender.FromBytes(fixtures.VoiceBytes(), "voice.ogg"))),
			}}, WantBlocks: s69Kinds1},
		&engine.SendRichMediaBlocksStep{WantBlocks: s69Kinds2},
		&engine.EditMessageRichStep{RichMessage: tg.InputRichMessage{Markdown: "galigo 10.2 edit\n\n![](tg://photo?id=q)",
			Media: []tg.InputRichMessageMedia{media("q", sender.NewInputMediaPhoto(sender.FromBytes(fixtures.PhotoBytes(), "photo.jpg")))}},
			WantBlocks: s69Kinds3},
		&engine.CleanupStep{},
	}
	assert.Equal(t, view102eSteps(want), view102eSteps(s.Steps()))

	// T-tb-6: every link names an id of the media list, in list order, and the wanted media kinds follow the links.
	step, ok := s.Steps()[0].(*engine.SendRichMessageStep)
	require.True(t, ok)
	links := regexp.MustCompile(`tg://(\w+)\?id=(\w+)`).FindAllStringSubmatch(step.RichMessage.Markdown, -1)
	require.Len(t, links, len(step.RichMessage.Media))
	require.Len(t, step.WantBlocks, len(links)+1, "a paragraph, then one block per link")
	linkKind := map[string]string{"photo": "photo", "video": "video", "animation": "video", "audio": "audio", "voice_note": "audio"}
	for i, l := range links {
		m := step.RichMessage.Media[i]
		assert.Equal(t, m.ID, l[2])
		assert.Equal(t, linkKind[m.Media.InputRichMediaType()], l[1], "link form of %s", m.ID)
		assert.Equal(t, m.Media.InputRichMediaType(), step.WantBlocks[i+1])
	}
}

// fake102e runs S69 on the real runner: the first sendRichMessage returns the uploaded media as blocks with file ids,
// the second returns the S69 block kinds, the edit returns a paragraph and a photo; deleteMessage is recorded.
type fake102e struct {
	engine.SenderClient
	sent    []tg.InputRichMessage
	edited  []tg.InputRichMessage
	editAt  [][2]int64 // the chat and message of each edit
	deleted [][2]int64
	reply1  string
	reply2  func() (*tg.Message, error) // the second sendRichMessage, when set
}

func (f *fake102e) rich(id int, js string) (*tg.Message, error) {
	var rich tg.RichMessage
	if err := json.Unmarshal([]byte(js), &rich); err != nil {
		return nil, err
	}
	return &tg.Message{MessageID: id, Chat: &tg.Chat{ID: testutil.TestChatID}, RichMessage: &rich}, nil
}

const s69Reply1 = `{"blocks":[{"type":"paragraph","text":"galigo 10.2 media"},` +
	`{"type":"photo","photo":[{"file_id":"ph-small","file_unique_id":"u1","width":1,"height":1},{"file_id":"ph-big","file_unique_id":"u2","width":9,"height":9}]},` +
	`{"type":"video","video":{"file_id":"vi","file_unique_id":"u3","width":1,"height":1,"duration":1}},` +
	`{"type":"audio","audio":{"file_id":"au","file_unique_id":"u4","duration":1}},` +
	`{"type":"animation","animation":{"file_id":"an","file_unique_id":"u5","width":1,"height":1,"duration":1}},` +
	`{"type":"voice_note","voice_note":{"file_id":"vn","file_unique_id":"u6","duration":1}}]}`

func (f *fake102e) SendRichMessage(_ context.Context, _ int64, rm tg.InputRichMessage) (*tg.Message, error) {
	f.sent = append(f.sent, rm)
	if len(f.sent) == 1 {
		return f.rich(601, f.reply1)
	}
	if f.reply2 != nil {
		return f.reply2()
	}
	return f.rich(602, `{"blocks":[{"type":"photo"},{"type":"video"},{"type":"audio"},{"type":"animation"},{"type":"voice_note"},{"type":"collage"},{"type":"slideshow"}]}`)
}

func (f *fake102e) EditMessageRich(_ context.Context, chatID int64, messageID int, rm tg.InputRichMessage) (*tg.Message, error) {
	f.edited = append(f.edited, rm)
	f.editAt = append(f.editAt, [2]int64{chatID, int64(messageID)})
	return f.rich(messageID, `{"blocks":[{"type":"paragraph","text":"e"},{"type":"photo"}]}`)
}

func (f *fake102e) DeleteMessage(_ context.Context, chatID int64, messageID int) error {
	f.deleted = append(f.deleted, [2]int64{chatID, int64(messageID)})
	return nil
}

// T-tb-7: S69 on the real runner: the blocks are made from the reply's file ids (the largest photo size), the edit
// goes to the blocks message, and both messages are deleted.
func TestAPI102E_S69Run(t *testing.T) {
	f := &fake102e{reply1: s69Reply1}
	rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
	runner := engine.NewRunner(rt, engine.RunnerConfig{MaxMessages: 100}, slog.New(slog.DiscardHandler))
	res := runner.Run(context.Background(), S69_RichMedia())
	require.True(t, res.Success, "%+v", res)
	require.Len(t, f.sent, 2)
	out, err := json.Marshal(f.sent[1])
	require.NoError(t, err)
	photo := `{"type":"photo","photo":{"media":"ph-big","type":"photo"}}`
	assert.Equal(t, `{"blocks":[`+
		`{"type":"photo","photo":{"media":"ph-big","type":"photo"},"caption":{"text":"photo"}},`+
		`{"type":"video","video":{"media":"vi","type":"video"},"caption":{"text":"video"}},`+
		`{"type":"audio","audio":{"media":"au","type":"audio"},"caption":{"text":"audio"}},`+
		`{"type":"animation","animation":{"media":"an","type":"animation"},"caption":{"text":"animation"}},`+
		`{"type":"voice_note","voice_note":{"media":"vn","type":"voice_note"},"caption":{"text":"voice note"}},`+
		`{"type":"collage","blocks":[`+photo+`,`+photo+`]},`+
		`{"type":"slideshow","blocks":[`+photo+`,`+photo+`]}]}`, string(out))
	require.Len(t, f.edited, 1)
	assert.Equal(t, [][2]int64{{testutil.TestChatID, 602}}, f.editAt, "the edit goes to the blocks message")
	assert.ElementsMatch(t, [][2]int64{{testutil.TestChatID, 601}, {testutil.TestChatID, 602}}, f.deleted)
	assert.Empty(t, rt.CreatedMessages)
}

// T-tb-8: the blocks step fails, naming the cause, when the last message has no rich message or lacks one of the five
// media kinds (before it sends), and when the reply is missing, has no rich message or other block kinds (a reply
// message is tracked for cleanup).
func TestAPI102E_S69StepErrors(t *testing.T) {
	missing := regexp.MustCompile(`,\{"type":"animation".*?\}\}`).ReplaceAllString(s69Reply1, "")
	require.NotEqual(t, s69Reply1, missing)
	f0 := &fake102e{}
	full, err := f0.rich(601, s69Reply1)
	require.NoError(t, err)
	part, err := f0.rich(601, missing)
	require.NoError(t, err)
	noSizes := regexp.MustCompile(`"photo":\[.*?\]\}`).ReplaceAllString(s69Reply1, `"photo":[]}`)
	require.NotEqual(t, s69Reply1, noSizes)
	empty, err := f0.rich(601, noSizes)
	require.NoError(t, err)
	other, err := f0.rich(602, `{"blocks":[{"type":"photo"},{"type":"video"},{"type":"audio"},{"type":"animation"},{"type":"voice_note"},{"type":"collage"}]}`)
	require.NoError(t, err)
	rows := []struct {
		name    string
		last    *tg.Message
		reply2  func() (*tg.Message, error)
		want    string
		sent    int
		tracked []int
	}{
		{"no-last-message", nil, nil, "sendRichMessage: no rich message to take the media from", 0, nil},
		{"last-not-rich", &tg.Message{MessageID: 5}, nil, "sendRichMessage: no rich message to take the media from", 0, nil},
		{"photo-no-sizes", empty, nil, "sendRichMessage: no photo file_id in the rich message of the last message", 0, nil},
		{"missing-kind", part, nil, "sendRichMessage: no animation file_id in the rich message of the last message", 0, nil},
		{"send-error", full, func() (*tg.Message, error) { return nil, errors.New("boom") }, "boom", 1, nil},
		{"no-message", full, func() (*tg.Message, error) { return nil, nil }, "sendRichMessage: no message in the reply", 1, nil},
		{"reply-not-rich", full, func() (*tg.Message, error) { return &tg.Message{MessageID: 603}, nil },
			"sendRichMessage: the reply has no rich_message", 1, []int{603}},
		{"other-kinds", full, func() (*tg.Message, error) { return other, nil },
			"sendRichMessage: blocks [photo video audio animation voice_note collage], want " +
				"[photo video audio animation voice_note collage slideshow]", 1, []int{602}},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			f := &fake102e{sent: []tg.InputRichMessage{{}}, reply2: r.reply2} // the first send already happened
			rt := engine.NewRuntime(f, testutil.TestChatID, testutil.TestUserID)
			rt.LastMessage = r.last
			_, err := (&engine.SendRichMediaBlocksStep{WantBlocks: s69Kinds2}).Execute(context.Background(), rt)
			require.Error(t, err)
			assert.Equal(t, r.want, err.Error())
			assert.Len(t, f.sent, 1+r.sent, "sends by the blocks step")
			var ids []int
			for _, m := range rt.CreatedMessages {
				ids = append(ids, m.MessageID)
			}
			assert.Equal(t, r.tracked, ids)
		})
	}
}
