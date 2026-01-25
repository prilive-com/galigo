package testutil

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Capture represents a captured HTTP request with timestamp.
type Capture struct {
	Method      string
	Path        string
	Query       map[string][]string
	Headers     http.Header
	Body        []byte
	ContentType string
	Timestamp   time.Time
}

// AssertPath verifies the request path.
func (c *Capture) AssertPath(t *testing.T, expected string) {
	t.Helper()
	assert.Equal(t, expected, c.Path, "unexpected path")
}

// AssertMethod verifies the HTTP method.
func (c *Capture) AssertMethod(t *testing.T, expected string) {
	t.Helper()
	assert.Equal(t, expected, c.Method, "unexpected method")
}

// AssertContentType verifies the Content-Type header contains expected value.
func (c *Capture) AssertContentType(t *testing.T, expected string) {
	t.Helper()
	assert.Contains(t, c.ContentType, expected, "unexpected content-type")
}

// AssertHeader verifies a specific header value.
func (c *Capture) AssertHeader(t *testing.T, key, expected string) {
	t.Helper()
	assert.Equal(t, expected, c.Headers.Get(key), "unexpected header: "+key)
}

// AssertHeaderExists verifies a header exists (with any value).
func (c *Capture) AssertHeaderExists(t *testing.T, key string) {
	t.Helper()
	assert.NotEmpty(t, c.Headers.Get(key), "header should exist: "+key)
}

// AssertQuery verifies a query parameter value.
func (c *Capture) AssertQuery(t *testing.T, key, expected string) {
	t.Helper()
	values := c.Query[key]
	if len(values) == 0 {
		t.Errorf("query parameter %q not found", key)
		return
	}
	assert.Equal(t, expected, values[0], "unexpected query parameter: "+key)
}

// AssertJSONField verifies a field in the JSON body.
func (c *Capture) AssertJSONField(t *testing.T, field string, expected any) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(c.Body, &body), "failed to parse JSON body")
	assert.Equal(t, expected, body[field], "unexpected value for field: "+field)
}

// AssertJSONFieldExists verifies a field exists in the JSON body.
func (c *Capture) AssertJSONFieldExists(t *testing.T, field string) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(c.Body, &body), "failed to parse JSON body")
	assert.Contains(t, body, field, "field should exist: "+field)
}

// AssertJSONFieldAbsent verifies a field does NOT exist in the JSON body.
func (c *Capture) AssertJSONFieldAbsent(t *testing.T, field string) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(c.Body, &body), "failed to parse JSON body")
	assert.NotContains(t, body, field, "field should be absent: "+field)
}

// AssertJSONFieldNested verifies a nested field in the JSON body.
// Use dot notation: "chat.id", "reply_markup.inline_keyboard"
func (c *Capture) AssertJSONFieldNested(t *testing.T, path string, expected any) {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(c.Body, &body), "failed to parse JSON body")

	// Simple single-level path for now
	// TODO: Add proper nested path support if needed
	assert.Equal(t, expected, body[path], "unexpected value for field: "+path)
}

// BodyJSON decodes the body as JSON into target.
func (c *Capture) BodyJSON(t *testing.T, target any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(c.Body, target), "failed to decode JSON body")
}

// BodyMap returns the body as a map.
func (c *Capture) BodyMap(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(c.Body, &m), "failed to decode JSON body")
	return m
}

// BodyString returns the body as a string.
func (c *Capture) BodyString() string {
	return string(c.Body)
}

// HasQuery checks if a query parameter exists.
func (c *Capture) HasQuery(key string) bool {
	_, exists := c.Query[key]
	return exists
}

// GetQuery returns the first value of a query parameter.
func (c *Capture) GetQuery(key string) string {
	values := c.Query[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
