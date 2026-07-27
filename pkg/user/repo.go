package user

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/infra"
	"github.com/Tuananh165-art/NexusChat/pkg/notification"
	"github.com/gocql/gocql"
)

var (
	userPrefix              = "rc:user"
	sessionPrefix           = "rc:session"
	userNameIndex           = "rc:user:names"
	userHandleIndex         = "rc:user:handles"
	usernamePrefix          = "rc:user:username"
	handlePrefix            = "rc:user:handle"
	googleSubjectPrefix     = "rc:user:google:sub"
	friendRequestPrefix     = "rc:friend:request"
	friendshipPrefix        = "rc:friendship"
	friendDeclinedPrefix    = "rc:friend:declined"
	notificationPrefix      = "rc:notifications"
	notificationIndexPrefix = "rc:notifications:index"
)

type UserRepo interface {
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, userID uint64) (*User, error)
	GetUserByUsername(ctx context.Context, username string) (*User, error)
	GetUserByHandle(ctx context.Context, handle string) (*User, error)
	GetUserByGoogleSubject(ctx context.Context, subject string) (*User, error)
	GetUserByOAuthEmail(ctx context.Context, authType AuthType, email string) (*User, error)
	SearchUsers(ctx context.Context, query string, limit int) ([]*User, error)
	SetUserSession(ctx context.Context, uid uint64, sid string) error
	GetUserIDBySession(ctx context.Context, sid string) (uint64, error)
	DeleteUserSession(ctx context.Context, sid string) error
	CreateFriendRequest(ctx context.Context, request *FriendRequest) error
	IsFriend(ctx context.Context, userID, peerID uint64) (bool, error)
	GetFriendRequests(ctx context.Context, userID uint64) ([]*FriendRequest, error)
	AcceptFriendRequest(ctx context.Context, userID, fromUserID uint64) error
	DeclineFriendRequest(ctx context.Context, userID, fromUserID uint64) error
	CancelFriendRequest(ctx context.Context, userID, toUserID uint64) error
	ListFriends(ctx context.Context, userID uint64) ([]uint64, error)
	RemoveFriend(ctx context.Context, userID, friendID uint64) error
	WasFriendRequestDeclined(ctx context.Context, fromUserID, toUserID uint64) (bool, error)
	CreateNotification(ctx context.Context, userID uint64, notification *Notification) error
	GetNotifications(ctx context.Context, userID uint64) ([]*Notification, error)
	MarkNotificationRead(ctx context.Context, userID uint64, notificationID string) error
	MarkAllNotificationsRead(ctx context.Context, userID uint64) error
}

type UserRepoImpl struct {
	r         infra.RedisCache
	cassandra *gocql.Session
	outbox    notification.Enqueuer
}

func NewUserRepoImpl(r infra.RedisCache, cassandra *gocql.Session, outbox ...notification.Enqueuer) *UserRepoImpl {
	var enqueue notification.Enqueuer
	if len(outbox) > 0 {
		enqueue = outbox[0]
	}
	return &UserRepoImpl{r: r, cassandra: cassandra, outbox: enqueue}
}

// NewUserRepoWithOutbox is the production constructor; NewUserRepoImpl keeps
// the historical two-argument form available to unit tests and callers.
func NewUserRepoWithOutbox(r infra.RedisCache, cassandra *gocql.Session, outbox notification.Enqueuer) *UserRepoImpl {
	return &UserRepoImpl{r: r, cassandra: cassandra, outbox: outbox}
}

func (repo *UserRepoImpl) CreateUser(ctx context.Context, user *User) error {
	claimedUsername := false
	if user.Username != "" {
		claimed, err := repo.claimUnique(ctx, usernameKey(user.Username), user.ID, ErrUsernameTaken)
		if err != nil {
			return err
		}
		claimedUsername = claimed
	}
	if user.Handle != "" {
		claimed, err := repo.claimUnique(ctx, handleKey(user.Handle), user.ID, ErrHandleTaken)
		if err != nil {
			if claimedUsername {
				_ = repo.r.Delete(ctx, usernameKey(user.Username))
			}
			return err
		}
		if claimed {
			claimedUsername = true
		}
	}
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	if err = repo.r.SetPersistent(ctx, constructKey(userPrefix, user.ID), data); err != nil {
		if claimedUsername && user.Username != "" {
			_ = repo.r.Delete(ctx, usernameKey(user.Username))
		}
		if user.Handle != "" {
			_ = repo.r.Delete(ctx, handleKey(user.Handle))
		}
		return err
	}
	if user.AuthType != LocalAuth && user.Email != "" {
		if err = repo.r.SetPersistent(ctx, constructOAuthKey(user.AuthType, user.Email), data); err != nil {
			return err
		}
	}
	if user.GoogleSubject != "" {
		if err = repo.r.SetPersistent(ctx, googleSubjectKey(user.GoogleSubject), user.ID); err != nil {
			return err
		}
	}
	return repo.indexUser(ctx, user)
}

func (repo *UserRepoImpl) UpdateUser(ctx context.Context, user *User) error {
	if user.Username != "" {
		if _, err := repo.claimUnique(ctx, usernameKey(user.Username), user.ID, ErrUsernameTaken); err != nil {
			return err
		}
	}
	if user.Handle != "" {
		if _, err := repo.claimUnique(ctx, handleKey(user.Handle), user.ID, ErrHandleTaken); err != nil {
			return err
		}
	}
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}
	if err = repo.r.SetPersistent(ctx, constructKey(userPrefix, user.ID), data); err != nil {
		return err
	}
	if user.AuthType != LocalAuth && user.Email != "" {
		if err = repo.r.SetPersistent(ctx, constructOAuthKey(user.AuthType, user.Email), data); err != nil {
			return err
		}
	}
	if user.GoogleSubject != "" {
		if err = repo.r.SetPersistent(ctx, googleSubjectKey(user.GoogleSubject), user.ID); err != nil {
			return err
		}
	}
	return repo.indexUser(ctx, user)
}

func (repo *UserRepoImpl) GetUserByID(ctx context.Context, userID uint64) (*User, error) {
	var user User
	exist, err := repo.r.Get(ctx, constructKey(userPrefix, userID), &user)
	if err != nil {
		return nil, err
	}
	if !exist {
		return nil, ErrUserNotFound
	}
	_ = repo.indexUser(ctx, &user)
	return &user, nil
}

func (repo *UserRepoImpl) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return repo.getUserByClaim(ctx, usernameKey(username))
}

func (repo *UserRepoImpl) GetUserByHandle(ctx context.Context, handle string) (*User, error) {
	return repo.getUserByClaim(ctx, handleKey(handle))
}

func (repo *UserRepoImpl) GetUserByGoogleSubject(ctx context.Context, subject string) (*User, error) {
	return repo.getUserByClaim(ctx, googleSubjectKey(subject))
}

func (repo *UserRepoImpl) getUserByClaim(ctx context.Context, key string) (*User, error) {
	var userID uint64
	exist, err := repo.r.Get(ctx, key, &userID)
	if err != nil {
		return nil, err
	}
	if !exist || userID == 0 {
		return nil, ErrUserNotFound
	}
	return repo.GetUserByID(ctx, userID)
}

func (repo *UserRepoImpl) GetUserByOAuthEmail(ctx context.Context, authType AuthType, email string) (*User, error) {
	var user User
	exist, err := repo.r.Get(ctx, constructOAuthKey(authType, strings.ToLower(strings.TrimSpace(email))), &user)
	if err != nil {
		return nil, err
	}
	if !exist {
		return nil, ErrUserNotFound
	}
	return &user, nil
}

func (repo *UserRepoImpl) SetUserSession(ctx context.Context, uid uint64, sid string) error {
	return repo.r.Set(ctx, common.Join(sessionPrefix, ":", sid), uid)
}

func (repo *UserRepoImpl) GetUserIDBySession(ctx context.Context, sid string) (uint64, error) {
	var userID uint64
	exist, err := repo.r.Get(ctx, common.Join(sessionPrefix, ":", sid), &userID)
	if err != nil {
		return 0, err
	}
	if !exist {
		return 0, ErrSessionNotFound
	}
	return userID, nil
}

func (repo *UserRepoImpl) DeleteUserSession(ctx context.Context, sid string) error {
	return repo.r.Delete(ctx, common.Join(sessionPrefix, ":", sid))
}

func constructKey(prefix string, id uint64) string {
	return common.Join(prefix, ":", strconv.FormatUint(id, 10))
}
func constructOAuthKey(authType AuthType, email string) string {
	return common.Join(userPrefix, ":", string(authType), ":", strings.ToLower(strings.TrimSpace(email)))
}
func usernameKey(username string) string {
	return common.Join(usernamePrefix, ":", strings.ToLower(strings.TrimSpace(username)))
}
func handleKey(handle string) string {
	return common.Join(handlePrefix, ":", strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@")))
}
func googleSubjectKey(subject string) string { return common.Join(googleSubjectPrefix, ":", subject) }

func (repo *UserRepoImpl) claimUnique(ctx context.Context, key string, userID uint64, taken error) (bool, error) {
	claimed, err := repo.r.SetNX(ctx, key, userID)
	if err != nil {
		return false, err
	}
	if claimed {
		return true, nil
	}
	var existing uint64
	exists, err := repo.r.Get(ctx, key, &existing)
	if err != nil {
		return false, err
	}
	if exists && existing == userID {
		return false, nil
	}
	return false, taken
}

func (repo *UserRepoImpl) indexUser(ctx context.Context, user *User) error {
	id := strconv.FormatUint(user.ID, 10)
	if name := strings.ToLower(strings.TrimSpace(user.Name)); name != "" {
		if err := repo.r.ZAdd(ctx, userNameIndex, 0, name+"\x00"+id); err != nil {
			return err
		}
	}
	if handle := strings.ToLower(strings.TrimSpace(user.Handle)); handle != "" {
		if err := repo.r.ZAdd(ctx, userHandleIndex, 0, handle+"\x00"+id); err != nil {
			return err
		}
	}
	return nil
}

func (repo *UserRepoImpl) SearchUsers(ctx context.Context, query string, limit int) ([]*User, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	query = strings.TrimPrefix(query, "@")
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if user, err := repo.GetUserByHandle(ctx, query); err == nil {
		return []*User{user}, nil
	}
	if id, err := strconv.ParseUint(query, 10, 64); err == nil && id > 0 {
		user, err := repo.GetUserByID(ctx, id)
		if err != nil {
			return nil, nil
		}
		return []*User{user}, nil
	}
	result := make([]*User, 0, limit)
	seen := make(map[uint64]struct{})
	for _, index := range []string{userHandleIndex, userNameIndex} {
		members, err := repo.r.ZRangeByLex(ctx, index, "["+query, "["+query+"\xff", limit)
		if err != nil {
			return nil, err
		}
		for _, member := range members {
			parts := strings.SplitN(member, "\x00", 2)
			if len(parts) != 2 {
				continue
			}
			id, err := strconv.ParseUint(parts[1], 10, 64)
			if err != nil {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			user, err := repo.GetUserByID(ctx, id)
			if err != nil {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, user)
			if len(result) >= limit {
				return result, nil
			}
		}
	}
	return result, nil
}

func (repo *UserRepoImpl) CreateFriendRequest(ctx context.Context, request *FriendRequest) error {
	if repo.cassandra != nil {
		createdAt := time.UnixMilli(request.CreatedAt).UTC()
		applied, err := repo.cassandra.Query("INSERT INTO friend_requests_by_recipient (recipient_id, sender_id, status, created_at, updated_at) VALUES (?, ?, 'pending', ?, ?) IF NOT EXISTS", request.ToUserID, request.FromUserID, createdAt, createdAt).WithContext(ctx).MapScanCAS(map[string]interface{}{})
		if err != nil {
			return err
		}
		if !applied {
			var status string
			if err := repo.cassandra.Query("SELECT status FROM friend_requests_by_recipient WHERE recipient_id = ? AND sender_id = ?", request.ToUserID, request.FromUserID).WithContext(ctx).Scan(&status); err != nil {
				return err
			}
			if status != "declined" {
				return ErrFriendRequestExists
			}
			applied, err = repo.cassandra.Query("UPDATE friend_requests_by_recipient SET status = 'pending', created_at = ?, updated_at = ? WHERE recipient_id = ? AND sender_id = ? IF status = 'declined'", createdAt, createdAt, request.ToUserID, request.FromUserID).WithContext(ctx).MapScanCAS(map[string]interface{}{})
			if err != nil {
				return err
			}
			if !applied {
				return ErrFriendRequestExists
			}
		}
		if data, marshalErr := json.Marshal(request); marshalErr == nil {
			// Cassandra is canonical; Redis is only a compatibility mirror.
			_ = repo.r.HDel(ctx, friendDeclinedKey(request.FromUserID, request.ToUserID), "status")
			_ = repo.r.HSet(ctx, friendRequestKey("from", request.FromUserID), strconv.FormatUint(request.ToUserID, 10), string(data))
			_ = repo.r.HSet(ctx, friendRequestKey("to", request.ToUserID), strconv.FormatUint(request.FromUserID, 10), string(data))
		}
		return nil
	}
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	fromKey := friendRequestKey("from", request.FromUserID)
	toKey := friendRequestKey("to", request.ToUserID)
	value := string(data)
	var existing FriendRequest
	exists, err := repo.r.HGet(ctx, toKey, strconv.FormatUint(request.FromUserID, 10), &existing)
	if err != nil {
		return err
	}
	if exists && existing.Status == "pending" {
		return ErrFriendRequestExists
	}
	if err := repo.r.HDel(ctx, friendDeclinedKey(request.FromUserID, request.ToUserID), "status"); err != nil {
		return err
	}
	if err := repo.r.HSet(ctx, fromKey, strconv.FormatUint(request.ToUserID, 10), value); err != nil {
		return err
	}
	return repo.r.HSet(ctx, toKey, strconv.FormatUint(request.FromUserID, 10), value)
}

func (repo *UserRepoImpl) IsFriend(ctx context.Context, userID, peerID uint64) (bool, error) {
	if repo.cassandra != nil {
		var found uint64
		err := repo.cassandra.Query("SELECT friend_id FROM friendships_by_user WHERE user_id = ? AND friend_id = ? LIMIT 1", userID, peerID).WithContext(ctx).Scan(&found)
		if err == nil {
			return found == peerID, nil
		}
		if err != gocql.ErrNotFound {
			return false, err
		}
		// Legacy friendships may predate the Cassandra projection. Keep Redis
		// as a compatibility fallback until the migration backfill completes.
		friends, redisErr := repo.r.HGetAll(ctx, friendshipKey(userID))
		if redisErr != nil {
			return false, redisErr
		}
		_, exists := friends[strconv.FormatUint(peerID, 10)]
		return exists, nil
	}
	friends, err := repo.r.HGetAll(ctx, friendshipKey(userID))
	if err != nil {
		return false, err
	}
	_, exists := friends[strconv.FormatUint(peerID, 10)]
	return exists, nil
}

func (repo *UserRepoImpl) GetFriendRequests(ctx context.Context, userID uint64) ([]*FriendRequest, error) {
	bySender := make(map[uint64]*FriendRequest)
	if repo.cassandra != nil {
		iter := repo.cassandra.Query("SELECT sender_id, status, created_at FROM friend_requests_by_recipient WHERE recipient_id = ?", userID).WithContext(ctx).Iter()
		var senderID uint64
		var status string
		var createdAt time.Time
		for iter.Scan(&senderID, &status, &createdAt) {
			if status == "pending" {
				bySender[senderID] = &FriendRequest{FromUserID: senderID, ToUserID: userID, Status: status, CreatedAt: createdAt.UnixMilli()}
			}
		}
		if err := iter.Close(); err != nil {
			return nil, err
		}
	}
	values, err := repo.r.HGetAll(ctx, friendRequestKey("to", userID))
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		var request FriendRequest
		if json.Unmarshal([]byte(value), &request) == nil && request.Status == "pending" {
			if _, exists := bySender[request.FromUserID]; !exists {
				copy := request
				bySender[request.FromUserID] = &copy
			}
		}
	}
	requests := make([]*FriendRequest, 0, len(bySender))
	for _, request := range bySender {
		requests = append(requests, request)
	}
	sort.SliceStable(requests, func(i, j int) bool { return requests[i].CreatedAt > requests[j].CreatedAt })
	return requests, nil
}

func (repo *UserRepoImpl) AcceptFriendRequest(ctx context.Context, userID, fromUserID uint64) error {
	if repo.cassandra != nil {
		request, found, err := repo.getDurableFriendRequest(ctx, userID, fromUserID)
		if err != nil {
			return err
		}
		if found {
			switch request.Status {
			case "pending":
				applied, err := repo.cassandra.Query("UPDATE friend_requests_by_recipient SET status = 'accepted', updated_at = ? WHERE recipient_id = ? AND sender_id = ? IF status = 'pending'", time.Now().UTC(), userID, fromUserID).WithContext(ctx).MapScanCAS(map[string]interface{}{})
				if err != nil {
					return err
				}
				if !applied {
					latest, latestFound, readErr := repo.getDurableFriendRequest(ctx, userID, fromUserID)
					if readErr != nil || !latestFound || latest.Status != "accepted" {
						if readErr != nil {
							return readErr
						}
						return ErrFriendRequestNotFound
					}
				}
			case "accepted":
			default:
				return ErrFriendRequestNotFound
			}
			createdAt := time.Now().UTC()
			if err := repo.persistFriendship(ctx, userID, fromUserID, createdAt); err != nil {
				return err
			}
			if err := repo.persistFriendship(ctx, fromUserID, userID, createdAt); err != nil {
				return err
			}
			_ = repo.r.HDel(ctx, friendRequestKey("to", userID), strconv.FormatUint(fromUserID, 10))
			_ = repo.r.HDel(ctx, friendRequestKey("from", fromUserID), strconv.FormatUint(userID, 10))
			_ = repo.r.HSet(ctx, friendshipKey(userID), strconv.FormatUint(fromUserID, 10), createdAt.Format(time.RFC3339))
			_ = repo.r.HSet(ctx, friendshipKey(fromUserID), strconv.FormatUint(userID, 10), createdAt.Format(time.RFC3339))
			return nil
		}
	}
	request, err := repo.getFriendRequest(ctx, userID, fromUserID)
	if err != nil {
		return err
	}
	request.Status = "accepted"
	if err := repo.r.HDel(ctx, friendRequestKey("to", userID), strconv.FormatUint(fromUserID, 10)); err != nil {
		return err
	}
	if err := repo.r.HDel(ctx, friendRequestKey("from", fromUserID), strconv.FormatUint(userID, 10)); err != nil {
		return err
	}
	createdAt := time.Now().UTC()
	if err := repo.r.HSet(ctx, friendshipKey(userID), strconv.FormatUint(fromUserID, 10), createdAt.Format(time.RFC3339)); err != nil {
		return err
	}
	if err := repo.r.HSet(ctx, friendshipKey(fromUserID), strconv.FormatUint(userID, 10), createdAt.Format(time.RFC3339)); err != nil {
		return err
	}
	if err := repo.persistFriendship(ctx, userID, fromUserID, createdAt); err != nil {
		return err
	}
	return repo.persistFriendship(ctx, fromUserID, userID, createdAt)
}

func (repo *UserRepoImpl) DeclineFriendRequest(ctx context.Context, userID, fromUserID uint64) error {
	if repo.cassandra != nil {
		request, found, err := repo.getDurableFriendRequest(ctx, userID, fromUserID)
		if err != nil {
			return err
		}
		if found {
			switch request.Status {
			case "pending":
				applied, err := repo.cassandra.Query("UPDATE friend_requests_by_recipient SET status = 'declined', updated_at = ? WHERE recipient_id = ? AND sender_id = ? IF status = 'pending'", time.Now().UTC(), userID, fromUserID).WithContext(ctx).MapScanCAS(map[string]interface{}{})
				if err != nil {
					return err
				}
				if !applied {
					return ErrFriendRequestNotFound
				}
			case "declined":
			default:
				return ErrFriendRequestNotFound
			}
			_ = repo.r.HDel(ctx, friendRequestKey("to", userID), strconv.FormatUint(fromUserID, 10))
			_ = repo.r.HDel(ctx, friendRequestKey("from", fromUserID), strconv.FormatUint(userID, 10))
			_ = repo.r.HSet(ctx, friendDeclinedKey(fromUserID, userID), "status", time.Now().UTC().Format(time.RFC3339))
			return nil
		}
	}
	if _, err := repo.getFriendRequest(ctx, userID, fromUserID); err != nil {
		return err
	}
	if err := repo.r.HDel(ctx, friendRequestKey("to", userID), strconv.FormatUint(fromUserID, 10)); err != nil {
		return err
	}
	if err := repo.r.HDel(ctx, friendRequestKey("from", fromUserID), strconv.FormatUint(userID, 10)); err != nil {
		return err
	}
	return repo.r.HSet(ctx, friendDeclinedKey(fromUserID, userID), "status", time.Now().UTC().Format(time.RFC3339))
}

func (repo *UserRepoImpl) getFriendRequest(ctx context.Context, userID, fromUserID uint64) (*FriendRequest, error) {
	if repo.cassandra != nil {
		request, found, err := repo.getDurableFriendRequest(ctx, userID, fromUserID)
		if err != nil {
			return nil, err
		}
		if found {
			if request.Status != "pending" {
				return nil, ErrFriendRequestNotFound
			}
			return request, nil
		}
	}
	var request FriendRequest
	exists, err := repo.r.HGet(ctx, friendRequestKey("to", userID), strconv.FormatUint(fromUserID, 10), &request)
	if err != nil {
		return nil, err
	}
	if !exists || request.Status != "pending" {
		return nil, ErrFriendRequestNotFound
	}
	return &request, nil
}

func (repo *UserRepoImpl) getDurableFriendRequest(ctx context.Context, recipientID, senderID uint64) (*FriendRequest, bool, error) {
	if repo.cassandra == nil {
		return nil, false, nil
	}
	var status string
	var createdAt, updatedAt time.Time
	err := repo.cassandra.Query("SELECT status, created_at, updated_at FROM friend_requests_by_recipient WHERE recipient_id = ? AND sender_id = ?", recipientID, senderID).WithContext(ctx).Scan(&status, &createdAt, &updatedAt)
	if err == gocql.ErrNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &FriendRequest{FromUserID: senderID, ToUserID: recipientID, Status: status, CreatedAt: createdAt.UnixMilli()}, true, nil
}

func friendRequestKey(direction string, userID uint64) string {
	return common.Join(friendRequestPrefix, ":", direction, ":", strconv.FormatUint(userID, 10))
}

func friendshipKey(userID uint64) string {
	return common.Join(friendshipPrefix, ":", strconv.FormatUint(userID, 10))
}

func (repo *UserRepoImpl) CancelFriendRequest(ctx context.Context, userID, toUserID uint64) error {
	if repo.cassandra != nil {
		request, found, err := repo.getDurableFriendRequest(ctx, toUserID, userID)
		if err != nil {
			return err
		}
		if found {
			if request.Status != "pending" {
				return ErrFriendRequestNotFound
			}
			applied, err := repo.cassandra.Query("DELETE FROM friend_requests_by_recipient WHERE recipient_id = ? AND sender_id = ? IF status = 'pending'", toUserID, userID).WithContext(ctx).MapScanCAS(map[string]interface{}{})
			if err != nil {
				return err
			}
			if !applied {
				return ErrFriendRequestNotFound
			}
			_ = repo.r.HDel(ctx, friendRequestKey("to", toUserID), strconv.FormatUint(userID, 10))
			_ = repo.r.HDel(ctx, friendRequestKey("from", userID), strconv.FormatUint(toUserID, 10))
			return nil
		}
	}
	if _, err := repo.getFriendRequest(ctx, toUserID, userID); err != nil {
		return err
	}
	if err := repo.r.HDel(ctx, friendRequestKey("to", toUserID), strconv.FormatUint(userID, 10)); err != nil {
		return err
	}
	return repo.r.HDel(ctx, friendRequestKey("from", userID), strconv.FormatUint(toUserID, 10))
}

func (repo *UserRepoImpl) ListFriends(ctx context.Context, userID uint64) ([]uint64, error) {
	friends := make(map[uint64]struct{})
	if repo.cassandra != nil {
		iter := repo.cassandra.Query("SELECT friend_id FROM friendships_by_user WHERE user_id = ?", userID).WithContext(ctx).Iter()
		var friendID uint64
		for iter.Scan(&friendID) {
			if friendID != 0 {
				friends[friendID] = struct{}{}
			}
		}
		if err := iter.Close(); err != nil {
			return nil, err
		}
	}
	values, err := repo.r.HGetAll(ctx, friendshipKey(userID))
	if err != nil {
		return nil, err
	}
	for raw := range values {
		id, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr == nil && id != 0 {
			friends[id] = struct{}{}
		}
	}
	result := make([]uint64, 0, len(friends))
	for id := range friends {
		result = append(result, id)
	}
	return result, nil
}

func (repo *UserRepoImpl) RemoveFriend(ctx context.Context, userID, friendID uint64) error {
	if err := repo.r.HDel(ctx, friendshipKey(userID), strconv.FormatUint(friendID, 10)); err != nil {
		return err
	}
	if err := repo.r.HDel(ctx, friendshipKey(friendID), strconv.FormatUint(userID, 10)); err != nil {
		return err
	}
	if err := repo.deleteFriendship(ctx, userID, friendID); err != nil {
		return err
	}
	return repo.deleteFriendship(ctx, friendID, userID)
}

func (repo *UserRepoImpl) CreateNotification(ctx context.Context, userID uint64, notification *Notification) error {
	if notification == nil || notification.ID == "" || notification.CreatedAt <= 0 {
		return errors.New("invalid notification")
	}
	if repo.outbox != nil {
		return repo.outbox.Enqueue(ctx, notification.Intent(userID))
	}
	if repo.cassandra != nil {
		rows, err := repo.notificationRows(ctx, userID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.id == notification.ID {
				return nil
			}
		}
		createdAt := time.UnixMilli(notification.CreatedAt).UTC()
		if err := repo.cassandra.Query("INSERT INTO notifications_by_user (user_id, created_at, notification_id, type, actor_id, payload, read) VALUES (?, ?, ?, ?, ?, ?, ?)",
			userID, createdAt, notification.ID, notification.Type, notification.ActorID, notification.Payload, notification.Read).WithContext(ctx).Exec(); err != nil {
			return err
		}
	}
	body, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	indexKey := common.Join(notificationIndexPrefix, ":", strconv.FormatUint(userID, 10))
	cached, err := repo.r.HGetAll(ctx, indexKey)
	if err != nil {
		return err
	}
	if _, exists := cached[notification.ID]; exists {
		return nil
	}
	if err := repo.r.RPush(ctx, common.Join(notificationPrefix, ":", strconv.FormatUint(userID, 10)), string(body)); err != nil {
		return err
	}
	// The marker prevents ordinary retries from duplicating the compatibility list.
	// Cassandra remains the durable source of truth when it is configured.
	_ = repo.r.HSet(ctx, indexKey, notification.ID, "1")
	return nil
}

func (repo *UserRepoImpl) GetNotifications(ctx context.Context, userID uint64) ([]*Notification, error) {
	byID := make(map[string]*Notification)
	if repo.cassandra != nil {
		iter := repo.cassandra.Query("SELECT created_at, notification_id, type, actor_id, payload, read FROM notifications_by_user WHERE user_id = ? LIMIT 100", userID).WithContext(ctx).Iter()
		var createdAt time.Time
		var notification Notification
		for iter.Scan(&createdAt, &notification.ID, &notification.Type, &notification.ActorID, &notification.Payload, &notification.Read) {
			notification.CreatedAt = createdAt.UnixMilli()
			copy := notification
			byID[copy.ID] = &copy
		}
		if err := iter.Close(); err != nil {
			return nil, err
		}
	}
	values, err := repo.r.LRange(ctx, common.Join(notificationPrefix, ":", strconv.FormatUint(userID, 10)), 0, 99)
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		var notification Notification
		if json.Unmarshal([]byte(value), &notification) == nil && notification.ID != "" {
			if _, exists := byID[notification.ID]; !exists {
				copy := notification
				byID[copy.ID] = &copy
			}
		}
	}
	result := make([]*Notification, 0, len(byID))
	for _, notification := range byID {
		result = append(result, notification)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	if len(result) > 100 {
		result = result[:100]
	}
	return result, nil
}

func (repo *UserRepoImpl) notificationRows(ctx context.Context, userID uint64) ([]struct {
	createdAt time.Time
	id        string
}, error) {
	if repo.cassandra == nil {
		return nil, nil
	}
	iter := repo.cassandra.Query("SELECT created_at, notification_id FROM notifications_by_user WHERE user_id = ? LIMIT 100", userID).WithContext(ctx).Iter()
	rows := make([]struct {
		createdAt time.Time
		id        string
	}, 0)
	var row struct {
		createdAt time.Time
		id        string
	}
	for iter.Scan(&row.createdAt, &row.id) {
		rows = append(rows, row)
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return rows, nil
}

func (repo *UserRepoImpl) MarkNotificationRead(ctx context.Context, userID uint64, notificationID string) error {
	rows, err := repo.notificationRows(ctx, userID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.id == notificationID {
			return repo.cassandra.Query("UPDATE notifications_by_user SET read = true WHERE user_id = ? AND created_at = ? AND notification_id = ?", userID, row.createdAt, row.id).WithContext(ctx).Exec()
		}
	}
	return ErrNotificationNotFound
}

func (repo *UserRepoImpl) MarkAllNotificationsRead(ctx context.Context, userID uint64) error {
	rows, err := repo.notificationRows(ctx, userID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := repo.cassandra.Query("UPDATE notifications_by_user SET read = true WHERE user_id = ? AND created_at = ? AND notification_id = ?", userID, row.createdAt, row.id).WithContext(ctx).Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (repo *UserRepoImpl) WasFriendRequestDeclined(ctx context.Context, fromUserID, toUserID uint64) (bool, error) {
	if repo.cassandra != nil {
		request, found, err := repo.getDurableFriendRequest(ctx, toUserID, fromUserID)
		if err != nil {
			return false, err
		}
		if found {
			return request.Status == "declined", nil
		}
	}
	values, err := repo.r.HGetAll(ctx, friendDeclinedKey(fromUserID, toUserID))
	if err != nil {
		return false, err
	}
	_, declined := values["status"]
	return declined, nil
}

func friendDeclinedKey(fromUserID, toUserID uint64) string {
	return common.Join(friendDeclinedPrefix, ":", strconv.FormatUint(fromUserID, 10), ":", strconv.FormatUint(toUserID, 10))
}

func (repo *UserRepoImpl) persistFriendship(ctx context.Context, userID, friendID uint64, createdAt time.Time) error {
	if repo.cassandra == nil {
		return nil
	}
	return repo.cassandra.Query("INSERT INTO friendships_by_user (user_id, friend_id, created_at) VALUES (?, ?, ?)", userID, friendID, createdAt).WithContext(ctx).Exec()
}

func (repo *UserRepoImpl) deleteFriendship(ctx context.Context, userID, friendID uint64) error {
	if repo.cassandra == nil {
		return nil
	}
	return repo.cassandra.Query("DELETE FROM friendships_by_user WHERE user_id = ? AND friend_id = ?", userID, friendID).WithContext(ctx).Exec()
}
