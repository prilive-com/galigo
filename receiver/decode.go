package receiver

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/prilive-com/galigo/tg"
)

// oneMember returns the raw value of obj's top-level member name when obj is a JSON object with exactly one member
// whose name equals name ignoring case, and that member is spelled exactly name. Otherwise it returns nil, false.
// Trailing data after the object (other than white space) also returns nil, false.
func oneMember(obj []byte, name string) (json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(obj))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	var value json.RawMessage
	matches := 0
	exact := false
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, _ := tok.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, false
		}
		if strings.EqualFold(key, name) {
			matches++
			exact = key == name
			value = v
		}
	}
	if _, err := dec.Token(); err != nil { // the closing brace: the first token was '{'
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF { //nolint:errorlint // Token returns io.EOF itself at the end of the input
		return nil, false
	}
	if matches != 1 || !exact {
		return nil, false
	}
	return value, true
}

// decodeUpdate decodes one update. If the full decode succeeds it returns the update, its UpdateID, ok=true and
// err=nil (today's result). If it fails and raw has a usable update_id (one top-level member spelled exactly
// "update_id", an integer from 1 to math.MaxInt, not math.MaxInt64), it returns a zero update, that id, ok=true and
// the decode error. Otherwise it returns ok=false: the caller cannot confirm the input.
func decodeUpdate(raw []byte) (u tg.Update, id int, ok bool, err error) {
	if err = json.Unmarshal(raw, &u); err == nil {
		return u, u.UpdateID, true, nil
	}
	v, found := oneMember(raw, "update_id")
	n, perr := strconv.ParseInt(string(v), 10, 64)
	if !found || perr != nil || n < 1 || n > math.MaxInt || n == math.MaxInt64 {
		return tg.Update{}, 0, false, err
	}
	return tg.Update{}, int(n), true, err
}

// pollUpdate is one element of a getUpdates result: a decoded update (err == nil), or the id of an element that
// did not decode and its error.
type pollUpdate struct {
	update tg.Update
	id     int
	err    error
}

// splitUpdates is the fallback when the whole getUpdates body does not decode. It returns the elements, with the
// ones that fail as drop items, only when the envelope is valid (ok is true, exactly one "result" member), at least
// one element fails, every failing element has a usable update_id and every decoded update_id is below
// math.MaxInt64. Otherwise it returns false, and the caller returns its own decode error.
func splitUpdates(body []byte) ([]pollUpdate, bool) {
	var raw apiResponse // the same envelope fields as getUpdatesResponse
	if err := json.Unmarshal(body, &raw); err != nil || !raw.OK {
		return nil, false
	}
	if _, ok := oneMember(body, "result"); !ok {
		return nil, false
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(raw.Result, &elems); err != nil {
		return nil, false
	}
	items := make([]pollUpdate, 0, len(elems))
	failed := false
	for _, elem := range elems {
		u, id, ok, err := decodeUpdate(elem)
		if !ok || (err == nil && int64(u.UpdateID) == math.MaxInt64) {
			return nil, false
		}
		failed = failed || err != nil
		items = append(items, pollUpdate{update: u, id: id, err: err})
	}
	if !failed {
		return nil, false
	}
	return items, true
}
