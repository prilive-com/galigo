package sender

import (
	"context"
	"encoding/json"
	"fmt"
)

// callJSON is the unified internal helper for all API calls.
// It wraps executeRequest() and provides consistent JSON decoding.
//
// Usage:
//
//	var result tg.ChatFullInfo
//	if err := c.callJSON(ctx, "getChat", req, &result); err != nil {
//	    return nil, err
//	}
//	return &result, nil
func (c *Client) callJSON(ctx context.Context, method string, payload any, out any, chatIDs ...string) error {
	resp, err := c.executeRequest(ctx, method, payload, chatIDs...)
	if err != nil {
		return err
	}
	if out == nil {
		return nil // For methods that return bool/void
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return fmt.Errorf("galigo: %s: failed to parse response: %w", method, err)
	}
	return nil
}
