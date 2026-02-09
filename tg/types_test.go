package tg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/prilive-com/galigo/tg"
)

func TestLinkPreviewOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    *tg.LinkPreviewOptions
		wantErr bool
	}{
		{
			name:    "nil options valid",
			opts:    nil,
			wantErr: false,
		},
		{
			name:    "empty options valid",
			opts:    &tg.LinkPreviewOptions{},
			wantErr: false,
		},
		{
			name: "disabled preview valid",
			opts: &tg.LinkPreviewOptions{
				IsDisabled: true,
			},
			wantErr: false,
		},
		{
			name: "prefer small media valid",
			opts: &tg.LinkPreviewOptions{
				PreferSmallMedia: true,
			},
			wantErr: false,
		},
		{
			name: "prefer large media valid",
			opts: &tg.LinkPreviewOptions{
				PreferLargeMedia: true,
			},
			wantErr: false,
		},
		{
			name: "show above text valid",
			opts: &tg.LinkPreviewOptions{
				ShowAboveText: true,
			},
			wantErr: false,
		},
		{
			name: "custom URL valid",
			opts: &tg.LinkPreviewOptions{
				URL: "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "full valid config",
			opts: &tg.LinkPreviewOptions{
				URL:              "https://example.com",
				PreferLargeMedia: true,
				ShowAboveText:    true,
			},
			wantErr: false,
		},
		{
			name: "mutually exclusive error",
			opts: &tg.LinkPreviewOptions{
				PreferSmallMedia: true,
				PreferLargeMedia: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "mutually exclusive")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
