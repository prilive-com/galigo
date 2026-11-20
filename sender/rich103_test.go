package sender_test

import (
	"context"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.3 rich message blocks through the sender (B-65 65c, TEST-PLAN §3). The sender has no 10.3 code: these
// rows show that the new blocks go through each caller's JSON and form body unchanged (AC3).

// b65cCallers are the four callers that send an InputRichMessage: the three of 10.2 and editEphemeralMessageText.
var b65cCallers = append(slices.Clone(api102dCallers), api102dCaller{"ephemeral", "editEphemeralMessageText",
	func(ctx context.Context, c *sender.Client, rm tg.InputRichMessage) error {
		return c.EditEphemeralMessageText(ctx, b65bText("", &rm))
	}, func(rich string) string { return `{` + api102cBody + `,"rich_message":` + rich + `}` }})

// b65cArgs are the form fields of each caller beside rich_message.
var b65cArgs = map[string]map[string]string{
	"send":      api102eArgs["send"],
	"draft":     api102eArgs["draft"],
	"edit":      api102eArgs["edit"],
	"ephemeral": {"chat_id": "-1001234567890", "receiver_user_id": "5000000001", "ephemeral_message_id": "4242"},
}

func b65cServer(t *testing.T) (*testutil.MockTelegramServer, *sender.Client) {
	t.Helper()
	server, _ := api102dServer(t)
	api102cOK(server, "editEphemeralMessageText")
	return server, testutil.NewTestClient(t, server.BaseURL())
}

func b65cRT(s string) tg.RichText { return tg.RichText{Plain: s} }

// b65cBlocks are the five 10.3 blocks the rows send; b65cBlocksJSON is their encoding, written out.
func b65cBlocks() []tg.InputRichBlock {
	cell, credit := b65cRT("x"), b65cRT("c")
	return []tg.InputRichBlock{
		tg.InputRichBlockButtons{Buttons: []tg.RichMessageButton{
			{Text: b65cRT("cb"), Style: tg.ButtonStylePrimary, CallbackData: "c"},
			{Text: b65cRT("cp"), CopyText: &tg.CopyTextButton{Text: "galigo"}},
		}, Align: "center"},
		tg.InputRichBlockParagraph{Text: tg.RichText{Parts: []tg.RichText{b65cRT("a"),
			{Node: tg.RichTextButton{Type: "button", Button: tg.RichMessageButton{Text: b65cRT("b"), CallbackData: "n"}}}}}},
		tg.InputRichBlockTable{Cells: [][]tg.RichBlockTableCell{{{Text: &cell, Align: "left", VAlign: "top"}}}, IsCompact: true},
		tg.InputRichBlockExpandableBlockQuotation{Text: b65cRT("q"), Credit: &credit},
		tg.InputRichBlockDocument{Document: sender.NewInputMediaDocument(sender.FromFileID("BQAC-fake-doc")),
			Caption: &tg.RichBlockCaption{Text: b65cRT("d")}},
	}
}

const b65cBlocksJSON = `[{"type":"buttons","buttons":[{"text":"cb","style":"primary","callback_data":"c"},` +
	`{"text":"cp","copy_text":{"text":"galigo"}}],"align":"center"},` +
	`{"type":"paragraph","text":["a",{"type":"button","button":{"text":"b","callback_data":"n"}}]},` +
	`{"type":"table","cells":[[{"text":"x","align":"left","valign":"top"}]],"is_compact":true},` +
	`{"type":"expandable_blockquote","text":"q","credit":"c"},` +
	`{"type":"document","document":{"type":"document","media":"BQAC-fake-doc"},"caption":{"text":"d"}}]`

// b65cCall runs one caller against a fresh mock server and returns the one request it sent, decoded.
func b65cCall(t *testing.T, c api102dCaller, rm tg.InputRichMessage) (string, map[string]string, map[string]wireFile) {
	t.Helper()
	server, client := b65cServer(t)
	require.NoError(t, c.call(context.Background(), client, rm))
	require.Equal(t, 1, server.CaptureCount())
	got := server.LastCapture()
	got.AssertPath(t, api101dPath(c.method))
	return wireArgs(t, capturedRequest(got))
}

// T-s-blocks: the 10.3 blocks in the JSON body of each caller.
func TestB65C_SendBlocks(t *testing.T) {
	for _, c := range b65cCallers {
		t.Run(c.name, func(t *testing.T) {
			server, client := b65cServer(t)
			require.NoError(t, c.call(context.Background(), client, tg.InputRichMessage{Blocks: b65cBlocks()}))
			require.Equal(t, 1, server.CaptureCount())
			got := server.LastCapture()
			got.AssertPath(t, api101dPath(c.method))
			got.AssertContentType(t, "application/json")
			assert.JSONEq(t, c.body(`{"blocks":`+b65cBlocksJSON+`}`), got.BodyString())
		})
	}
}

// T-s-form: the same blocks beside an upload in the media list go in the rich_message form field, unchanged.
func TestB65C_SendBlocksForm(t *testing.T) {
	for _, c := range b65cCallers {
		t.Run(c.name, func(t *testing.T) {
			mt, args, files := b65cCall(t, c, tg.InputRichMessage{Blocks: b65cBlocks(), Media: []tg.InputRichMessageMedia{
				{ID: "p", Media: sender.NewInputMediaPhoto(sender.FromBytes([]byte("ph"), "p.jpg"))}}})
			assert.Equal(t, "multipart/form-data", mt)
			assert.JSONEq(t, `{"blocks":`+b65cBlocksJSON+`,"media":[{"id":"p","media":{"media":"attach://file0","type":"photo"}}]}`,
				args["rich_message"])
			delete(args, "rich_message")
			assertArgs(t, b65cArgs[c.name], args)
			assertFiles(t, map[string]wireFile{"file0": {FileName: "p.jpg", Data: []byte("ph")}}, files)
		})
	}
}

// T-s-doc-upload for editEphemeralMessageText (TestAPI102E_SendMediaUploads has the other three callers): a
// tg://document link with an upload in the media list.
func TestB65C_EphemeralDocumentUpload(t *testing.T) {
	mt, args, files := b65cCall(t, b65cCallers[3], tg.InputRichMessage{Markdown: "![](tg://document?id=d)",
		Media: []tg.InputRichMessageMedia{{ID: "d", Media: sender.NewInputMediaDocument(sender.FromBytes([]byte("doc"), "d.txt"))}}})
	assert.Equal(t, "multipart/form-data", mt)
	want := map[string]string{"rich_message": `{"markdown":"![](tg://document?id=d)","media":[{"id":"d","media":{"media":"attach://file0","type":"document"}}]}`}
	for k, v := range b65cArgs["ephemeral"] {
		want[k] = v
	}
	assertArgs(t, want, args)
	assertFiles(t, map[string]wireFile{"file0": {FileName: "d.txt", Data: []byte("doc")}}, files)
}

// T-s-doc-block-upload (B-61 for the document block): an upload in the document block is refused before any request,
// as a pointer and inside a collage beside an upload in the media list (the form path), on each caller.
// TestAPI102E_UploadElsewhereRefused and TestB65B_EditTextRichBlockUpload have the plain value form.
func TestB65C_DocumentBlockUploadRefused(t *testing.T) {
	for _, c := range b65cCallers {
		for _, k := range []string{"pointer", "in-collage-beside-list-upload"} {
			t.Run(c.name+"/"+k, func(t *testing.T) {
				var cnt api102eCounter
				rm := tg.InputRichMessage{}
				switch k {
				case "pointer":
					d := sender.NewInputMediaDocument(cnt.source("x"))
					rm.Blocks = []tg.InputRichBlock{tg.InputRichBlockDocument{Document: &d}}
				case "in-collage-beside-list-upload":
					rm.Blocks = []tg.InputRichBlock{tg.InputRichBlockCollage{Blocks: []tg.InputRichBlock{
						tg.InputRichBlockDocument{Document: sender.NewInputMediaDocument(cnt.reader("x"))}}}}
					rm.Media = []tg.InputRichMessageMedia{{ID: "u", Media: sender.NewInputMediaPhoto(sender.FromBytes([]byte("p"), "p.jpg"))}}
				}
				server, client := b65cServer(t)
				err := c.call(context.Background(), client, rm)
				require.Error(t, err)
				assert.Contains(t, err.Error(), api102eUploadErr)
				assert.Equal(t, 0, server.CaptureCount())
				cnt.assertUnused(t)
			})
		}
	}
}
