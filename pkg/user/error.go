package user

import "errors"

var (
	ErrUserNotFound          = errors.New("error user not found")
	ErrSessionNotFound       = errors.New("error session not found")
	ErrFriendRequestNotFound = errors.New("friend request not found")
	ErrCannotFriendSelf      = errors.New("cannot send a friend request to yourself")
	ErrFriendRequestExists   = errors.New("friend request already exists")
	ErrNotificationNotFound  = errors.New("notification not found")
	ErrNotificationDelivery  = errors.New("notification delivery failed")
	ErrAlreadyFriends        = errors.New("users are already friends")
	ErrBlocked               = errors.New("interaction is blocked")
	ErrSafetyUnavailable     = errors.New("safety service unavailable")
	ErrUsernameTaken         = errors.New("username is already taken")
	ErrHandleTaken           = errors.New("handle is already taken")
	ErrInvalidCredentials    = errors.New("invalid username or password")
	ErrInvalidUsername       = errors.New("username must be 3-30 characters using letters, numbers, or underscore")
	ErrInvalidPassword       = errors.New("password must be at least 8 characters")
	ErrInvalidDisplayName    = errors.New("display name must be 1-30 characters")
	ErrGoogleUserInvalid     = errors.New("google account information is invalid")
)
