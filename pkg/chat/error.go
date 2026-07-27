package chat

import "errors"

var (
	ErrUserNotFound             = errors.New("error user not found")
	ErrChannelOrUserNotFound    = errors.New("error channel or user not found")
	ErrDirectChatRequiresFriend = errors.New("direct chat requires an accepted friendship")
	ErrDirectChannelExists      = errors.New("direct channel already exists")
	ErrExceedMessageNumLimits   = errors.New("error exceed max number of messages")
	ErrMessageNotFound          = errors.New("error message not found")
	ErrNotMessageOwner          = errors.New("error not message owner")
	ErrMessageAlreadyDeleted    = errors.New("error message already deleted")
)
