package sender

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// FilePart represents a file to be uploaded via multipart.
type FilePart struct {
	FieldName string    // e.g., "photo", "document", "thumbnail"
	FileName  string    // e.g., "photo.jpg"
	Reader    io.Reader // File content
}

// MultipartRequest represents a request with files and parameters.
type MultipartRequest struct {
	Files  []FilePart        // Explicit file parts
	Params map[string]string // String-encoded parameters
}

// MultipartEncoder encodes requests as multipart/form-data.
type MultipartEncoder struct {
	w *multipart.Writer
}

// NewMultipartEncoder creates a new multipart encoder.
func NewMultipartEncoder(w io.Writer) *MultipartEncoder {
	return &MultipartEncoder{
		w: multipart.NewWriter(w),
	}
}

// ContentType returns the Content-Type header value including boundary.
func (e *MultipartEncoder) ContentType() string {
	return e.w.FormDataContentType()
}

// Close closes the multipart writer.
func (e *MultipartEncoder) Close() error {
	return e.w.Close()
}

// Encode writes the multipart request.
func (e *MultipartEncoder) Encode(req MultipartRequest) error {
	// 1. Write all file parts (explicit, type-safe)
	for _, file := range req.Files {
		if err := e.writeFile(file); err != nil {
			return fmt.Errorf("file %s: %w", file.FieldName, err)
		}
	}

	// 2. Write all parameter fields
	for name, value := range req.Params {
		if err := e.w.WriteField(name, value); err != nil {
			return fmt.Errorf("param %s: %w", name, err)
		}
	}

	return nil
}

func (e *MultipartEncoder) writeFile(file FilePart) error {
	part, err := e.w.CreateFormFile(file.FieldName, file.FileName)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}

	// Stream directly - no buffering
	_, err = io.Copy(part, file.Reader)
	return err
}

// BuildMultipartRequest creates a MultipartRequest from a typed request struct.
// Uses reflection for field iteration, but explicit handling for known types.
func BuildMultipartRequest(req any) (MultipartRequest, error) {
	result, _, err := buildMultipartRequest(req)
	return result, err
}

// buildMultipartRequest is BuildMultipartRequest plus formOnly: true when a
// field (InputMedia, []InputFile) is encoded by a handler whose value
// json.Marshal of the payload would not produce, so the request must be sent
// as multipart/form-data even without files (B-45).
func buildMultipartRequest(req any) (MultipartRequest, bool, error) {
	formOnly := false
	result := MultipartRequest{
		Files:  make([]FilePart, 0),
		Params: make(map[string]string),
	}

	rv := reflect.ValueOf(req)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	rt := rv.Type()
	attachIdx := 0

	for i := range rt.NumField() {
		field := rt.Field(i)
		value := rv.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Skip zero values (omitempty behavior)
		if value.IsZero() {
			continue
		}

		// Get JSON field name
		fieldName := getJSONFieldName(field)
		if fieldName == "-" {
			continue
		}

		// Handle by type (explicit, fast path)
		switch v := value.Interface().(type) {
		case InputFile:
			if err := handleInputFile(&result, fieldName, v, &attachIdx); err != nil {
				return result, false, fmt.Errorf("field %s: %w", fieldName, err)
			}

		case *InputFile:
			if v != nil {
				if err := handleInputFile(&result, fieldName, *v, &attachIdx); err != nil {
					return result, false, fmt.Errorf("field %s: %w", fieldName, err)
				}
			}

		case []InputFile:
			formOnly = true
			if err := handleInputFileSlice(&result, fieldName, v, &attachIdx); err != nil {
				return result, false, fmt.Errorf("field %s: %w", fieldName, err)
			}

		case InputMedia:
			formOnly = true
			if err := handleInputMedia(&result, fieldName, v, &attachIdx); err != nil {
				return result, false, fmt.Errorf("field %s: %w", fieldName, err)
			}

		case *InputMedia, []InputPollOption: // poll media (Bot API 10.0); nil is skipped above
			needsForm, err := handlePollField(&result, fieldName, v, &attachIdx)
			if err != nil {
				return result, false, fmt.Errorf("field %s: %w", fieldName, err)
			}
			formOnly = formOnly || needsForm

		case string:
			result.Params[fieldName] = v

		case int:
			result.Params[fieldName] = strconv.Itoa(v)

		case int64:
			result.Params[fieldName] = strconv.FormatInt(v, 10)

		case float64:
			result.Params[fieldName] = strconv.FormatFloat(v, 'f', -1, 64)

		case bool:
			result.Params[fieldName] = strconv.FormatBool(v)

		case []FilePart:
			// Direct file parts (e.g., sticker uploads with attach:// references)
			result.Files = append(result.Files, v...)

		default:
			// Handle named string types (tg.ParseMode, tg.ChatType, etc.)
			// These don't match "case string:" because they're distinct types,
			// but we need to serialize them as plain strings, not JSON-encoded.
			// See: https://github.com/prilive-com/galigo/issues/5
			if value.Kind() == reflect.String {
				result.Params[fieldName] = value.String()
				continue
			}

			// Complex types (structs, slices, maps) -> JSON encode
			data, err := json.Marshal(v)
			if err != nil {
				return result, false, fmt.Errorf("field %s: JSON marshal: %w", fieldName, err)
			}
			result.Params[fieldName] = formValue(data)
		}
	}

	return result, formOnly, nil
}

// handlePollField writes a poll media field (Bot API 10.0) and reports whether
// it needs multipart/form-data: a *InputMedia always does; the options only
// when an option has media (without option media the text is the same JSON as
// before, so the request is sent as JSON unless another field needs multipart).
func handlePollField(req *MultipartRequest, fieldName string, v any, attachIdx *int) (bool, error) {
	switch v := v.(type) {
	case *InputMedia:
		if v == nil { // a nil pointer held in an `any` field: written like a nil *InputFile (nothing)
			return false, nil
		}
		return true, handleInputMedia(req, fieldName, *v, attachIdx)
	case []InputPollOption:
		hasMedia := slices.ContainsFunc(v, func(o InputPollOption) bool { return o.Media != nil })
		return hasMedia, handlePollOptions(req, fieldName, v, attachIdx)
	}
	return false, fmt.Errorf("unexpected poll field type %T", v)
}

// handlePollOptions writes the options of a poll (Bot API 10.0 option media):
// each option keeps its own JSON fields, and its media is the item
// inputMediaItem builds (uploads take the next attach names).
func handlePollOptions(req *MultipartRequest, fieldName string, options []InputPollOption, attachIdx *int) error {
	type wireOption struct {
		InputPollOption
		Media map[string]any `json:"media,omitempty"` // hides the embedded Media
	}
	out := make([]wireOption, len(options))
	for i, o := range options {
		out[i].InputPollOption = o
		if o.Media != nil {
			item, err := inputMediaItem(req, *o.Media, attachIdx)
			if err != nil {
				return fmt.Errorf("option %d: %w", i, err)
			}
			out[i].Media = item
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("JSON marshal poll options: %w", err)
	}
	req.Params[fieldName] = string(data)
	return nil
}

// formValue returns the form text of a JSON-encoded value. A JSON string (a
// named string in an any field, a json.RawMessage holding a string) is sent
// as its decoded text: the Bot API server reads a JSON body's string value the
// same way (B-45). Any other JSON value is sent as its JSON text.
func formValue(data []byte) string {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err == nil {
			return s
		}
	}
	return string(data)
}

func handleInputMedia(req *MultipartRequest, fieldName string, media InputMedia, attachIdx *int) error {
	item, err := inputMediaItem(req, media, attachIdx)
	if err != nil {
		return err
	}
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("JSON marshal InputMedia: %w", err)
	}
	req.Params[fieldName] = string(data)

	return nil
}

// inputMediaItem builds the JSON item of one InputMedia and adds its upload
// parts to req. A location or venue (Bot API 10.0) or a link (Bot API 10.1) has
// no file: a file on it is an error, so it is not dropped without a word.
func inputMediaItem(req *MultipartRequest, media InputMedia, attachIdx *int) (map[string]any, error) {
	item := map[string]any{
		"type": media.Type,
	}

	switch media.Type {
	case "location", "venue", "link":
		f := media.Media
		if f.FileID != "" || f.URL != "" || f.Reader != nil || f.Source != nil || f.Photo != nil {
			return nil, fmt.Errorf("InputMedia: %s has no media file", media.Type)
		}
		if media.Type == "link" {
			item["url"] = media.URL
			return item, nil
		}
		item["latitude"] = media.Latitude
		item["longitude"] = media.Longitude
		if media.HorizontalAccuracy != 0 {
			item["horizontal_accuracy"] = media.HorizontalAccuracy
		}
		if media.Type == "venue" {
			item["title"] = media.Title
			item["address"] = media.Address
			for key, val := range map[string]string{
				"foursquare_id":     media.FoursquareID,
				"foursquare_type":   media.FoursquareType,
				"google_place_id":   media.GooglePlaceID,
				"google_place_type": media.GooglePlaceType,
			} {
				if val != "" {
					item[key] = val
				}
			}
		}
		return item, nil
	}

	switch {
	case media.Media.FileID != "":
		item["media"] = media.Media.FileID

	case media.Media.URL != "":
		item["media"] = media.Media.URL

	case media.Media.Reader != nil || media.Media.Source != nil:
		attachName := fmt.Sprintf("file%d", *attachIdx)
		*attachIdx++

		item["media"] = "attach://" + attachName
		req.Files = append(req.Files, FilePart{
			FieldName: attachName,
			FileName:  media.Media.FileName,
			Reader:    media.Media.OpenReader(),
		})

	default:
		return nil, fmt.Errorf("InputMedia.Media must have FileID, URL, or Reader set")
	}

	if media.Media.Photo != nil {
		ref, err := attachRef(req, *media.Media.Photo, attachIdx)
		if err != nil {
			return nil, fmt.Errorf("InputMedia.Media.Photo: %w", err)
		}
		item["photo"] = ref
	} else if media.Type == "live_photo" {
		return nil, fmt.Errorf("InputMedia: live_photo requires Media.Photo")
	}

	if media.Emoji != "" {
		item["emoji"] = media.Emoji
	}
	if media.Caption != "" {
		item["caption"] = media.Caption
	}
	if media.ParseMode != "" {
		item["parse_mode"] = media.ParseMode
	}

	return item, nil
}

func handleInputFile(req *MultipartRequest, fieldName string, file InputFile, attachIdx *int) error {
	switch {
	case file.FileID != "":
		req.Params[fieldName] = file.FileID

	case file.URL != "":
		req.Params[fieldName] = file.URL

	case file.Reader != nil || file.Source != nil:
		// For single file uploads (sendDocument, sendPhoto, etc.),
		// put file directly in field with the correct name.
		// The attach:// syntax is only for sendMediaGroup.
		req.Files = append(req.Files, FilePart{
			FieldName: fieldName, // Use actual field name: "document", "photo", etc.
			FileName:  file.FileName,
			Reader:    file.OpenReader(),
		})
		// Don't add to Params - the file IS the value

	default:
		return fmt.Errorf("InputFile must have FileID, URL, or Reader set")
	}

	return nil
}

func handleInputFileSlice(req *MultipartRequest, fieldName string, files []InputFile, attachIdx *int) error {
	// For media groups, each file needs attach:// reference
	mediaItems := make([]map[string]any, 0, len(files))

	for i, file := range files {
		item := map[string]any{
			"type": file.MediaType,
		}

		switch {
		case file.FileID != "":
			item["media"] = file.FileID

		case file.URL != "":
			item["media"] = file.URL

		case file.Reader != nil || file.Source != nil:
			attachName := fmt.Sprintf("file%d", *attachIdx)
			*attachIdx++

			item["media"] = "attach://" + attachName
			req.Files = append(req.Files, FilePart{
				FieldName: attachName,
				FileName:  file.FileName,
				Reader:    file.OpenReader(),
			})

		default:
			return fmt.Errorf("item %d: InputFile must have FileID, URL, or Reader set", i)
		}

		if file.Photo != nil {
			ref, err := attachRef(req, *file.Photo, attachIdx)
			if err != nil {
				return fmt.Errorf("item %d: photo: %w", i, err)
			}
			item["photo"] = ref
		} else if file.MediaType == "live_photo" {
			return fmt.Errorf("item %d: live_photo requires Photo", i)
		}

		if file.Caption != "" {
			item["caption"] = file.Caption
		}
		if file.ParseMode != "" {
			item["parse_mode"] = file.ParseMode
		}

		mediaItems = append(mediaItems, item)
	}

	data, err := json.Marshal(mediaItems)
	if err != nil {
		return fmt.Errorf("JSON marshal media array: %w", err)
	}
	req.Params[fieldName] = string(data)

	return nil
}

// attachRef returns the media value of f inside an item (file id, URL, or
// attach://fileN for an upload). An upload adds its file part and takes the
// next attach index; a file id or URL takes none.
func attachRef(req *MultipartRequest, f InputFile, attachIdx *int) (string, error) {
	ref, part, err := resolveInputFile(f, fmt.Sprintf("file%d", *attachIdx))
	if err != nil {
		return "", err
	}
	if part != nil {
		req.Files = append(req.Files, *part)
		*attachIdx++
	}
	return ref, nil
}

func getJSONFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return strings.ToLower(field.Name)
	}
	parts := strings.Split(tag, ",")
	return parts[0]
}

// HasUploads returns true if the request contains file uploads.
func (r MultipartRequest) HasUploads() bool {
	return len(r.Files) > 0
}
