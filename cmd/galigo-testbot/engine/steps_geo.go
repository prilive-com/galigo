package engine

import (
	"context"
)

// SendLocationStep sends a GPS location.
type SendLocationStep struct {
	Latitude  float64
	Longitude float64
}

func (s *SendLocationStep) Name() string { return "sendLocation" }

func (s *SendLocationStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	lat := s.Latitude
	if lat == 0 {
		lat = 48.8584 // Eiffel Tower
	}
	lon := s.Longitude
	if lon == 0 {
		lon = 2.2945
	}

	msg, err := rt.Sender.SendLocation(ctx, rt.ChatID, lat, lon)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendLocation",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"latitude":   lat,
			"longitude":  lon,
		},
	}, nil
}

// SendVenueStep sends a venue.
type SendVenueStep struct {
	Latitude  float64
	Longitude float64
	Title     string
	Address   string
}

func (s *SendVenueStep) Name() string { return "sendVenue" }

func (s *SendVenueStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	lat := s.Latitude
	if lat == 0 {
		lat = 48.8584
	}
	lon := s.Longitude
	if lon == 0 {
		lon = 2.2945
	}
	title := s.Title
	if title == "" {
		title = "Test Venue"
	}
	address := s.Address
	if address == "" {
		address = "Paris, France"
	}

	msg, err := rt.Sender.SendVenue(ctx, rt.ChatID, lat, lon, title, address)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendVenue",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id": msg.MessageID,
			"title":      title,
			"address":    address,
		},
	}, nil
}

// SendContactStep sends a phone contact.
type SendContactStep struct {
	PhoneNumber string
	FirstName   string
	LastName    string
}

func (s *SendContactStep) Name() string { return "sendContact" }

func (s *SendContactStep) Execute(ctx context.Context, rt *Runtime) (*StepResult, error) {
	phone := s.PhoneNumber
	if phone == "" {
		phone = "+10000000000"
	}
	firstName := s.FirstName
	if firstName == "" {
		firstName = "Galigo"
	}
	lastName := s.LastName
	if lastName == "" {
		lastName = "Test"
	}

	msg, err := rt.Sender.SendContact(ctx, rt.ChatID, phone, firstName, lastName)
	if err != nil {
		return nil, err
	}

	rt.LastMessage = msg
	rt.TrackMessage(rt.ChatID, msg.MessageID)

	return &StepResult{
		Method:     "sendContact",
		MessageIDs: []int{msg.MessageID},
		Evidence: map[string]any{
			"message_id":   msg.MessageID,
			"phone_number": phone,
			"first_name":   firstName,
		},
	}, nil
}
