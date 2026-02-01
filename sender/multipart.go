package sender

import (
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"reflect"
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
	result := MultipartRequest{
		Files:  make([]FilePart, 0),
		Params: make(map[string]string),
	}

	rv := reflect.ValueOf(req)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}

	rt := rv.Type()
	attachIdx := 0

	for i := 0; i < rt.NumField(); i++ {
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
				return result, fmt.Errorf("field %s: %w", fieldName, err)
			}

		case *InputFile:
			if v != nil {
				if err := handleInputFile(&result, fieldName, *v, &attachIdx); err != nil {
					return result, fmt.Errorf("field %s: %w", fieldName, err)
				}
			}

		case []InputFile:
			if err := handleInputFileSlice(&result, fieldName, v, &attachIdx); err != nil {
				return result, fmt.Errorf("field %s: %w", fieldName, err)
			}

		case InputMedia:
			if err := handleInputMedia(&result, fieldName, v, &attachIdx); err != nil {
				return result, fmt.Errorf("field %s: %w", fieldName, err)
			}

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
			// Complex types (structs, slices, maps) -> JSON encode
			data, err := json.Marshal(v)
			if err != nil {
				return result, fmt.Errorf("field %s: JSON marshal: %w", fieldName, err)
			}
			result.Params[fieldName] = string(data)
		}
	}

	return result, nil
}

func handleInputMedia(req *MultipartRequest, fieldName string, media InputMedia, attachIdx *int) error {
	item := map[string]any{
		"type": media.Type,
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
		return fmt.Errorf("InputMedia.Media must have FileID, URL, or Reader set")
	}

	if media.Caption != "" {
		item["caption"] = media.Caption
	}
	if media.ParseMode != "" {
		item["parse_mode"] = media.ParseMode
	}

	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("JSON marshal InputMedia: %w", err)
	}
	req.Params[fieldName] = string(data)

	return nil
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
