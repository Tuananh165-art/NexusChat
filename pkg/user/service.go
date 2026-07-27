package user

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/realtime"
	"golang.org/x/crypto/bcrypt"
)

const oauthGoogleURLAPI = "https://www.googleapis.com/oauth2/v3/userinfo"

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,30}$`)

type UserService interface {
	GetGoogleUser(ctx context.Context, code string) (*GoogleUserPresenter, error)
	GetOrCreateUserByOAuth(ctx context.Context, user *User) (*User, error)
	CreateUser(ctx context.Context, user *User) (*User, error)
	Signup(ctx context.Context, username, password, displayName string) (*User, error)
	Login(ctx context.Context, username, password string) (*User, error)
	Logout(ctx context.Context, sid string) error
	UpdateUserProfile(ctx context.Context, uid uint64, name, picture string) (*User, error)
	SetUserSession(ctx context.Context, uid uint64) (string, error)
	GetUserByID(ctx context.Context, uid uint64) (*User, error)
	GetUserIDBySession(ctx context.Context, sid string) (uint64, error)
	SearchUsers(ctx context.Context, requesterID uint64, query string, limit int) ([]*User, error)
	SendFriendRequest(ctx context.Context, fromUserID, toUserID uint64) (*FriendRequest, error)
	IsFriend(ctx context.Context, userID, peerID uint64) (bool, error)
	GetRelationshipStatus(ctx context.Context, userID, peerID uint64) (string, error)
	GetFriendRequests(ctx context.Context, userID uint64) ([]*FriendRequest, error)
	AcceptFriendRequest(ctx context.Context, userID, fromUserID uint64) error
	DeclineFriendRequest(ctx context.Context, userID, fromUserID uint64) error
	CancelFriendRequest(ctx context.Context, userID, toUserID uint64) error
	ListFriends(ctx context.Context, userID uint64) ([]uint64, error)
	RemoveFriend(ctx context.Context, userID, friendID uint64) error
	GetNotifications(ctx context.Context, userID uint64) ([]*Notification, error)
	MarkNotificationRead(ctx context.Context, userID uint64, notificationID string) error
	MarkAllNotificationsRead(ctx context.Context, userID uint64) error
}

type UserServiceImpl struct {
	userRepo UserRepo
	sf       common.IDGenerator
}

func NewUserServiceImpl(userRepo UserRepo, sf common.IDGenerator) *UserServiceImpl {
	return &UserServiceImpl{userRepo, sf}
}

func NormalizeUsername(raw string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "@")))
	if !usernamePattern.MatchString(username) {
		return "", ErrInvalidUsername
	}
	return username, nil
}

func NormalizeHandle(raw string) (string, error) {
	handle := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "@")))
	if !usernamePattern.MatchString(handle) {
		return "", ErrInvalidUsername
	}
	return handle, nil
}

func ValidatePassword(password string) error {
	if len([]rune(password)) < 8 || len(password) > 256 {
		return ErrInvalidPassword
	}
	return nil
}

func (svc *UserServiceImpl) GetGoogleUser(ctx context.Context, accessToken string) (*GoogleUserPresenter, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, ErrGoogleUserInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oauthGoogleURLAPI, nil)
	if err != nil {
		return nil, fmt.Errorf("create google user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed getting google user info: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading google user response: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("google userinfo returned status %d", response.StatusCode)
	}
	var googleUser GoogleUserPresenter
	if err := json.Unmarshal(contents, &googleUser); err != nil {
		return nil, fmt.Errorf("failed decoding google user response: %w", err)
	}
	if googleUser.Subject == "" || googleUser.Email == "" || !googleUser.EmailVerified {
		return nil, ErrGoogleUserInvalid
	}
	return &googleUser, nil
}

func (svc *UserServiceImpl) CreateUser(ctx context.Context, user *User) (*User, error) {
	userID, err := svc.sf.NextID()
	if err != nil {
		return nil, fmt.Errorf("error create snowflake ID: %w", err)
	}
	newUser := &User{ID: userID, Email: strings.ToLower(strings.TrimSpace(user.Email)), Name: strings.TrimSpace(user.Name), Picture: user.Picture, AuthType: user.AuthType, Username: user.Username, Handle: user.Handle, PasswordHash: user.PasswordHash, GoogleSubject: user.GoogleSubject}
	if err = svc.userRepo.CreateUser(ctx, newUser); err != nil {
		return nil, fmt.Errorf("error create user %d: %w", userID, err)
	}
	return newUser, nil
}

func (svc *UserServiceImpl) Signup(ctx context.Context, rawUsername, password, displayName string) (*User, error) {
	username, err := NormalizeUsername(rawUsername)
	if err != nil {
		return nil, err
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = username
	}
	if len([]rune(name)) > 30 {
		return nil, ErrInvalidDisplayName
	}
	userID, err := svc.sf.NextID()
	if err != nil {
		return nil, fmt.Errorf("error create snowflake ID: %w", err)
	}
	for attempt := 0; attempt < 100; attempt++ {
		handle := generatedHandle(username, userID, attempt)
		candidate := &User{ID: userID, Name: name, Username: username, Handle: handle, PasswordHash: string(passwordHash), AuthType: LocalAuth}
		err = svc.userRepo.CreateUser(ctx, candidate)
		if err == nil {
			return candidate, nil
		}
		if errors.Is(err, ErrHandleTaken) {
			continue
		}
		return nil, err
	}
	return nil, ErrHandleTaken
}

func generatedHandle(base string, id uint64, attempt int) string {
	base = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(base), "@"))
	if len(base) > 24 {
		base = base[:24]
	}
	suffix := fmt.Sprintf("%03d", id%1000)
	if attempt > 0 {
		suffix = strconv.Itoa(attempt) + suffix
	}
	maxBase := 30 - len(suffix)
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + suffix
}

func (svc *UserServiceImpl) Login(ctx context.Context, rawUsername, password string) (*User, error) {
	username, err := NormalizeUsername(rawUsername)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	user, err := svc.userRepo.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if user.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

func (svc *UserServiceImpl) Logout(ctx context.Context, sid string) error {
	if strings.TrimSpace(sid) == "" {
		return nil
	}
	return svc.userRepo.DeleteUserSession(ctx, sid)
}

func (svc *UserServiceImpl) UpdateUserProfile(ctx context.Context, uid uint64, name, picture string) (*User, error) {
	user, err := svc.userRepo.GetUserByID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("error get user %d: %w", uid, err)
	}
	user.Name, user.Picture = name, picture
	if err := svc.userRepo.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("error update user %d: %w", uid, err)
	}
	return user, nil
}

func (svc *UserServiceImpl) SetUserSession(ctx context.Context, uid uint64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("error create sid: %w", err)
	}
	sid := base64.URLEncoding.EncodeToString(b)
	if err := svc.userRepo.SetUserSession(ctx, uid, sid); err != nil {
		return "", fmt.Errorf("error set sid for user %d: %w", uid, err)
	}
	return sid, nil
}

func (svc *UserServiceImpl) GetUserByID(ctx context.Context, uid uint64) (*User, error) {
	user, err := svc.userRepo.GetUserByID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("error get user %d: %w", uid, err)
	}
	return user, nil
}

func (svc *UserServiceImpl) GetUserIDBySession(ctx context.Context, sid string) (uint64, error) {
	userID, err := svc.userRepo.GetUserIDBySession(ctx, sid)
	if err != nil {
		return 0, fmt.Errorf("error get user id by sid: %w", err)
	}
	return userID, nil
}

func (svc *UserServiceImpl) SearchUsers(ctx context.Context, requesterID uint64, query string, limit int) ([]*User, error) {
	users, err := svc.userRepo.SearchUsers(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	result := make([]*User, 0, len(users))
	for _, candidate := range users {
		if candidate.ID != requesterID {
			result = append(result, candidate)
		}
	}
	return result, nil
}

func (svc *UserServiceImpl) SendFriendRequest(ctx context.Context, fromUserID, toUserID uint64) (*FriendRequest, error) {
	if fromUserID == 0 || fromUserID == toUserID {
		return nil, ErrCannotFriendSelf
	}
	if _, err := svc.userRepo.GetUserByID(ctx, fromUserID); err != nil {
		return nil, err
	}
	if _, err := svc.userRepo.GetUserByID(ctx, toUserID); err != nil {
		return nil, err
	}
	if endpoint := os.Getenv("USER_GRPC_CLIENT_SAFETY_ENDPOINT"); endpoint != "" {
		decision, safetyErr := realtime.CallStructRPC(ctx, endpoint, "user", "nexuschat.safety.v1.SafetyService", "IsUserBlocked", map[string]any{
			"user_id": strconv.FormatUint(fromUserID, 10), "peer_id": strconv.FormatUint(toUserID, 10),
		})
		if safetyErr != nil {
			return nil, ErrSafetyUnavailable
		}
		if decision != nil && decision.GetFields()["blocked"].GetBoolValue() {
			return nil, ErrBlocked
		}
	}
	friends, err := svc.userRepo.IsFriend(ctx, fromUserID, toUserID)
	if err != nil {
		return nil, fmt.Errorf("check friendship: %w", err)
	}
	if friends {
		return nil, ErrAlreadyFriends
	}
	incoming, err := svc.userRepo.GetFriendRequests(ctx, fromUserID)
	if err != nil {
		return nil, fmt.Errorf("check reverse friend request: %w", err)
	}
	for _, existing := range incoming {
		if existing.FromUserID == toUserID && existing.ToUserID == fromUserID {
			return nil, ErrFriendRequestExists
		}
	}
	request := &FriendRequest{FromUserID: fromUserID, ToUserID: toUserID, Status: "pending", CreatedAt: time.Now().UnixMilli()}
	if err := svc.userRepo.CreateFriendRequest(ctx, request); err != nil {
		if errors.Is(err, ErrFriendRequestExists) {
			// A previous attempt may have persisted the request but failed while
			// writing its notification. Retry the deterministic notification.
			if existingRequests, lookupErr := svc.userRepo.GetFriendRequests(ctx, toUserID); lookupErr == nil {
				for _, existing := range existingRequests {
					if existing.FromUserID == fromUserID {
						if notifyErr := svc.createFriendRequestNotification(ctx, existing); notifyErr != nil {
							return nil, notifyErr
						}
						break
					}
				}
			}
		}
		return nil, fmt.Errorf("create friend request: %w", err)
	}
	if err := svc.createFriendRequestNotification(ctx, request); err != nil {
		return nil, err
	}
	return request, nil
}

func (svc *UserServiceImpl) createFriendRequestNotification(ctx context.Context, request *FriendRequest) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("%w: marshal friend request notification: %v", ErrNotificationDelivery, err)
	}
	notification := &Notification{
		ID:        fmt.Sprintf("friend-request-%d-%d", request.FromUserID, request.CreatedAt),
		Type:      "friend_request",
		ActorID:   request.FromUserID,
		Payload:   string(payload),
		CreatedAt: request.CreatedAt,
	}
	if err := svc.userRepo.CreateNotification(ctx, request.ToUserID, notification); err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationDelivery, err)
	}
	return nil
}

func (svc *UserServiceImpl) IsFriend(ctx context.Context, userID, peerID uint64) (bool, error) {
	return svc.userRepo.IsFriend(ctx, userID, peerID)
}

func (svc *UserServiceImpl) GetRelationshipStatus(ctx context.Context, userID, peerID uint64) (string, error) {
	friends, err := svc.userRepo.IsFriend(ctx, userID, peerID)
	if err != nil {
		return "", err
	}
	if friends {
		return "accepted", nil
	}
	outgoing, err := svc.userRepo.GetFriendRequests(ctx, peerID)
	if err != nil {
		return "", err
	}
	for _, request := range outgoing {
		if request.FromUserID == userID && request.ToUserID == peerID {
			return "pending_outgoing", nil
		}
	}
	incoming, err := svc.userRepo.GetFriendRequests(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, request := range incoming {
		if request.FromUserID == peerID && request.ToUserID == userID {
			return "pending_incoming", nil
		}
	}
	declined, err := svc.userRepo.WasFriendRequestDeclined(ctx, userID, peerID)
	if err != nil {
		return "", err
	}
	if declined {
		return "declined", nil
	}
	return "none", nil
}

func (svc *UserServiceImpl) GetFriendRequests(ctx context.Context, userID uint64) ([]*FriendRequest, error) {
	return svc.userRepo.GetFriendRequests(ctx, userID)
}
func (svc *UserServiceImpl) AcceptFriendRequest(ctx context.Context, userID, fromUserID uint64) error {
	if err := svc.userRepo.AcceptFriendRequest(ctx, userID, fromUserID); err != nil {
		return err
	}
	notification := &Notification{
		ID:        fmt.Sprintf("friend-accepted-%d-%d", userID, fromUserID),
		Type:      "friend_request_accepted",
		ActorID:   userID,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := svc.userRepo.CreateNotification(ctx, fromUserID, notification); err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationDelivery, err)
	}
	return nil
}
func (svc *UserServiceImpl) DeclineFriendRequest(ctx context.Context, userID, fromUserID uint64) error {
	if err := svc.userRepo.DeclineFriendRequest(ctx, userID, fromUserID); err != nil {
		return err
	}
	notification := &Notification{
		ID:        fmt.Sprintf("friend-declined-%d-%d", userID, fromUserID),
		Type:      "friend_request_declined",
		ActorID:   userID,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := svc.userRepo.CreateNotification(ctx, fromUserID, notification); err != nil {
		return fmt.Errorf("%w: %v", ErrNotificationDelivery, err)
	}
	return nil
}
func (svc *UserServiceImpl) CancelFriendRequest(ctx context.Context, userID, toUserID uint64) error {
	return svc.userRepo.CancelFriendRequest(ctx, userID, toUserID)
}
func (svc *UserServiceImpl) ListFriends(ctx context.Context, userID uint64) ([]uint64, error) {
	return svc.userRepo.ListFriends(ctx, userID)
}
func (svc *UserServiceImpl) RemoveFriend(ctx context.Context, userID, friendID uint64) error {
	return svc.userRepo.RemoveFriend(ctx, userID, friendID)
}
func (svc *UserServiceImpl) GetNotifications(ctx context.Context, userID uint64) ([]*Notification, error) {
	return svc.userRepo.GetNotifications(ctx, userID)
}
func (svc *UserServiceImpl) MarkNotificationRead(ctx context.Context, userID uint64, notificationID string) error {
	return svc.userRepo.MarkNotificationRead(ctx, userID, notificationID)
}
func (svc *UserServiceImpl) MarkAllNotificationsRead(ctx context.Context, userID uint64) error {
	return svc.userRepo.MarkAllNotificationsRead(ctx, userID)
}

func (svc *UserServiceImpl) GetOrCreateUserByOAuth(ctx context.Context, incoming *User) (*User, error) {
	if incoming.GoogleSubject != "" {
		if existing, err := svc.userRepo.GetUserByGoogleSubject(ctx, incoming.GoogleSubject); err == nil {
			return existing, nil
		} else if !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
	}
	if incoming.Email != "" {
		if existing, err := svc.userRepo.GetUserByOAuthEmail(ctx, incoming.AuthType, incoming.Email); err == nil {
			if existing.GoogleSubject == "" && incoming.GoogleSubject != "" {
				existing.GoogleSubject = incoming.GoogleSubject
				_ = svc.userRepo.UpdateUser(ctx, existing)
			}
			return existing, nil
		} else if !errors.Is(err, ErrUserNotFound) {
			return nil, err
		}
	}
	base := incoming.Name
	if base == "" {
		base = strings.Split(incoming.Email, "@")[0]
	}
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, base)
	base = strings.ToLower(base)
	if len(base) < 3 {
		base = "user"
	}
	name := strings.TrimSpace(incoming.Name)
	if name == "" {
		name = base
	}
	for attempt := 0; attempt < 100; attempt++ {
		username := base
		if attempt > 0 {
			username = generatedHandle(base, uint64(attempt), 0)
		}
		if len(username) > 30 {
			username = username[:30]
		}
		id, err := svc.sf.NextID()
		if err != nil {
			return nil, err
		}
		candidate := &User{ID: id, Email: incoming.Email, Name: name, Picture: incoming.Picture, AuthType: GoogleAuth, Username: username, Handle: generatedHandle(username, id, attempt), GoogleSubject: incoming.GoogleSubject}
		if err := svc.userRepo.CreateUser(ctx, candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, ErrUsernameTaken) && !errors.Is(err, ErrHandleTaken) {
			return nil, err
		}
	}
	return nil, ErrUsernameTaken
}
