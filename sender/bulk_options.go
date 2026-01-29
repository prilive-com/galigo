package sender

// ================== Typed Bulk Options ==================

// ForwardMessagesOption configures ForwardMessages.
// This is a typed option - it only works with ForwardMessagesRequest.
type ForwardMessagesOption func(*ForwardMessagesRequest)

// WithForwardSilent forwards messages without notification.
func WithForwardSilent() ForwardMessagesOption {
	return func(r *ForwardMessagesRequest) {
		r.DisableNotification = true
	}
}

// WithForwardProtected protects forwarded messages from further forwarding.
func WithForwardProtected() ForwardMessagesOption {
	return func(r *ForwardMessagesRequest) {
		r.ProtectContent = true
	}
}

// CopyMessagesOption configures CopyMessages.
// This is a typed option - it only works with CopyMessagesRequest.
type CopyMessagesOption func(*CopyMessagesRequest)

// WithCopySilent copies messages without notification.
func WithCopySilent() CopyMessagesOption {
	return func(r *CopyMessagesRequest) {
		r.DisableNotification = true
	}
}

// WithCopyProtected protects copied messages from further forwarding.
func WithCopyProtected() CopyMessagesOption {
	return func(r *CopyMessagesRequest) {
		r.ProtectContent = true
	}
}

// WithRemoveCaption removes captions from copied messages.
func WithRemoveCaption() CopyMessagesOption {
	return func(r *CopyMessagesRequest) {
		r.RemoveCaption = true
	}
}
