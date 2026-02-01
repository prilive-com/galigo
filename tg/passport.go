package tg

// PassportElementError describes an error in Telegram Passport data.
// This is a SEND-ONLY type — no UnmarshalJSON needed.
type PassportElementError interface {
	passportElementErrorTag()
	GetSource() string
}

// PassportElementErrorDataField represents an error in a data field.
type PassportElementErrorDataField struct {
	Source    string `json:"source"` // Always "data"
	Type      string `json:"type"`
	FieldName string `json:"field_name"`
	DataHash  string `json:"data_hash"`
	Message   string `json:"message"`
}

func (PassportElementErrorDataField) passportElementErrorTag() {}
func (PassportElementErrorDataField) GetSource() string        { return "data" }

// PassportElementErrorFrontSide represents an error with the front side.
type PassportElementErrorFrontSide struct {
	Source   string `json:"source"` // Always "front_side"
	Type     string `json:"type"`
	FileHash string `json:"file_hash"`
	Message  string `json:"message"`
}

func (PassportElementErrorFrontSide) passportElementErrorTag() {}
func (PassportElementErrorFrontSide) GetSource() string        { return "front_side" }

// PassportElementErrorReverseSide represents an error with the reverse side.
type PassportElementErrorReverseSide struct {
	Source   string `json:"source"` // Always "reverse_side"
	Type     string `json:"type"`
	FileHash string `json:"file_hash"`
	Message  string `json:"message"`
}

func (PassportElementErrorReverseSide) passportElementErrorTag() {}
func (PassportElementErrorReverseSide) GetSource() string        { return "reverse_side" }

// PassportElementErrorSelfie represents an error with the selfie.
type PassportElementErrorSelfie struct {
	Source   string `json:"source"` // Always "selfie"
	Type     string `json:"type"`
	FileHash string `json:"file_hash"`
	Message  string `json:"message"`
}

func (PassportElementErrorSelfie) passportElementErrorTag() {}
func (PassportElementErrorSelfie) GetSource() string        { return "selfie" }

// PassportElementErrorFile represents an error with a document scan.
type PassportElementErrorFile struct {
	Source   string `json:"source"` // Always "file"
	Type     string `json:"type"`
	FileHash string `json:"file_hash"`
	Message  string `json:"message"`
}

func (PassportElementErrorFile) passportElementErrorTag() {}
func (PassportElementErrorFile) GetSource() string        { return "file" }

// PassportElementErrorFiles represents an error with multiple document scans.
type PassportElementErrorFiles struct {
	Source     string   `json:"source"` // Always "files"
	Type       string   `json:"type"`
	FileHashes []string `json:"file_hashes"`
	Message    string   `json:"message"`
}

func (PassportElementErrorFiles) passportElementErrorTag() {}
func (PassportElementErrorFiles) GetSource() string        { return "files" }

// PassportElementErrorTranslationFile represents an error with one translation file.
type PassportElementErrorTranslationFile struct {
	Source   string `json:"source"` // Always "translation_file"
	Type     string `json:"type"`
	FileHash string `json:"file_hash"`
	Message  string `json:"message"`
}

func (PassportElementErrorTranslationFile) passportElementErrorTag() {}
func (PassportElementErrorTranslationFile) GetSource() string        { return "translation_file" }

// PassportElementErrorTranslationFiles represents an error with translation files.
type PassportElementErrorTranslationFiles struct {
	Source     string   `json:"source"` // Always "translation_files"
	Type       string   `json:"type"`
	FileHashes []string `json:"file_hashes"`
	Message    string   `json:"message"`
}

func (PassportElementErrorTranslationFiles) passportElementErrorTag() {}
func (PassportElementErrorTranslationFiles) GetSource() string        { return "translation_files" }

// PassportElementErrorUnspecified represents an unspecified error.
type PassportElementErrorUnspecified struct {
	Source      string `json:"source"` // Always "unspecified"
	Type        string `json:"type"`
	ElementHash string `json:"element_hash"`
	Message     string `json:"message"`
}

func (PassportElementErrorUnspecified) passportElementErrorTag() {}
func (PassportElementErrorUnspecified) GetSource() string        { return "unspecified" }
