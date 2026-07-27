package chat

import (
	"context"
	"strconv"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/infra"
)

var (
	channelUsersPrefix = "rc:chanusers"
	onlineUsersPrefix  = "rc:onlineusers"
)

type UserRepoCache interface {
	AddUserToChannel(ctx context.Context, channelID uint64, userID uint64) error
	AreFriends(ctx context.Context, userID, peerID uint64) (bool, error)
	GetUserByID(ctx context.Context, userID uint64) (*User, error)
	GetUserIDBySession(ctx context.Context, sid string) (uint64, error)
	IsChannelUserExist(ctx context.Context, channelID, userID uint64) (bool, error)
	GetChannelUserIDs(ctx context.Context, channelID uint64) ([]uint64, error)
	AddOnlineUser(ctx context.Context, channelID uint64, userID uint64) error
	DeleteOnlineUser(ctx context.Context, channelID, userID uint64) error
	GetOnlineUserIDs(ctx context.Context, channelID uint64) ([]uint64, error)
}

type MessageRepoCache interface {
	InsertMessage(ctx context.Context, msg *Message) error
	MarkMessageRead(ctx context.Context, channelID, userID, messageID uint64) error
	GetLastReadMessageID(ctx context.Context, channelID, userID uint64) (uint64, error)
	PublishMessage(ctx context.Context, msg *Message) error
	ListMessages(ctx context.Context, channelID uint64, pageStateStr string) ([]*Message, string, error)
	EditMessage(ctx context.Context, channelID, messageID uint64, newPayload string, editedAt int64) error
	DeleteMessageForAll(ctx context.Context, channelID, messageID, deletedBy uint64) error
	GetMessage(ctx context.Context, channelID, messageID uint64) (*Message, error)
	AddReaction(ctx context.Context, channelID, messageID, userID uint64, emoji string) error
	RemoveReaction(ctx context.Context, channelID, messageID, userID uint64, emoji string) error
	GetReactions(ctx context.Context, channelID, messageID uint64) ([]ReactionSummary, error)
	PinMessage(ctx context.Context, channelID, messageID, pinnedBy uint64) error
	UnpinMessage(ctx context.Context, channelID, messageID uint64) error
	GetPinnedMessages(ctx context.Context, channelID uint64) ([]PinnedMessage, error)
	SearchMessages(ctx context.Context, channelID uint64, query string, limit int) ([]*Message, error)
	ListMediaMessages(ctx context.Context, channelID uint64, mediaType string, limit int) ([]*Message, error)
}

type ChannelRepoCache interface {
	CreateChannel(ctx context.Context, channelID uint64) (*Channel, error)
	SetChannelKind(ctx context.Context, channelID uint64, kind string) error
	GetChannelKind(ctx context.Context, channelID uint64) (string, error)
	GetDirectPeer(ctx context.Context, channelID, userID uint64) (uint64, error)
	DeleteChannel(ctx context.Context, channelID uint64) error
	AssignRole(ctx context.Context, channelID, userID uint64, role Role) error
	GetRole(ctx context.Context, channelID, userID uint64) (Role, error)
	CreateRoom(ctx context.Context, room *Room) error
	UpdateRoomAvatar(ctx context.Context, channelID uint64, avatar string) error
	GetRoom(ctx context.Context, channelID uint64) (*Room, error)
	GetRoomByInviteCode(ctx context.Context, inviteCode string) (*Room, error)
	GetDirectChannel(ctx context.Context, user1, user2 uint64) (*Channel, error)
	CreateDirectChannel(ctx context.Context, channelID, user1, user2 uint64) error
	IssueWebSocketTicket(ctx context.Context, userID, channelID uint64, accessToken string) (string, error)
	ConsumeWebSocketTicket(ctx context.Context, ticket string) (uint64, uint64, string, error)
	ListRoomsByUser(ctx context.Context, userID uint64) ([]Room, error)
	AddRoomMember(ctx context.Context, room *Room, userID uint64, role Role) error
	RemoveRoomMember(ctx context.Context, channelID, userID uint64) error
}

type UserRepoCacheImpl struct {
	r        infra.RedisCache
	userRepo UserRepo
}

func NewUserRepoCacheImpl(r infra.RedisCache, userRepo UserRepo) *UserRepoCacheImpl {
	return &UserRepoCacheImpl{r, userRepo}
}
func (cache *UserRepoCacheImpl) AddUserToChannel(ctx context.Context, channelID uint64, userID uint64) error {
	if err := cache.userRepo.AddUserToChannel(ctx, channelID, userID); err != nil {
		return err
	}
	key := constructKey(channelUsersPrefix, channelID)
	return cache.r.HSet(ctx, key, strconv.FormatUint(userID, 10), 1)
}
func (cache *UserRepoCacheImpl) AreFriends(ctx context.Context, userID, peerID uint64) (bool, error) {
	friends, err := cache.r.HGetAll(ctx, common.Join("rc:friendship", ":", strconv.FormatUint(userID, 10)))
	if err != nil {
		return false, err
	}
	_, exists := friends[strconv.FormatUint(peerID, 10)]
	return exists, nil
}
func (cache *UserRepoCacheImpl) GetUserByID(ctx context.Context, userID uint64) (*User, error) {
	return cache.userRepo.GetUserByID(ctx, userID)
}
func (cache *UserRepoCacheImpl) GetUserIDBySession(ctx context.Context, sid string) (uint64, error) {
	return cache.userRepo.GetUserIDBySession(ctx, sid)
}
func (cache *UserRepoCacheImpl) IsChannelUserExist(ctx context.Context, channelID, userID uint64) (bool, error) {
	key := constructKey(channelUsersPrefix, channelID)
	var dummy int
	var err error
	channelExists, userExists, err := cache.r.HGetIfKeyExists(ctx, key, strconv.FormatUint(userID, 10), &dummy)
	if err != nil {
		return false, err
	}
	if channelExists {
		if userExists {
			return true, nil
		}
		// Redis can contain a stale partial membership hash after a room was
		// joined by an older application version. Refresh from Cassandra before
		// denying the WebSocket/ticket request.
		channelUserIDs, err := cache.userRepo.GetChannelUserIDs(ctx, channelID)
		if err != nil {
			return false, err
		}
		args := make([]interface{}, 0, len(channelUserIDs)*2)
		channelUserExist := false
		for _, channelUserID := range channelUserIDs {
			if userID == channelUserID {
				channelUserExist = true
			}
			args = append(args, channelUserID, 1)
		}
		if len(args) > 0 {
			if err := cache.r.HSet(ctx, key, args...); err != nil {
				return channelUserExist, err
			}
		} else {
			_ = cache.r.Delete(ctx, key)
		}
		return channelUserExist, nil
	}

	channelUserIDs, err := cache.userRepo.GetChannelUserIDs(ctx, channelID)
	if err != nil {
		return false, err
	}
	channelUserExist := false
	var args []interface{}
	for _, channelUserID := range channelUserIDs {
		if userID == channelUserID {
			channelUserExist = true
		}
		args = append(args, channelUserID, 1)
	}
	if len(args) > 0 {
		if err := cache.r.HSet(ctx, key, args...); err != nil {
			return channelUserExist, err
		}
	}
	return channelUserExist, nil
}
func (cache *UserRepoCacheImpl) GetChannelUserIDs(ctx context.Context, channelID uint64) ([]uint64, error) {
	key := constructKey(channelUsersPrefix, channelID)
	userMap, err := cache.r.HGetAll(ctx, key)
	if err != nil {
		return nil, err
	}
	var userIDs []uint64
	if len(userMap) > 0 {
		for userIDStr := range userMap {
			userID, err := strconv.ParseUint(userIDStr, 10, 64)
			if err != nil {
				return nil, err
			}
			userIDs = append(userIDs, userID)
		}
		return userIDs, nil
	}

	userIDs, err = cache.userRepo.GetChannelUserIDs(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var args []interface{}
	for _, userID := range userIDs {
		args = append(args, userID, 1)
	}
	if err := cache.r.HSet(ctx, key, args...); err != nil {
		return userIDs, err
	}
	return userIDs, nil
}
func (cache *UserRepoCacheImpl) AddOnlineUser(ctx context.Context, channelID uint64, userID uint64) error {
	key := constructKey(onlineUsersPrefix, channelID)
	_, err := cache.r.HIncrBy(ctx, key, strconv.FormatUint(userID, 10), 1)
	return err
}
func (cache *UserRepoCacheImpl) DeleteOnlineUser(ctx context.Context, channelID, userID uint64) error {
	key := constructKey(onlineUsersPrefix, channelID)
	userKey := strconv.FormatUint(userID, 10)
	count, err := cache.r.HIncrBy(ctx, key, userKey, -1)
	if err != nil {
		return err
	}
	if count <= 0 {
		return cache.r.HDel(ctx, key, userKey)
	}
	return nil
}
func (cache *UserRepoCacheImpl) GetOnlineUserIDs(ctx context.Context, channelID uint64) ([]uint64, error) {
	key := constructKey(onlineUsersPrefix, channelID)
	userMap, err := cache.r.HGetAll(ctx, key)
	if err != nil {
		return nil, err
	}
	var userIDs []uint64
	for userIDStr := range userMap {
		userID, err := strconv.ParseUint(userIDStr, 10, 64)
		if err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs, nil
}

type MessageRepoCacheImpl struct {
	messageRepo MessageRepo
}

func NewMessageRepoCacheImpl(messageRepo MessageRepo) *MessageRepoCacheImpl {
	return &MessageRepoCacheImpl{messageRepo}
}

func (cache *MessageRepoCacheImpl) InsertMessage(ctx context.Context, msg *Message) error {
	return cache.messageRepo.InsertMessage(ctx, msg)
}
func (cache *MessageRepoCacheImpl) MarkMessageRead(ctx context.Context, channelID, userID, messageID uint64) error {
	return cache.messageRepo.MarkMessageRead(ctx, channelID, userID, messageID)
}
func (cache *MessageRepoCacheImpl) GetLastReadMessageID(ctx context.Context, channelID, userID uint64) (uint64, error) {
	return cache.messageRepo.GetLastReadMessageID(ctx, channelID, userID)
}
func (cache *MessageRepoCacheImpl) PublishMessage(ctx context.Context, msg *Message) error {
	return cache.messageRepo.PublishMessage(ctx, msg)
}
func (cache *MessageRepoCacheImpl) ListMessages(ctx context.Context, channelID uint64, pageStateStr string) ([]*Message, string, error) {
	return cache.messageRepo.ListMessages(ctx, channelID, pageStateStr)
}
func (cache *MessageRepoCacheImpl) EditMessage(ctx context.Context, channelID, messageID uint64, newPayload string, editedAt int64) error {
	return cache.messageRepo.EditMessage(ctx, channelID, messageID, newPayload, editedAt)
}
func (cache *MessageRepoCacheImpl) DeleteMessageForAll(ctx context.Context, channelID, messageID, deletedBy uint64) error {
	return cache.messageRepo.DeleteMessageForAll(ctx, channelID, messageID, deletedBy)
}
func (cache *MessageRepoCacheImpl) GetMessage(ctx context.Context, channelID, messageID uint64) (*Message, error) {
	return cache.messageRepo.GetMessage(ctx, channelID, messageID)
}
func (cache *MessageRepoCacheImpl) AddReaction(ctx context.Context, channelID, messageID, userID uint64, emoji string) error {
	return cache.messageRepo.AddReaction(ctx, channelID, messageID, userID, emoji)
}
func (cache *MessageRepoCacheImpl) RemoveReaction(ctx context.Context, channelID, messageID, userID uint64, emoji string) error {
	return cache.messageRepo.RemoveReaction(ctx, channelID, messageID, userID, emoji)
}
func (cache *MessageRepoCacheImpl) GetReactions(ctx context.Context, channelID, messageID uint64) ([]ReactionSummary, error) {
	return cache.messageRepo.GetReactions(ctx, channelID, messageID)
}
func (cache *MessageRepoCacheImpl) PinMessage(ctx context.Context, channelID, messageID, pinnedBy uint64) error {
	return cache.messageRepo.PinMessage(ctx, channelID, messageID, pinnedBy)
}
func (cache *MessageRepoCacheImpl) UnpinMessage(ctx context.Context, channelID, messageID uint64) error {
	return cache.messageRepo.UnpinMessage(ctx, channelID, messageID)
}
func (cache *MessageRepoCacheImpl) GetPinnedMessages(ctx context.Context, channelID uint64) ([]PinnedMessage, error) {
	return cache.messageRepo.GetPinnedMessages(ctx, channelID)
}
func (cache *MessageRepoCacheImpl) SearchMessages(ctx context.Context, channelID uint64, query string, limit int) ([]*Message, error) {
	return cache.messageRepo.SearchMessages(ctx, channelID, query, limit)
}
func (cache *MessageRepoCacheImpl) ListMediaMessages(ctx context.Context, channelID uint64, mediaType string, limit int) ([]*Message, error) {
	return cache.messageRepo.ListMediaMessages(ctx, channelID, mediaType, limit)
}

type ChannelRepoCacheImpl struct {
	r           infra.RedisCache
	channelRepo ChannelRepo
}

func NewChannelRepoCacheImpl(r infra.RedisCache, channelRepo ChannelRepo) *ChannelRepoCacheImpl {
	return &ChannelRepoCacheImpl{r, channelRepo}
}

func (cache *ChannelRepoCacheImpl) CreateChannel(ctx context.Context, channelID uint64) (*Channel, error) {
	return cache.channelRepo.CreateChannel(ctx, channelID)
}
func (cache *ChannelRepoCacheImpl) SetChannelKind(ctx context.Context, channelID uint64, kind string) error {
	return cache.channelRepo.SetChannelKind(ctx, channelID, kind)
}
func (cache *ChannelRepoCacheImpl) GetChannelKind(ctx context.Context, channelID uint64) (string, error) {
	return cache.channelRepo.GetChannelKind(ctx, channelID)
}
func (cache *ChannelRepoCacheImpl) GetDirectPeer(ctx context.Context, channelID, userID uint64) (uint64, error) {
	return cache.channelRepo.GetDirectPeer(ctx, channelID, userID)
}
func (cache *ChannelRepoCacheImpl) DeleteChannel(ctx context.Context, channelID uint64) error {
	if err := cache.channelRepo.DeleteChannel(ctx, channelID); err != nil {
		return err
	}
	cmds := []infra.RedisCmd{
		{
			OpType: infra.DELETE,
			Payload: infra.RedisDeletePayload{
				Key: constructKey(onlineUsersPrefix, channelID),
			},
		},
		{
			OpType: infra.DELETE,
			Payload: infra.RedisDeletePayload{
				Key: constructKey(channelUsersPrefix, channelID),
			},
		},
	}
	return cache.r.ExecPipeLine(ctx, &cmds)
}
func (cache *ChannelRepoCacheImpl) AssignRole(ctx context.Context, channelID, userID uint64, role Role) error {
	return cache.channelRepo.AssignRole(ctx, channelID, userID, role)
}
func (cache *ChannelRepoCacheImpl) GetRole(ctx context.Context, channelID, userID uint64) (Role, error) {
	return cache.channelRepo.GetRole(ctx, channelID, userID)
}
func (cache *ChannelRepoCacheImpl) CreateRoom(ctx context.Context, room *Room) error {
	if err := cache.channelRepo.CreateRoom(ctx, room); err != nil {
		return err
	}
	return cache.r.HSet(ctx, constructKey(channelUsersPrefix, room.ChannelID), strconv.FormatUint(room.OwnerID, 10), 1)
}
func (cache *ChannelRepoCacheImpl) UpdateRoomAvatar(ctx context.Context, channelID uint64, avatar string) error {
	return cache.channelRepo.UpdateRoomAvatar(ctx, channelID, avatar)
}
func (cache *ChannelRepoCacheImpl) GetRoom(ctx context.Context, channelID uint64) (*Room, error) {
	return cache.channelRepo.GetRoom(ctx, channelID)
}
func (cache *ChannelRepoCacheImpl) GetRoomByInviteCode(ctx context.Context, inviteCode string) (*Room, error) {
	return cache.channelRepo.GetRoomByInviteCode(ctx, inviteCode)
}
func (cache *ChannelRepoCacheImpl) GetDirectChannel(ctx context.Context, user1, user2 uint64) (*Channel, error) {
	return cache.channelRepo.GetDirectChannel(ctx, user1, user2)
}
func (cache *ChannelRepoCacheImpl) IssueWebSocketTicket(ctx context.Context, userID, channelID uint64, accessToken string) (string, error) {
	return cache.channelRepo.IssueWebSocketTicket(ctx, userID, channelID, accessToken)
}
func (cache *ChannelRepoCacheImpl) ConsumeWebSocketTicket(ctx context.Context, ticket string) (uint64, uint64, string, error) {
	return cache.channelRepo.ConsumeWebSocketTicket(ctx, ticket)
}
func (cache *ChannelRepoCacheImpl) CreateDirectChannel(ctx context.Context, channelID, user1, user2 uint64) error {
	return cache.channelRepo.CreateDirectChannel(ctx, channelID, user1, user2)
}
func (cache *ChannelRepoCacheImpl) ListRoomsByUser(ctx context.Context, userID uint64) ([]Room, error) {
	return cache.channelRepo.ListRoomsByUser(ctx, userID)
}
func (cache *ChannelRepoCacheImpl) AddRoomMember(ctx context.Context, room *Room, userID uint64, role Role) error {
	if err := cache.channelRepo.AddRoomMember(ctx, room, userID, role); err != nil {
		return err
	}
	return cache.r.HSet(ctx, constructKey(channelUsersPrefix, room.ChannelID), strconv.FormatUint(userID, 10), 1)
}
func (cache *ChannelRepoCacheImpl) RemoveRoomMember(ctx context.Context, channelID, userID uint64) error {
	if err := cache.channelRepo.RemoveRoomMember(ctx, channelID, userID); err != nil {
		return err
	}
	return cache.r.HDel(ctx, constructKey(channelUsersPrefix, channelID), strconv.FormatUint(userID, 10))
}

func constructKey(prefix string, id uint64) string {
	return common.Join(prefix, ":", strconv.FormatUint(id, 10))
}
