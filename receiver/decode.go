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

// pollBatch is one getUpdates result. errs is nil when every update decoded (the common case: updates is the
// decoded result, not a copy). Otherwise errs has one entry per update: errs[i] != nil marks an element that did not
// decode, and updates[i] then holds only its UpdateID.
type pollBatch struct {
	updates []tg.Update
	errs    []error
}

// splitUpdates is the fallback when the whole getUpdates body does not decode. It returns the elements, with the
// ones that fail as drop items, only when the envelope is valid (ok is true, exactly one "result" member), at least
// one element fails, every failing element has a usable update_id and every decoded update_id is below
// math.MaxInt64. Otherwise it returns false, and the caller returns its own decode error.
func splitUpdates(body []byte) (pollBatch, bool) {
	var raw apiResponse // the same envelope fields as getUpdatesResponse
	if err := json.Unmarshal(body, &raw); err != nil || !raw.OK {
		return pollBatch{}, false
	}
	if _, ok := oneMember(body, "result"); !ok {
		return pollBatch{}, false
	}
	var elems []json.RawMessage
	if err := json.Unmarshal(raw.Result, &elems); err != nil {
		return pollBatch{}, false
	}
	batch := pollBatch{updates: make([]tg.Update, 0, len(elems)), errs: make([]error, 0, len(elems))}
	failed := false
	for _, elem := range elems {
		u, id, ok, err := decodeUpdate(elem)
		if !ok || (err == nil && int64(u.UpdateID) == math.MaxInt64) {
			return pollBatch{}, false
		}
		failed = failed || err != nil
		u.UpdateID = id // the same id for a decoded update; the only field set for one that failed
		batch.updates = append(batch.updates, u)
		batch.errs = append(batch.errs, err)
	}
	if !failed {
		return pollBatch{}, false
	}
	return batch, true
}
