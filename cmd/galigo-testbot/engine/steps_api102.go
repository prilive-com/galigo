package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// ==================== Bot API 10.2 steps (rich message media) ====================

// SendRichMediaBlocksStep sends to rt.ChatID a rich message of media blocks made from the file_ids in the rich message
// of rt.LastMessage: a photo, video, audio, animation and voice note block, each with its kind as caption, then a
// collage and a slideshow of two photo blocks. It checks that the reply has the block types WantBlocks.
type SendRichMediaBlocksStep struct {
	WantBlocks []string
}

func (s *SendRichMediaBlocksStep) Name() string { return "sendRichMessage (media blocks)" }

func (s *SendRichMediaBlocksStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendRichMessage"}
	if rt.LastMessage == nil || rt.LastMessage.RichMessage == nil {
		return res, errors.New("sendRichMessage: no rich message to take the media from")
	}
	ids := map[string]string{}
	for _, b := range rt.LastMessage.RichMessage.Blocks {
		switch v := b.(type) {
		case tg.RichBlockPhoto:
			if len(v.Photo) > 0 {
				ids["photo"] = v.Photo[len(v.Photo)-1].FileID
			}
		case tg.RichBlockVideo:
			ids["video"] = v.Video.FileID
		case tg.RichBlockAudio:
			ids["audio"] = v.Audio.FileID
		case tg.RichBlockAnimation:
			ids["animation"] = v.Animation.FileID
		case tg.RichBlockVoiceNote:
			ids["voice_note"] = v.VoiceNote.FileID
		}
	}
	for _, kind := range []string{"photo", "video", "audio", "animation", "voice_note"} {
		if ids[kind] == "" {
			return res, fmt.Errorf("sendRichMessage: no %s file_id in the rich message of the last message", kind)
		}
	}
	caption := func(s string) *tg.RichBlockCaption { return &tg.RichBlockCaption{Text: tg.RichText{Plain: s}} }
	photo := func() tg.InputRichBlock {
		return tg.InputRichBlockPhoto{Photo: sender.NewInputMediaPhoto(sender.FromFileID(ids["photo"]))}
	}
	rm := tg.InputRichMessage{Blocks: []tg.InputRichBlock{
		tg.InputRichBlockPhoto{Photo: sender.NewInputMediaPhoto(sender.FromFileID(ids["photo"])), Caption: caption("photo")},
		tg.InputRichBlockVideo{Video: sender.NewInputMediaVideo(sender.FromFileID(ids["video"])), Caption: caption("video")},
		tg.InputRichBlockAudio{Audio: sender.NewInputMediaAudio(sender.FromFileID(ids["audio"])), Caption: caption("audio")},
		tg.InputRichBlockAnimation{Animation: sender.NewInputMediaAnimation(sender.FromFileID(ids["animation"])),
			Caption: caption("animation")},
		tg.InputRichBlockVoiceNote{VoiceNote: sender.NewInputMediaVoiceNote(sender.FromFileID(ids["voice_note"])),
			Caption: caption("voice note")},
		tg.InputRichBlockCollage{Blocks: []tg.InputRichBlock{photo(), photo()}},
		tg.InputRichBlockSlideshow{Blocks: []tg.InputRichBlock{photo(), photo()}},
	}}
	msg, err := rt.Sender.SendRichMessage(ctx, rt.ChatID, rm)
	if err != nil {
		return res, err
	}
	if msg == nil {
		return res, errors.New("sendRichMessage: no message in the reply")
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res.MessageIDs = []int{msg.MessageID}
	if msg.RichMessage == nil {
		return res, errors.New("sendRichMessage: the reply has no rich_message")
	}
	if got := richBlockTypes(msg.RichMessage); !slices.Equal(got, s.WantBlocks) {
		return res, fmt.Errorf("sendRichMessage: blocks %v, want %v", got, s.WantBlocks)
	}
	res.Evidence = map[string]any{"message_id": msg.MessageID, "blocks": s.WantBlocks}
	return res, nil
}
