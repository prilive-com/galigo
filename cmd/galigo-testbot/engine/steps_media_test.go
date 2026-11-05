package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// B-45 testbot steps (TEST-PLAN §4, T-tb-2 … T-tb-4).

const mediaChat = int64(555)

// fakeMedia is a SenderClient for the file_id media steps: it embeds the
// interface (calling any other method panics) and overrides the three methods
// the steps use. It records every request and returns scripted replies in order.
type fakeMedia struct {
	SenderClient

	calls map[string]int

	docChats   []int64
	groupChats []int64
	groupMedia [][]MediaInput
	editChats  []int64
	editMsgIDs []int
	editMedia  []sender.InputMedia

	docReplies []*tg.Message
	groupReply []*tg.Message
	editReply  *tg.Message
}

func newFakeMedia() *fakeMedia { return &fakeMedia{calls: map[string]int{}} }

func (f *fakeMedia) SendDocument(_ context.Context, chatID int64, _ MediaInput, _ string) (*tg.Message, error) {
	f.calls["sendDocument"]++
	f.docChats = append(f.docChats, chatID)
	m := f.docReplies[0]
	f.docReplies = f.docReplies[1:]
	return m, nil
}

func (f *fakeMedia) SendMediaGroup(_ context.Context, chatID int64, media []MediaInput) ([]*tg.Message, error) {
	f.calls["sendMediaGroup"]++
	f.groupChats = append(f.groupChats, chatID)
	f.groupMedia = append(f.groupMedia, media)
	return f.groupReply, nil
}

func (f *fakeMedia) EditMessageMedia(_ context.Context, chatID int64, messageID int, media sender.InputMedia) (*tg.Message, error) {
	f.calls["editMessageMedia"]++
	f.editChats = append(f.editChats, chatID)
	f.editMsgIDs = append(f.editMsgIDs, messageID)
	f.editMedia = append(f.editMedia, media)
	return f.editReply, nil
}

func docMessage(id int, fileID string) *tg.Message {
	return &tg.Message{MessageID: id, Document: &tg.Document{FileID: fileID}}
}

func photoMessage(id int, fileID string) *tg.Message {
	return &tg.Message{MessageID: id, Photo: []tg.PhotoSize{{FileID: fileID}}}
}

// T-tb-2
func TestSendMediaGroupByFileIDStep(t *testing.T) {
	step := &SendMediaGroupByFileIDStep{FileKeys: []string{"doc_a", "doc_b"}, MediaType: "document"}
	newRT := func(f *fakeMedia) *Runtime {
		rt := NewRuntime(f, mediaChat, 0)
		rt.CapturedFileIDs["doc_a"] = "FA"
		rt.CapturedFileIDs["doc_b"] = "FB"
		return rt
	}

	t.Run("sends the captured file_ids, no upload", func(t *testing.T) {
		f := newFakeMedia()
		f.groupReply = []*tg.Message{docMessage(31, "x1"), docMessage(32, "x2")}
		rt := newRT(f)

		res, err := step.Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, 1, f.calls["sendMediaGroup"])
		assert.Equal(t, []int64{mediaChat}, f.groupChats)
		require.Len(t, f.groupMedia, 1)
		for _, m := range f.groupMedia[0] {
			assert.Nil(t, m.Reader, "no upload")
			assert.Empty(t, m.URL)
		}
		assert.Equal(t, []MediaInput{{FileID: "FA", Type: "document"}, {FileID: "FB", Type: "document"}}, f.groupMedia[0])

		assert.Equal(t, "sendMediaGroup", res.Method)
		assert.Equal(t, []int{31, 32}, res.MessageIDs)
		assert.Equal(t, []string{"FA", "FB"}, res.FileIDs)
		assert.Equal(t, []CreatedMessage{{ChatID: mediaChat, MessageID: 31}, {ChatID: mediaChat, MessageID: 32}}, rt.CreatedMessages)
	})

	t.Run("missing key: error, no call", func(t *testing.T) {
		f := newFakeMedia()
		rt := NewRuntime(f, mediaChat, 0)
		rt.CapturedFileIDs["doc_a"] = "FA"
		_, err := step.Execute(context.Background(), rt)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "doc_b")
		assert.Zero(t, f.calls["sendMediaGroup"])
	})

	t.Run("fewer messages than keys: error", func(t *testing.T) {
		f := newFakeMedia()
		f.groupReply = []*tg.Message{docMessage(31, "x1")}
		_, err := step.Execute(context.Background(), newRT(f))
		require.Error(t, err)
	})

	t.Run("another media kind: error", func(t *testing.T) {
		f := newFakeMedia()
		f.groupReply = []*tg.Message{photoMessage(31, "x1"), photoMessage(32, "x2")}
		_, err := step.Execute(context.Background(), newRT(f))
		require.Error(t, err)
	})
}

// T-tb-3
func TestEditMessageMediaByFileIDStep(t *testing.T) {
	step := &EditMessageMediaByFileIDStep{FileKey: "document", MediaType: "document", Caption: "cap"}
	newRT := func(f *fakeMedia) *Runtime {
		rt := NewRuntime(f, mediaChat, 0)
		rt.CapturedFileIDs["document"] = "FD"
		rt.LastMessage = photoMessage(77, "P")
		return rt
	}

	t.Run("edits the last message by the captured file_id", func(t *testing.T) {
		f := newFakeMedia()
		reply := &tg.Message{MessageID: 77, Document: &tg.Document{FileID: "FD2"}, Caption: "cap"}
		f.editReply = reply
		rt := newRT(f)

		res, err := step.Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, 1, f.calls["editMessageMedia"])
		assert.Equal(t, []int64{mediaChat}, f.editChats)
		assert.Equal(t, []int{77}, f.editMsgIDs)
		require.Len(t, f.editMedia, 1)
		assert.Nil(t, f.editMedia[0].Media.Reader, "no upload")
		assert.Nil(t, f.editMedia[0].Media.Source, "no upload")
		assert.Equal(t, sender.InputMedia{Type: "document", Media: sender.InputFile{FileID: "FD"}, Caption: "cap"}, f.editMedia[0])

		assert.Equal(t, "editMessageMedia", res.Method)
		assert.Equal(t, []string{"FD"}, res.FileIDs)
		assert.Empty(t, res.MessageIDs, "the edit sends no new message")
		assert.Empty(t, rt.CreatedMessages, "nothing new is tracked")
		assert.Same(t, reply, rt.LastMessage)
	})

	t.Run("no last message: error, no call", func(t *testing.T) {
		f := newFakeMedia()
		rt := newRT(f)
		rt.LastMessage = nil
		_, err := step.Execute(context.Background(), rt)
		require.Error(t, err)
		assert.Zero(t, f.calls["editMessageMedia"])
	})

	t.Run("missing key: error, no call", func(t *testing.T) {
		f := newFakeMedia()
		rt := newRT(f)
		delete(rt.CapturedFileIDs, "document")
		_, err := step.Execute(context.Background(), rt)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "document")
		assert.Zero(t, f.calls["editMessageMedia"])
	})

	t.Run("caption differs: error", func(t *testing.T) {
		f := newFakeMedia()
		f.editReply = &tg.Message{MessageID: 77, Document: &tg.Document{FileID: "FD2"}, Caption: "other"}
		_, err := step.Execute(context.Background(), newRT(f))
		require.Error(t, err)
	})

	t.Run("media kind differs: error", func(t *testing.T) {
		f := newFakeMedia()
		f.editReply = &tg.Message{MessageID: 77, Photo: []tg.PhotoSize{{FileID: "P2"}}, Caption: "cap"}
		_, err := step.Execute(context.Background(), newRT(f))
		require.Error(t, err)
	})
}

// T-tb-4
func TestSendDocumentStepStoreAs(t *testing.T) {
	t.Run("two StoreAs keys on one runtime", func(t *testing.T) {
		f := newFakeMedia()
		f.docReplies = []*tg.Message{docMessage(11, "doc1"), docMessage(12, "doc2")}
		rt := NewRuntime(f, mediaChat, 0)

		resA, err := (&SendDocumentStep{Document: MediaFromBytes([]byte("a"), "a.txt", "document"), StoreAs: "doc_a"}).Execute(context.Background(), rt)
		require.NoError(t, err)
		resB, err := (&SendDocumentStep{Document: MediaFromBytes([]byte("b"), "b.txt", "document"), StoreAs: "doc_b"}).Execute(context.Background(), rt)
		require.NoError(t, err)

		assert.Equal(t, map[string]string{"doc_a": "doc1", "doc_b": "doc2", "document": "doc2"}, rt.CapturedFileIDs)
		require.NotNil(t, rt.LastMessage)
		assert.Equal(t, 12, rt.LastMessage.MessageID)
		assert.Equal(t, []int{11}, resA.MessageIDs)
		assert.Equal(t, []int{12}, resB.MessageIDs)
		assert.Equal(t, []string{"doc1"}, resA.FileIDs)
		assert.Equal(t, []string{"doc2"}, resB.FileIDs)
		assert.Equal(t, []CreatedMessage{{ChatID: mediaChat, MessageID: 11}, {ChatID: mediaChat, MessageID: 12}}, rt.CreatedMessages)
	})

	t.Run("no StoreAs: only document (existing behaviour)", func(t *testing.T) {
		f := newFakeMedia()
		f.docReplies = []*tg.Message{docMessage(11, "doc1")}
		rt := NewRuntime(f, mediaChat, 0)
		res, err := (&SendDocumentStep{Document: MediaFromBytes([]byte("a"), "a.txt", "document")}).Execute(context.Background(), rt)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"document": "doc1"}, rt.CapturedFileIDs)
		assert.Equal(t, []int{11}, res.MessageIDs)
		assert.Equal(t, []CreatedMessage{{ChatID: mediaChat, MessageID: 11}}, rt.CreatedMessages)
	})
}
