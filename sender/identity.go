package sender

import (
	"context"
	"regexp"

	"github.com/prilive-com/galigo/tg"
)

// ================== Request Types ==================

// SetMyCommandsRequest represents a setMyCommands request.
type SetMyCommandsRequest struct {
	Commands     []tg.BotCommand     `json:"commands"`
	Scope        *tg.BotCommandScope `json:"scope,omitempty"`
	LanguageCode string              `json:"language_code,omitempty"`
}

// GetMyCommandsRequest represents a getMyCommands request.
type GetMyCommandsRequest struct {
	Scope        *tg.BotCommandScope `json:"scope,omitempty"`
	LanguageCode string              `json:"language_code,omitempty"`
}

// DeleteMyCommandsRequest represents a deleteMyCommands request.
type DeleteMyCommandsRequest struct {
	Scope        *tg.BotCommandScope `json:"scope,omitempty"`
	LanguageCode string              `json:"language_code,omitempty"`
}

// SetMyNameRequest represents a setMyName request.
type SetMyNameRequest struct {
	Name         string `json:"name,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// GetMyNameRequest represents a getMyName request.
type GetMyNameRequest struct {
	LanguageCode string `json:"language_code,omitempty"`
}

// SetMyDescriptionRequest represents a setMyDescription request.
type SetMyDescriptionRequest struct {
	Description  string `json:"description,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// GetMyDescriptionRequest represents a getMyDescription request.
type GetMyDescriptionRequest struct {
	LanguageCode string `json:"language_code,omitempty"`
}

// SetMyShortDescriptionRequest represents a setMyShortDescription request.
type SetMyShortDescriptionRequest struct {
	ShortDescription string `json:"short_description,omitempty"`
	LanguageCode     string `json:"language_code,omitempty"`
}

// GetMyShortDescriptionRequest represents a getMyShortDescription request.
type GetMyShortDescriptionRequest struct {
	LanguageCode string `json:"language_code,omitempty"`
}

// SetMyProfilePhotoRequest represents a setMyProfilePhoto request.
// Added in Bot API 9.4.
type SetMyProfilePhotoRequest struct {
	Photo      InputFile `json:"photo"`
	IsPersonal bool      `json:"is_personal,omitempty"`
}

// RemoveMyProfilePhotoRequest represents a removeMyProfilePhoto request.
// Added in Bot API 9.4.
type RemoveMyProfilePhotoRequest struct {
	IsPersonal bool `json:"is_personal,omitempty"`
}

// SetMyDefaultAdministratorRightsRequest represents a setMyDefaultAdministratorRights request.
type SetMyDefaultAdministratorRightsRequest struct {
	Rights      *tg.ChatAdministratorRights `json:"rights,omitempty"`
	ForChannels bool                        `json:"for_channels,omitempty"`
}

// GetMyDefaultAdministratorRightsRequest represents a getMyDefaultAdministratorRights request.
type GetMyDefaultAdministratorRightsRequest struct {
	ForChannels bool `json:"for_channels,omitempty"`
}

// ================== Bot Commands ==================

var commandRegex = regexp.MustCompile(`^[a-z0-9_]+$`)

// SetMyCommands sets the bot's command list for the specified scope and language.
// Commands appear in the menu button when users type "/".
func (c *Client) SetMyCommands(ctx context.Context, commands []tg.BotCommand, opts ...BotCommandOption) error {
	if len(commands) > 100 {
		return tg.NewValidationError("commands", "must have at most 100 commands")
	}
	for _, cmd := range commands {
		if len(cmd.Command) < 1 || len(cmd.Command) > 32 {
			return tg.NewValidationError("command", "must be 1-32 characters")
		}
		if !commandRegex.MatchString(cmd.Command) {
			return tg.NewValidationError("command", "must be lowercase a-z, 0-9, underscore only")
		}
		if len(cmd.Description) < 1 || len(cmd.Description) > 256 {
			return tg.NewValidationError("description", "must be 1-256 characters")
		}
	}

	req := SetMyCommandsRequest{Commands: commands}
	for _, opt := range opts {
		opt.applyToSetMyCommands(&req)
	}

	return c.callJSON(ctx, "setMyCommands", req, nil)
}

// GetMyCommands returns the bot's command list for the specified scope and language.
func (c *Client) GetMyCommands(ctx context.Context, opts ...BotCommandOption) ([]tg.BotCommand, error) {
	req := GetMyCommandsRequest{}
	for _, opt := range opts {
		opt.applyToGetMyCommands(&req)
	}

	var result []tg.BotCommand
	if err := c.callJSON(ctx, "getMyCommands", req, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteMyCommands removes the bot's command list for the specified scope and language.
func (c *Client) DeleteMyCommands(ctx context.Context, opts ...BotCommandOption) error {
	req := DeleteMyCommandsRequest{}
	for _, opt := range opts {
		opt.applyToDeleteMyCommands(&req)
	}

	return c.callJSON(ctx, "deleteMyCommands", req, nil)
}

// ================== Bot Profile ==================

// SetMyName sets the bot's name for the specified language.
// Pass empty string to remove the dedicated name for that language.
func (c *Client) SetMyName(ctx context.Context, name string, opts ...LanguageOption) error {
	if len(name) > 64 {
		return tg.NewValidationError("name", "must be at most 64 characters")
	}

	req := SetMyNameRequest{Name: name}
	for _, opt := range opts {
		opt.applyLanguage(&req.LanguageCode)
	}

	return c.callJSON(ctx, "setMyName", req, nil)
}

// GetMyName returns the bot's name for the specified language.
func (c *Client) GetMyName(ctx context.Context, opts ...LanguageOption) (*tg.BotName, error) {
	req := GetMyNameRequest{}
	for _, opt := range opts {
		opt.applyLanguage(&req.LanguageCode)
	}

	var result tg.BotName
	if err := c.callJSON(ctx, "getMyName", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetMyDescription sets the bot's description (shown in empty chat).
func (c *Client) SetMyDescription(ctx context.Context, description string, opts ...LanguageOption) error {
	if len(description) > 512 {
		return tg.NewValidationError("description", "must be at most 512 characters")
	}

	req := SetMyDescriptionRequest{Description: description}
	for _, opt := range opts {
		opt.applyLanguage(&req.LanguageCode)
	}

	return c.callJSON(ctx, "setMyDescription", req, nil)
}

// GetMyDescription returns the bot's description.
func (c *Client) GetMyDescription(ctx context.Context, opts ...LanguageOption) (*tg.BotDescription, error) {
	req := GetMyDescriptionRequest{}
	for _, opt := range opts {
		opt.applyLanguage(&req.LanguageCode)
	}

	var result tg.BotDescription
	if err := c.callJSON(ctx, "getMyDescription", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetMyShortDescription sets the bot's short description (shown in profile/search).
func (c *Client) SetMyShortDescription(ctx context.Context, shortDescription string, opts ...LanguageOption) error {
	if len(shortDescription) > 120 {
		return tg.NewValidationError("short_description", "must be at most 120 characters")
	}

	req := SetMyShortDescriptionRequest{ShortDescription: shortDescription}
	for _, opt := range opts {
		opt.applyLanguage(&req.LanguageCode)
	}

	return c.callJSON(ctx, "setMyShortDescription", req, nil)
}

// GetMyShortDescription returns the bot's short description.
func (c *Client) GetMyShortDescription(ctx context.Context, opts ...LanguageOption) (*tg.BotShortDescription, error) {
	req := GetMyShortDescriptionRequest{}
	for _, opt := range opts {
		opt.applyLanguage(&req.LanguageCode)
	}

	var result tg.BotShortDescription
	if err := c.callJSON(ctx, "getMyShortDescription", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SetMyProfilePhoto sets the bot's profile photo.
// Added in Bot API 9.4.
func (c *Client) SetMyProfilePhoto(ctx context.Context, photo InputFile, isPersonal bool) error {
	req := SetMyProfilePhotoRequest{
		Photo:      photo,
		IsPersonal: isPersonal,
	}
	return c.callJSON(ctx, "setMyProfilePhoto", req, nil)
}

// RemoveMyProfilePhoto removes the bot's profile photo.
// Added in Bot API 9.4.
func (c *Client) RemoveMyProfilePhoto(ctx context.Context, isPersonal bool) error {
	req := RemoveMyProfilePhotoRequest{IsPersonal: isPersonal}
	return c.callJSON(ctx, "removeMyProfilePhoto", req, nil)
}

// ================== Default Admin Rights ==================

// SetMyDefaultAdministratorRights sets the default admin rights requested when bot is added to groups/channels.
func (c *Client) SetMyDefaultAdministratorRights(ctx context.Context, opts ...AdminRightsOption) error {
	req := SetMyDefaultAdministratorRightsRequest{}
	for _, opt := range opts {
		opt.applyToAdminRights(&req)
	}

	return c.callJSON(ctx, "setMyDefaultAdministratorRights", req, nil)
}

// GetMyDefaultAdministratorRights returns the bot's default admin rights.
func (c *Client) GetMyDefaultAdministratorRights(ctx context.Context, forChannels bool) (*tg.ChatAdministratorRights, error) {
	req := GetMyDefaultAdministratorRightsRequest{ForChannels: forChannels}

	var result tg.ChatAdministratorRights
	if err := c.callJSON(ctx, "getMyDefaultAdministratorRights", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ================== Options ==================

// BotCommandOption configures bot command methods.
type BotCommandOption interface {
	applyToSetMyCommands(*SetMyCommandsRequest)
	applyToGetMyCommands(*GetMyCommandsRequest)
	applyToDeleteMyCommands(*DeleteMyCommandsRequest)
}

type botCommandOption struct {
	scope        *tg.BotCommandScope
	languageCode string
}

func (o botCommandOption) applyToSetMyCommands(r *SetMyCommandsRequest) {
	r.Scope = o.scope
	r.LanguageCode = o.languageCode
}

func (o botCommandOption) applyToGetMyCommands(r *GetMyCommandsRequest) {
	r.Scope = o.scope
	r.LanguageCode = o.languageCode
}

func (o botCommandOption) applyToDeleteMyCommands(r *DeleteMyCommandsRequest) {
	r.Scope = o.scope
	r.LanguageCode = o.languageCode
}

// WithCommandScope sets the scope for bot commands.
func WithCommandScope(scope tg.BotCommandScope) BotCommandOption {
	return botCommandOption{scope: &scope}
}

// WithCommandLanguage sets the IETF language tag for bot commands.
func WithCommandLanguage(code string) BotCommandOption {
	return botCommandOption{languageCode: code}
}

// WithCommandScopeAndLanguage sets both scope and language for bot commands.
func WithCommandScopeAndLanguage(scope tg.BotCommandScope, code string) BotCommandOption {
	return botCommandOption{scope: &scope, languageCode: code}
}

// LanguageOption sets the language for profile methods.
type LanguageOption interface {
	applyLanguage(*string)
}

type languageOption string

func (o languageOption) applyLanguage(s *string) { *s = string(o) }

// WithLanguage sets the IETF language tag (e.g., "en", "es", "ru").
func WithLanguage(code string) LanguageOption {
	return languageOption(code)
}

// AdminRightsOption configures admin rights methods.
type AdminRightsOption interface {
	applyToAdminRights(*SetMyDefaultAdministratorRightsRequest)
}

type adminRightsOption struct {
	rights      *tg.ChatAdministratorRights
	forChannels bool
}

func (o adminRightsOption) applyToAdminRights(r *SetMyDefaultAdministratorRightsRequest) {
	r.Rights = o.rights
	r.ForChannels = o.forChannels
}

// WithAdminRights sets the default administrator rights.
func WithAdminRights(rights tg.ChatAdministratorRights) AdminRightsOption {
	return adminRightsOption{rights: &rights}
}

// ForChannels applies the rights to channels instead of groups.
func ForChannels() AdminRightsOption {
	return adminRightsOption{forChannels: true}
}

// WithAdminRightsForChannels sets the default administrator rights for channels.
func WithAdminRightsForChannels(rights tg.ChatAdministratorRights) AdminRightsOption {
	return adminRightsOption{rights: &rights, forChannels: true}
}
