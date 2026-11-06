package sender_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prilive-com/galigo/internal/testutil"
	"github.com/prilive-com/galigo/sender"
	"github.com/prilive-com/galigo/tg"
)

// Bot API 10.0 part c on the send side of sendPoll: members_only, country_codes and 1-12 options
// (B-25 slice 2c, TEST-PLAN §3, rows T-s-1 … T-s-7).

// polls100Probe21 is the probe 21 message, the same bytes as api100cProbe21 in tg/api100c_test.go (E10/gen100c.py).
const polls100Probe21 = `{"message_id":1101,"from":{"id":123456789,"is_bot":true,"first_name":"Test Bot","username":"testbot"},"chat":{"id":123456789,"title":"Test Chat","is_forum":true,"type":"supergroup"},"date":1793928592,"poll":{"id":"poll-21","question":"probe100c option media","options":[{"persistent_id":"0","text":"photo","media":{"photo":[{"file_id":"PH90-file","file_unique_id":"PH90-uniq","file_size":341,"width":90,"height":90},{"file_id":"PH100-file","file_unique_id":"PH100-uniq","file_size":361,"width":100,"height":100}]},"voter_count":0},{"persistent_id":"1","text":"loc","media":{"location":{"latitude":52.520005,"longitude":13.404999}},"voter_count":0},{"persistent_id":"2","text":"venue","media":{"venue":{"location":{"latitude":48.858396,"longitude":2.294497},"title":"probe venue","address":"probe address"}},"voter_count":0},{"persistent_id":"3","text":"sticker","media":{"sticker":{"width":512,"height":512,"is_animated":false,"is_video":false,"type":"regular","thumbnail":{"file_id":"STKTH-file","file_unique_id":"STKTH-uniq","file_size":4109,"width":320,"height":320},"thumb":{"file_id":"STKTH-file","file_unique_id":"STKTH-uniq","file_size":4109,"width":320,"height":320},"file_id":"STK-file","file_unique_id":"STK-uniq","file_size":1961}},"voter_count":0},{"persistent_id":"4","text":"video","media":{"video":{"duration":1,"width":320,"height":240,"file_name":"v.mp4","mime_type":"video/mp4","thumbnail":{"file_id":"VIDTH-file","file_unique_id":"VIDTH-uniq","file_size":733,"width":320,"height":240},"thumb":{"file_id":"VIDTH-file","file_unique_id":"VIDTH-uniq","file_size":733,"width":320,"height":240},"file_id":"VID-file","file_unique_id":"VID-uniq","file_size":663}},"voter_count":0},{"persistent_id":"5","text":"live","media":{"live_photo":{"photo":[{"file_id":"PH90-file","file_unique_id":"PH90-uniq","file_size":341,"width":90,"height":90},{"file_id":"PH100-file","file_unique_id":"PH100-uniq","file_size":361,"width":100,"height":100}],"duration":1,"width":320,"height":240,"mime_type":"video/mp4","file_id":"LIVE-file","file_unique_id":"LIVE-uniq","file_size":663}},"voter_count":0}],"total_voter_count":0,"is_closed":false,"is_anonymous":false,"allows_multiple_answers":false,"allows_revoting":true,"members_only":false,"type":"regular"}}`

func polls100Send(t *testing.T, req sender.SendPollRequest) (string, map[string]string) {
	t.Helper()
	mediaType, args, files, err := wireCall(t, "sendPoll", replyMessage, func(ctx context.Context, c *sender.Client) error {
		_, err := c.SendPoll(ctx, req)
		return err
	})
	require.NoError(t, err)
	assert.Empty(t, files)
	return mediaType, args
}

func polls100Req() sender.SendPollRequest {
	return sender.SendPollRequest{ChatID: int64(-1001234567890), Question: "Q?",
		Options: []sender.InputPollOption{{Text: "A"}, {Text: "B"}}}
}

// T-s-1: each new field alone and both together; the whole argument list is compared (T1-02).
func TestPolls100_SendPollWire(t *testing.T) {
	base := map[string]string{"chat_id": "-1001234567890", "question": "Q?", "options": `[{"text":"A"},{"text":"B"}]`}
	with := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	rows := []struct {
		name    string
		members bool
		codes   []string
		want    map[string]string
	}{
		{"members", true, nil, with("members_only", "true")},
		{"countries", false, []string{"DE", "FT"}, with("country_codes", `["DE","FT"]`)},
		{"both", true, []string{"DE", "FT"}, with("members_only", "true", "country_codes", `["DE","FT"]`)},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			req := polls100Req()
			req.MembersOnly, req.CountryCodes = r.members, r.codes
			mediaType, args := polls100Send(t, req)
			assert.Equal(t, "application/json", mediaType)
			assertArgs(t, r.want, args)
		})
	}
}

// T-s-2: false and nil or empty send neither new key.
func TestPolls100_SendPollWireOmitted(t *testing.T) {
	want := map[string]string{"chat_id": "-1001234567890", "question": "Q?", "options": `[{"text":"A"},{"text":"B"}]`}
	for name, codes := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			req := polls100Req()
			req.CountryCodes = codes
			_, args := polls100Send(t, req)
			assertArgs(t, want, args)
		})
	}
}

// T-s-3: a request with every pre-10.0c field and neither new one sends the same bytes as before.
func TestPolls100_SendPollBytes96(t *testing.T) {
	server := api96PollServer(t)
	client := testutil.NewTestClient(t, server.BaseURL())
	anon, f, zero := false, false, 0
	_, err := client.SendPoll(context.Background(), sender.SendPollRequest{
		ChatID: int64(-100123), Question: "Q?", Options: []sender.InputPollOption{{Text: "A"}, {Text: "B"}},
		IsAnonymous: &anon, Type: "quiz", AllowsMultipleAnswers: true, CorrectOptionID: &zero, Explanation: "e",
		ExplanationParseMode: tg.ParseModeHTML, OpenPeriod: 60, CloseDate: 1793928546, IsClosed: true,
		DisableNotification: true, ProtectContent: true, ReplyToMessageID: 7, ReplyMarkup: map[string]any{"k": 1},
		CorrectOptionIDs: []int{0}, AllowsRevoting: &f, ShuffleOptions: true, AllowAddingOptions: true,
		HideResultsUntilCloses: true, Description: "d", DescriptionParseMode: tg.ParseModeHTML,
		DescriptionEntities: []tg.MessageEntity{{Type: "bold", Offset: 0, Length: 1}},
	})
	require.NoError(t, err)
	require.Equal(t, 1, server.CaptureCount())
	assert.Equal(t, `{"chat_id":-100123,"question":"Q?","options":[{"text":"A"},{"text":"B"}],"is_anonymous":false,`+
		`"type":"quiz","allows_multiple_answers":true,"correct_option_id":0,"explanation":"e",`+
		`"explanation_parse_mode":"HTML","open_period":60,"close_date":1793928546,"is_closed":true,`+
		`"disable_notification":true,"protect_content":true,"reply_to_message_id":7,"reply_markup":{"k":1},`+
		`"correct_option_ids":[0],"allows_revoting":false,"shuffle_options":true,"allow_adding_options":true,`+
		`"hide_results_until_closes":true,"description":"d","description_parse_mode":"HTML",`+
		`"description_entities":[{"type":"bold","offset":0,"length":1}]}`,
		strings.TrimSuffix(server.LastCapture().BodyString(), "\n"))
}

func polls100Options(n int) []string {
	o := make([]string, n)
	for i := range o {
		o[i] = "o" + strconv.Itoa(i)
	}
	return o
}

// T-s-4: every count 1..12 is sent, with its options in order (T1-03).
func TestPolls100_SimpleValidCounts(t *testing.T) {
	for n := 1; n <= 12; n++ {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			var msg *tg.Message
			_, args, _, err := wireCall(t, "sendPoll", replyMessage, func(ctx context.Context, c *sender.Client) error {
				var err error
				msg, err = c.SendPollSimple(ctx, int64(-100123), "Q?", polls100Options(n))
				return err
			})
			require.NoError(t, err)
			require.NotNil(t, msg)
			assert.Equal(t, 42, msg.MessageID)
			parts := make([]string, n)
			for i := range parts {
				parts[i] = fmt.Sprintf(`{"text":"o%d"}`, i)
			}
			assert.Equal(t, "["+strings.Join(parts, ",")+"]", args["options"])
		})
	}
}

// T-s-5: 0 and 13 or more options are refused before any request, with the whole error text (T1-03).
func TestPolls100_SimpleInvalidCounts(t *testing.T) {
	for _, n := range []int{0, 13, 14, 20} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			server := testutil.NewMockServer(t)
			client := testutil.NewTestClient(t, server.BaseURL())
			_, err := client.SendPollSimple(context.Background(), int64(-100123), "Q?", polls100Options(n))
			var ve *tg.ValidationError
			require.True(t, errors.As(err, &ve), "error %v", err)
			assert.Equal(t, "options", ve.Field)
			want := "cannot exceed 12 options"
			if n == 0 {
				want = "must have at least 1 option"
			}
			assert.Equal(t, tg.NewValidationError("options", want).Error(), err.Error())
			assert.Equal(t, 0, server.CaptureCount())
		})
	}
}

// T-s-7: a sendPoll reply with option media (probe 21, edited as tg/api100c_test.go says) decodes whole,
// envelope included.
func TestPolls100_SendPollReply(t *testing.T) {
	server := testutil.NewMockServer(t)
	server.On("/bot"+testutil.TestToken+"/sendPoll", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":` + polls100Probe21 + `}`))
	})
	client := testutil.NewTestClient(t, server.BaseURL())
	msg, err := client.SendPoll(context.Background(), polls100Req())
	require.NoError(t, err)
	require.NotNil(t, msg.Poll)
	ph := []tg.PhotoSize{
		{FileID: "PH90-file", FileUniqueID: "PH90-uniq", Width: 90, Height: 90, FileSize: 341},
		{FileID: "PH100-file", FileUniqueID: "PH100-uniq", Width: 100, Height: 100, FileSize: 361},
	}
	loc := &tg.Location{Latitude: 52.520005, Longitude: 13.404999}
	want := []tg.PollOption{
		{Text: "photo", PersistentID: "0", Media: &tg.PollMedia{Photo: ph}},
		{Text: "loc", PersistentID: "1", Media: &tg.PollMedia{Location: loc}},
		{Text: "venue", PersistentID: "2", Media: &tg.PollMedia{Venue: &tg.Venue{
			Location: tg.Location{Latitude: 48.858396, Longitude: 2.294497}, Title: "probe venue", Address: "probe address"}}},
		{Text: "sticker", PersistentID: "3", Media: &tg.PollMedia{Sticker: &tg.Sticker{FileID: "STK-file",
			FileUniqueID: "STK-uniq", Type: "regular", Width: 512, Height: 512, FileSize: 1961,
			Thumbnail: &tg.PhotoSize{FileID: "STKTH-file", FileUniqueID: "STKTH-uniq", Width: 320, Height: 320, FileSize: 4109}}}},
		{Text: "video", PersistentID: "4", Media: &tg.PollMedia{Video: &tg.Video{FileID: "VID-file",
			FileUniqueID: "VID-uniq", Width: 320, Height: 240, Duration: 1, FileName: "v.mp4", MimeType: "video/mp4",
			FileSize: 663, Thumbnail: &tg.PhotoSize{FileID: "VIDTH-file", FileUniqueID: "VIDTH-uniq", Width: 320,
				Height: 240, FileSize: 733}}}},
		{Text: "live", PersistentID: "5", Media: &tg.PollMedia{LivePhoto: &tg.LivePhoto{Photo: ph, FileID: "LIVE-file",
			FileUniqueID: "LIVE-uniq", Width: 320, Height: 240, Duration: 1, MimeType: "video/mp4", FileSize: 663}}},
	}
	assert.Equal(t, tg.Message{MessageID: 1101, Date: 1793928592,
		From: &tg.User{ID: 123456789, IsBot: true, FirstName: "Test Bot", Username: "testbot"},
		Chat: &tg.Chat{ID: 123456789, Title: "Test Chat", IsForum: true, Type: "supergroup"},
		Poll: &tg.Poll{ID: "poll-21", Question: "probe100c option media", Options: want, Type: "regular",
			AllowsRevoting: true}}, *msg) // the whole message (c1 C1-03)
}
