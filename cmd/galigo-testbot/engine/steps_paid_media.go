package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/prilive-com/galigo/cmd/galigo-testbot/fixtures"
	"github.com/prilive-com/galigo/sender"
)

// paidMediaKinds is the order of the items SendPaidMediaStep sends, as the reply's paid_media must list them.
var paidMediaKinds = []string{"photo", "video", "live_photo"}

// SendPaidMediaStep sends paid media (Bot API 7.6) priced at one Star: the fixture photo, the fixture video and a
// live photo (10.0) made of both, each uploaded. The reply must carry paid_media with star_count 1 and the three
// kinds in order, and the caption.
type SendPaidMediaStep struct{}

func (s *SendPaidMediaStep) Name() string { return "sendPaidMedia (7.6)" }

func (s *SendPaidMediaStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	res := &StepResult{StepName: s.Name(), Method: "sendPaidMedia"}
	const caption = "galigo paid media"
	photo := func(name string) sender.InputFile { return sender.FromBytes(fixtures.PhotoBytes(), name) }
	video := func(name string) sender.InputFile { return sender.FromBytes(fixtures.VideoBytes(), name) }

	msg, err := rt.Sender.SendPaidMedia(ctx, sender.SendPaidMediaRequest{
		ChatID:    rt.ChatID,
		StarCount: 1,
		Media: []sender.InputPaidMedia{
			sender.NewInputPaidMediaPhoto(photo("galigo-paid.jpg")),
			sender.NewInputPaidMediaVideo(video("galigo-paid.mp4")),
			sender.NewInputPaidMediaLivePhoto(video("galigo-paid-live.mp4"), photo("galigo-paid-live.jpg")),
		},
		Payload: "galigo-paid-media",
		Caption: caption,
	})
	if err != nil {
		return res, err
	}
	if msg == nil {
		return res, errors.New("sendPaidMedia: no message in the reply")
	}
	rt.TrackMessage(rt.ChatID, msg.MessageID)
	rt.LastMessage = msg
	res.MessageIDs = []int{msg.MessageID}

	if msg.PaidMedia == nil {
		return res, errors.New("sendPaidMedia: the reply has no paid_media")
	}
	kinds := make([]string, 0, len(msg.PaidMedia.PaidMedia))
	for _, item := range msg.PaidMedia.PaidMedia {
		kinds = append(kinds, item.Type)
	}
	res.Evidence = map[string]any{
		"message_id": msg.MessageID,
		"star_count": msg.PaidMedia.StarCount,
		"kinds":      kinds,
	}
	if msg.PaidMedia.StarCount != 1 {
		return res, fmt.Errorf("sendPaidMedia: star_count %d, want 1", msg.PaidMedia.StarCount)
	}
	if !slices.Equal(kinds, paidMediaKinds) {
		return res, fmt.Errorf("sendPaidMedia: paid media kinds %v, want %v", kinds, paidMediaKinds)
	}
	if msg.Caption != caption {
		return res, fmt.Errorf("sendPaidMedia: caption %q, want %q", msg.Caption, caption)
	}
	return res, nil
}
