package sender

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateChatID(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		wantErr bool
		errMsg  string
	}{
		{"valid int64", int64(123456), false, ""},
		{"valid negative int64", int64(-1001234567890), false, ""},
		{"valid int", int(123456), false, ""},
		{"valid username", "@testchannel", false, ""},
		{"zero int64", int64(0), true, "cannot be zero"},
		{"zero int", int(0), true, "cannot be zero"},
		{"empty string", "", true, "cannot be empty"},
		{"nil", nil, true, "is required"},
		{"invalid type float", 123.456, true, "must be int64"},
		{"invalid type struct", struct{}{}, true, "must be int64"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateChatID(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateUserID(t *testing.T) {
	tests := []struct {
		name    string
		input   int64
		wantErr bool
	}{
		{"valid", 123456, false},
		{"zero", 0, true},
		{"negative", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUserID(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMessageID(t *testing.T) {
	tests := []struct {
		name    string
		input   int
		wantErr bool
	}{
		{"valid", 1, false},
		{"zero", 0, true},
		{"negative", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessageID(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMessageIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   []int
		wantErr bool
		errMsg  string
	}{
		{"valid single", []int{1}, false, ""},
		{"valid multiple", []int{1, 2, 3}, false, ""},
		{"empty", []int{}, true, "cannot be empty"},
		{"nil", nil, true, "cannot be empty"},
		{"contains zero", []int{1, 0, 3}, true, "must be positive"},
		{"contains negative", []int{1, -1, 3}, true, "must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMessageIDs(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateThreadID(t *testing.T) {
	tests := []struct {
		name    string
		input   int
		wantErr bool
	}{
		{"valid", 1, false},
		{"large valid", 999999, false},
		{"zero", 0, true},
		{"negative", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateThreadID(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "message_thread_id")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMessageIDs_TooMany(t *testing.T) {
	ids := make([]int, 101)
	for i := range ids {
		ids[i] = i + 1
	}
	err := validateMessageIDs(ids)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot exceed 100")
}
