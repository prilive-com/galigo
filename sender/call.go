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

// callJSONResult is a generic version for cleaner call sites.
// Requires Go 1.18+ generics.
//
// Usage:
//
//	info, err := callJSONResult[tg.ChatFullInfo](c, ctx, "getChat", req)
func callJSONResult[T any](c *Client, ctx context.Context, method string, payload any, chatIDs ...string) (T, error) {
	var result T
	if err := c.callJSON(ctx, method, payload, &result, chatIDs...); err != nil {
		var zero T
		return zero, err
	}
	return result, nil
}
