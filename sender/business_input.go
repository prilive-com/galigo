package sender

// InputStoryContent represents content for a story.
// Lives in sender/ because it may contain InputFile.
type InputStoryContent interface {
	inputStoryContentTag()
}

// InputStoryContentPhoto represents a photo for a story.
type InputStoryContentPhoto struct {
	Photo InputFile `json:"-"` // Handled by multipart encoder
}

func (InputStoryContentPhoto) inputStoryContentTag() {}

// InputStoryContentVideo represents a video for a story.
type InputStoryContentVideo struct {
	Video          InputFile `json:"-"` // Handled by multipart encoder
	Duration       float64   `json:"duration,omitempty"`
	CoverFrameTime float64   `json:"cover_frame_time,omitempty"`
	IsAnimation    bool      `json:"is_animation,omitempty"`
}

func (InputStoryContentVideo) inputStoryContentTag() {}

// InputProfilePhoto represents a profile photo to set.
type InputProfilePhoto interface {
	inputProfilePhotoTag()
}

// InputProfilePhotoStatic represents a static profile photo.
type InputProfilePhotoStatic struct {
	Photo InputFile `json:"-"` // Handled by multipart encoder
}

func (InputProfilePhotoStatic) inputProfilePhotoTag() {}

// InputProfilePhotoAnimated represents an animated profile photo.
type InputProfilePhotoAnimated struct {
	Animation     InputFile `json:"-"` // Handled by multipart encoder
	MainFrameTime float64   `json:"main_frame_time,omitempty"`
}

func (InputProfilePhotoAnimated) inputProfilePhotoTag() {}
