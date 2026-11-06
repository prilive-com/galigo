package sender_test

// Bot API 10.0 part d (B-25 slice 2d, TEST-PLAN §2, T-s-1 … T-s-2): InputMedia sticker, location and venue.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/sender"
)

// T-s-1: every new name has its planned type. A wrong field type or constructor signature does not compile
// (P1-01 openai).
func TestInputMedia100D_APIShapes(t *testing.T) {
	hasType[func(sender.InputFile) sender.InputMedia](sender.NewInputMediaSticker)
	hasType[func(float64, float64) sender.InputMedia](sender.NewInputMediaLocation)
	hasType[func(float64, float64, string, string) sender.InputMedia](sender.NewInputMediaVenue)

	var m sender.InputMedia
	hasType[string](m.Emoji)
	hasType[float64](m.Latitude)
	hasType[float64](m.Longitude)
	hasType[float64](m.HorizontalAccuracy)
	hasType[string](m.Title)
	hasType[string](m.Address)
	hasType[string](m.FoursquareID)
	hasType[string](m.FoursquareType)
	hasType[string](m.GooglePlaceID)
	hasType[string](m.GooglePlaceType)

	var r sender.SendPollRequest
	hasType[*sender.InputMedia](r.Media)
	hasType[*sender.InputMedia](r.ExplanationMedia)

	var o sender.InputPollOption
	hasType[*sender.InputMedia](o.Media)
}

// hasType compiles only when v's type is assignable to T: a func or named type must be identical, so a wrong
// field type or constructor signature is a compile error in this file.
func hasType[T any](T) {}

// T-s-2: each constructor returns the whole planned value (latitude ≠ longitude and title ≠ address, so a swap
// shows).
func TestInputMedia100D_Constructors(t *testing.T) {
	t.Run("sticker", func(t *testing.T) {
		assert.Equal(t, sender.InputMedia{Type: "sticker", Media: sender.FromFileID("S")},
			sender.NewInputMediaSticker(sender.FromFileID("S")))
	})
	t.Run("location", func(t *testing.T) {
		assert.Equal(t, sender.InputMedia{Type: "location", Latitude: 52.52, Longitude: 13.405},
			sender.NewInputMediaLocation(52.52, 13.405))
	})
	t.Run("venue", func(t *testing.T) {
		assert.Equal(t, sender.InputMedia{Type: "venue", Latitude: 48.8584, Longitude: 2.2945, Title: "T", Address: "A"},
			sender.NewInputMediaVenue(48.8584, 2.2945, "T", "A"))
	})
}
