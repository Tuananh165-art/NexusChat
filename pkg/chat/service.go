package chat

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/realtime"
)

const (
	reactionActionAdd    = "add"
	reactionActionRemove = "remove"
	pinActionPin         = "pin"
	pinActionUnpin       = "unpin"
)

type MessageService interface {
	BroadcastTextMessage(ctx context.Context, channelID, userID uint64, payload string, parentID uint64) error
	BroadcastConnectMessage(ctx context.Context, channelID, userID uint64) error
	BroadcastActionMessage(ctx context.Context, channelID, userID uint64, action Action) error
	BroadcastFileMessage(ctx context.Context, channelID, userID uint64, payload string) error
	MarkMessageSeen(ctx context.Context, channelID, userID, messageID uint64) error
	InsertMessage(ctx context.Context, msg *Message) error
	PublishMessage(ctx context.Context, msg *Message) error
	ListMessages(ctx context.Context, channelID uint64, pageState string) ([]*Message, string, error)
	EditMessage(ctx context.Context, channelID, userID, messageID uint64, newPayload string) error
	DeleteMessageForAll(ctx context.Context, channelID, userID, messageID uint64) error
	AddReaction(ctx context.Context, channelID, userID, messageID uint64, emoji string) error
	RemoveReaction(ctx context.Context, channelID, userID, messageID uint64, emoji string) error
	PinMessage(ctx context.Context, channelID, userID, messageID uint64) error
	UnpinMessage(ctx context.Context, channelID, messageID uint64) error
	GetPinnedMessages(ctx context.Context, channelID uint64) ([]PinnedMessage, error)
	SearchMessages(ctx context.Context, channelID uint64, query string, limit int) ([]*Message, error)
	ListMediaMessages(ctx context.Context, channelID uint64, mediaType string, limit int) ([]*Message, error)
}

type UserService interface {
	AddUserToChannel(ctx context.Context, channelID, userID uint64) error
	GetUser(ctx context.Context, userID uint64) (*User, error)
	IsChannelUserExist(ctx context.Context, channelID, userID uint64) (bool, error)
	GetChannelUserIDs(ctx context.Context, channelID uint64) ([]uint64, error)
	AddOnlineUser(ctx context.Context, channelID, userID uint64) error
	DeleteOnlineUser(ctx context.Context, channelID, userID uint64) error
	GetOnlineUserIDs(ctx context.Context, channelID uint64) ([]uint64, error)
}

type ChannelService interface {
	CreateChannel(ctx context.Context) (*Channel, error)
	DeleteChannel(ctx context.Context, channelID uint64) error
	AssignRole(ctx context.Context, channelID, userID uint64, role Role) error
	GetRole(ctx context.Context, channelID, userID uint64) (Role, error)
	CreateRoom(ctx context.Context, ownerID uint64, name string, memberIDs []uint64) (*Room, error)
	JoinRoom(ctx context.Context, userID uint64, inviteCode string) (*Room, error)
	ListRooms(ctx context.Context, userID uint64) ([]Room, error)
	OpenRoom(ctx context.Context, userID, channelID uint64) (*Channel, error)
	LeaveRoom(ctx context.Context, userID, channelID uint64) error
}

type ForwardService interface {
	RegisterChannelSession(ctx context.Context, channelID, userID uint64, subscriber string) error
	RemoveChannelSession(ctx context.Context, channelID, userID uint64) error
}

type MessageServiceImpl struct {
	msgRepo  MessageRepoCache
	userRepo UserRepoCache
	sf       common.IDGenerator
}

func NewMessageServiceImpl(msgRepo MessageRepoCache, userRepo UserRepoCache, sf common.IDGenerator) *MessageServiceImpl {
	return &MessageServiceImpl{msgRepo, userRepo, sf}
}
func (svc *MessageServiceImpl) BroadcastTextMessage(ctx context.Context, channelID, userID uint64, payload string, parentID uint64) error {
	msg, err := svc.newMessage(EventText, channelID, userID, payload)
	if err != nil {
		return fmt.Errorf("error create text message: %w", err)
	}
	msg.ParentID = parentID
	if endpoint := os.Getenv("CHAT_GRPC_CLIENT_SAFETY_ENDPOINT"); endpoint != "" {
		decision, moderationErr := realtime.CallStructRPC(ctx, endpoint, "nexuschat.safety.v1.SafetyService", "ModerateMessage", map[string]any{
			"channel_id": strconv.FormatUint(channelID, 10),
			"message_id": strconv.FormatUint(msg.MessageID, 10),
			"user_id":    strconv.FormatUint(userID, 10),
			"content":    payload,
		})
		if moderationErr != nil {
			// Degraded mode: keep chat available; Safety consumes the chat event
			// and can apply an asynchronous decision later.
			slog.Warn("safety moderation unavailable", "error", moderationErr)
		} else if decision != nil {
			action := decision.GetFields()["action"].GetStringValue()
			if action == "block" {
				return fmt.Errorf("message blocked by safety policy: %s", decision.GetFields()["reason"].GetStringValue())
			}
			if action == "warn" {
				msg.Payload = "[Nội dung đã được cảnh báo bởi Safety] " + payload
			}
		}
	}
	if parentID > 0 {
		parent, err := svc.msgRepo.GetMessage(ctx, channelID, parentID)
		if err == nil && parent != nil {
			msg.ReplyPreview = newReplyPreview(parent)
		}
	}
	if err := svc.msgRepo.InsertMessage(ctx, msg); err != nil {
		return fmt.Errorf("error broadcast text message: %w", err)
	}
	if err := svc.PublishMessage(ctx, msg); err != nil {
		return fmt.Errorf("error broadcast text message: %w", err)
	}
	return nil
}
func (svc *MessageServiceImpl) BroadcastConnectMessage(ctx context.Context, channelID, userID uint64) error {
	onlineUserIDs, err := svc.userRepo.GetOnlineUserIDs(ctx, channelID)
	if err != nil {
		return fmt.Errorf("error get online user ids from channel %d: %w", channelID, err)
	}
	if len(onlineUserIDs) == 1 {
		return svc.BroadcastActionMessage(ctx, channelID, userID, WaitingMessage)
	}
	return svc.BroadcastActionMessage(ctx, channelID, userID, JoinedMessage)
}
func (svc *MessageServiceImpl) BroadcastActionMessage(ctx context.Context, channelID, userID uint64, action Action) error {
	msg, err := svc.newMessage(EventAction, channelID, userID, string(action))
	if err != nil {
		return fmt.Errorf("error create action message: %w", err)
	}
	if err := svc.PublishMessage(ctx, msg); err != nil {
		return fmt.Errorf("error broadcast action message: %w", err)
	}
	return nil
}
func (svc *MessageServiceImpl) BroadcastFileMessage(ctx context.Context, channelID, userID uint64, payload string) error {
	msg, err := svc.newMessage(EventFile, channelID, userID, payload)
	if err != nil {
		return fmt.Errorf("error create file message: %w", err)
	}
	if err := svc.msgRepo.InsertMessage(ctx, msg); err != nil {
		return fmt.Errorf("error broadcast file message: %w", err)
	}
	if err := svc.PublishMessage(ctx, msg); err != nil {
		return fmt.Errorf("error broadcast file message: %w", err)
	}
	return nil
}
func (svc *MessageServiceImpl) MarkMessageSeen(ctx context.Context, channelID, userID, messageID uint64) error {
	if err := svc.msgRepo.MarkMessageSeen(ctx, channelID, messageID); err != nil {
		return fmt.Errorf("error mark message %d seen in channel %d: %w", messageID, channelID, err)
	}
	msg, err := svc.newMessage(EventSeen, channelID, userID, formatUint(messageID))
	if err != nil {
		return fmt.Errorf("error create seen event message: %w", err)
	}
	msg.Seen = true
	if err := svc.PublishMessage(ctx, msg); err != nil {
		return fmt.Errorf("error mark message %d seen in channel %d: %w", messageID, channelID, err)
	}
	return nil
}
func (svc *MessageServiceImpl) InsertMessage(ctx context.Context, msg *Message) error {
	if err := svc.msgRepo.InsertMessage(ctx, msg); err != nil {
		return fmt.Errorf("error insert message: %w", err)
	}
	return nil
}
func (svc *MessageServiceImpl) PublishMessage(ctx context.Context, msg *Message) error {
	if err := svc.msgRepo.PublishMessage(ctx, msg); err != nil {
		return fmt.Errorf("error publish message: %w", err)
	}
	return nil
}
func (svc *MessageServiceImpl) ListMessages(ctx context.Context, channelID uint64, pageState string) ([]*Message, string, error) {
	msgs, nextPageState, err := svc.msgRepo.ListMessages(ctx, channelID, pageState)
	if err != nil {
		return nil, "", fmt.Errorf("error list messages in channel %d with page state %s: %w", channelID, pageState, err)
	}
	return msgs, nextPageState, nil
}
func (svc *MessageServiceImpl) EditMessage(ctx context.Context, channelID, userID, messageID uint64, newPayload string) error {
	msg, err := svc.msgRepo.GetMessage(ctx, channelID, messageID)
	if err != nil {
		return fmt.Errorf("error get message %d for edit: %w", messageID, err)
	}
	if msg.UserID != userID {
		return ErrNotMessageOwner
	}
	if msg.DeletedForAll {
		return ErrMessageAlreadyDeleted
	}
	editedAt := time.Now().UnixMilli()
	if err := svc.msgRepo.EditMessage(ctx, channelID, messageID, newPayload, editedAt); err != nil {
		return fmt.Errorf("error edit message %d: %w", messageID, err)
	}
	editMsg, err := svc.newMessage(EventEdit, channelID, userID, formatEventPayload(formatUint(messageID), newPayload))
	if err != nil {
		return fmt.Errorf("error create edit event: %w", err)
	}
	editMsg.Time = editedAt
	editMsg.EditedAt = editedAt
	if err := svc.PublishMessage(ctx, editMsg); err != nil {
		return fmt.Errorf("error publish edit message %d: %w", messageID, err)
	}
	return nil
}
func (svc *MessageServiceImpl) DeleteMessageForAll(ctx context.Context, channelID, userID, messageID uint64) error {
	msg, err := svc.msgRepo.GetMessage(ctx, channelID, messageID)
	if err != nil {
		return fmt.Errorf("error get message %d for delete: %w", messageID, err)
	}
	if msg.UserID != userID {
		return ErrNotMessageOwner
	}
	if msg.DeletedForAll {
		return ErrMessageAlreadyDeleted
	}
	if err := svc.msgRepo.DeleteMessageForAll(ctx, channelID, messageID, userID); err != nil {
		return fmt.Errorf("error delete message %d for all: %w", messageID, err)
	}
	deleteMsg, err := svc.newMessage(EventDelete, channelID, userID, formatUint(messageID))
	if err != nil {
		return fmt.Errorf("error create delete event: %w", err)
	}
	deleteMsg.DeletedForAll = true
	deleteMsg.DeletedBy = userID
	if err := svc.PublishMessage(ctx, deleteMsg); err != nil {
		return fmt.Errorf("error publish delete message %d: %w", messageID, err)
	}
	return nil
}
func (svc *MessageServiceImpl) AddReaction(ctx context.Context, channelID, userID, messageID uint64, emoji string) error {
	if err := svc.msgRepo.AddReaction(ctx, channelID, messageID, userID, emoji); err != nil {
		return fmt.Errorf("error add reaction: %w", err)
	}
	reactionMsg, err := svc.newMessage(EventReaction, channelID, userID, formatEventPayload(formatUint(messageID), emoji, reactionActionAdd))
	if err != nil {
		return fmt.Errorf("error create reaction event: %w", err)
	}
	return svc.PublishMessage(ctx, reactionMsg)
}
func (svc *MessageServiceImpl) RemoveReaction(ctx context.Context, channelID, userID, messageID uint64, emoji string) error {
	if err := svc.msgRepo.RemoveReaction(ctx, channelID, messageID, userID, emoji); err != nil {
		return fmt.Errorf("error remove reaction: %w", err)
	}
	reactionMsg, err := svc.newMessage(EventReaction, channelID, userID, formatEventPayload(formatUint(messageID), emoji, reactionActionRemove))
	if err != nil {
		return fmt.Errorf("error create reaction event: %w", err)
	}
	return svc.PublishMessage(ctx, reactionMsg)
}
func (svc *MessageServiceImpl) PinMessage(ctx context.Context, channelID, userID, messageID uint64) error {
	if err := svc.msgRepo.PinMessage(ctx, channelID, messageID, userID); err != nil {
		return fmt.Errorf("error pin message: %w", err)
	}
	pinMsg, err := svc.newMessage(EventPin, channelID, userID, formatEventPayload(formatUint(messageID), pinActionPin))
	if err != nil {
		return fmt.Errorf("error create pin event: %w", err)
	}
	return svc.PublishMessage(ctx, pinMsg)
}
func (svc *MessageServiceImpl) UnpinMessage(ctx context.Context, channelID, messageID uint64) error {
	if err := svc.msgRepo.UnpinMessage(ctx, channelID, messageID); err != nil {
		return fmt.Errorf("error unpin message: %w", err)
	}
	pinMsg, err := svc.newMessage(EventPin, channelID, 0, formatEventPayload(formatUint(messageID), pinActionUnpin))
	if err != nil {
		return fmt.Errorf("error create pin event: %w", err)
	}
	return svc.PublishMessage(ctx, pinMsg)
}
func (svc *MessageServiceImpl) GetPinnedMessages(ctx context.Context, channelID uint64) ([]PinnedMessage, error) {
	return svc.msgRepo.GetPinnedMessages(ctx, channelID)
}
func (svc *MessageServiceImpl) SearchMessages(ctx context.Context, channelID uint64, query string, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 50
	}
	return svc.msgRepo.SearchMessages(ctx, channelID, query, limit)
}
func (svc *MessageServiceImpl) ListMediaMessages(ctx context.Context, channelID uint64, mediaType string, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 50
	}
	return svc.msgRepo.ListMediaMessages(ctx, channelID, mediaType, limit)
}

func (svc *MessageServiceImpl) newMessage(event int, channelID, userID uint64, payload string) (*Message, error) {
	messageID, err := svc.sf.NextID()
	if err != nil {
		return nil, fmt.Errorf("error create snowflake ID: %w", err)
	}
	return &Message{
		MessageID: messageID,
		Event:     event,
		ChannelID: channelID,
		UserID:    userID,
		Payload:   payload,
		Time:      time.Now().UnixMilli(),
	}, nil
}

func newReplyPreview(parent *Message) *ReplyPreview {
	return &ReplyPreview{
		MessageID: formatUint(parent.MessageID),
		UserID:    formatUint(parent.UserID),
		Payload:   parent.Payload,
	}
}

func formatUint(v uint64) string {
	return strconv.FormatUint(v, 10)
}

func formatEventPayload(parts ...string) string {
	return strings.Join(parts, "|")
}

type UserServiceImpl struct {
	userRepo UserRepoCache
}

func NewUserServiceImpl(userRepo UserRepoCache) *UserServiceImpl {
	return &UserServiceImpl{userRepo}
}
func (svc *UserServiceImpl) AddUserToChannel(ctx context.Context, channelID, userID uint64) error {
	if err := svc.userRepo.AddUserToChannel(ctx, channelID, userID); err != nil {
		return fmt.Errorf("error add user %d to channel %d: %w", userID, channelID, err)
	}
	return nil
}
func (svc *UserServiceImpl) GetUser(ctx context.Context, userID uint64) (*User, error) {
	user, err := svc.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("error get user %d: %w", userID, err)
	}
	return user, nil
}
func (svc *UserServiceImpl) IsChannelUserExist(ctx context.Context, channelID, userID uint64) (bool, error) {
	exist, err := svc.userRepo.IsChannelUserExist(ctx, channelID, userID)
	if err != nil {
		return false, fmt.Errorf("error check user %d in channel %d: %w", userID, channelID, err)
	}
	return exist, nil
}
func (svc *UserServiceImpl) GetChannelUserIDs(ctx context.Context, channelID uint64) ([]uint64, error) {
	users, err := svc.userRepo.GetChannelUserIDs(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("error get users in channel %d: %w", channelID, err)
	}
	return users, nil
}
func (svc *UserServiceImpl) AddOnlineUser(ctx context.Context, channelID, userID uint64) error {
	if err := svc.userRepo.AddOnlineUser(ctx, channelID, userID); err != nil {
		return fmt.Errorf("error add online user %d to channel %d: %w", userID, channelID, err)
	}
	return nil
}
func (svc *UserServiceImpl) DeleteOnlineUser(ctx context.Context, channelID, userID uint64) error {
	if err := svc.userRepo.DeleteOnlineUser(ctx, channelID, userID); err != nil {
		return fmt.Errorf("error delete online user %d from channel %d: %w", userID, channelID, err)
	}
	return nil
}
func (svc *UserServiceImpl) GetOnlineUserIDs(ctx context.Context, channelID uint64) ([]uint64, error) {
	users, err := svc.userRepo.GetOnlineUserIDs(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("error get online users in channel %d: %w", channelID, err)
	}
	return users, nil
}

type ChannelServiceImpl struct {
	chanRepo ChannelRepoCache
	userRepo UserRepoCache
	sf       common.IDGenerator
}

func NewChannelServiceImpl(chanRepo ChannelRepoCache, userRepo UserRepoCache, sf common.IDGenerator) *ChannelServiceImpl {
	return &ChannelServiceImpl{chanRepo, userRepo, sf}
}
func (svc *ChannelServiceImpl) CreateChannel(ctx context.Context) (*Channel, error) {
	channelID, err := svc.sf.NextID()
	if err != nil {
		return nil, fmt.Errorf("error create snowflake ID for new channel: %w", err)
	}
	channel, err := svc.chanRepo.CreateChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("error create channel %d: %w", channelID, err)
	}
	return channel, nil
}
func (svc *ChannelServiceImpl) DeleteChannel(ctx context.Context, channelID uint64) error {
	if err := svc.chanRepo.DeleteChannel(ctx, channelID); err != nil {
		return fmt.Errorf("error delete channel %d: %w", channelID, err)
	}
	return nil
}
func (svc *ChannelServiceImpl) AssignRole(ctx context.Context, channelID, userID uint64, role Role) error {
	return svc.chanRepo.AssignRole(ctx, channelID, userID, role)
}
func (svc *ChannelServiceImpl) GetRole(ctx context.Context, channelID, userID uint64) (Role, error) {
	return svc.chanRepo.GetRole(ctx, channelID, userID)
}
func (svc *ChannelServiceImpl) CreateRoom(ctx context.Context, ownerID uint64, name string, memberIDs []uint64) (*Room, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("room name is required")
	}
	if len([]rune(name)) > 80 {
		return nil, fmt.Errorf("room name is too long")
	}
	if _, err := svc.userRepo.GetUserByID(ctx, ownerID); err != nil {
		return nil, err
	}
	channelID, err := svc.sf.NextID()
	if err != nil {
		return nil, fmt.Errorf("error create snowflake ID for new room: %w", err)
	}
	uniqueMembers := map[uint64]Role{ownerID: RoleOwner}
	for _, memberID := range memberIDs {
		if memberID == 0 || memberID == ownerID {
			continue
		}
		if len(uniqueMembers) >= 50 {
			return nil, fmt.Errorf("room member limit exceeded")
		}
		if _, err := svc.userRepo.GetUserByID(ctx, memberID); err != nil {
			return nil, fmt.Errorf("member %d not found: %w", memberID, err)
		}
		uniqueMembers[memberID] = RoleMember
	}
	room := &Room{
		ChannelID:   channelID,
		Name:        name,
		OwnerID:     ownerID,
		MemberCount: len(uniqueMembers),
	}
	if err := svc.chanRepo.CreateRoom(ctx, room); err != nil {
		return nil, fmt.Errorf("error create room: %w", err)
	}
	for memberID, role := range uniqueMembers {
		if memberID == ownerID {
			continue
		}
		if err := svc.chanRepo.AddRoomMember(ctx, room, memberID, role); err != nil {
			return nil, fmt.Errorf("error add room member %d: %w", memberID, err)
		}
	}
	room.Role = RoleOwner
	return room, nil
}
func (svc *ChannelServiceImpl) JoinRoom(ctx context.Context, userID uint64, inviteCode string) (*Room, error) {
	inviteCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	if inviteCode == "" {
		return nil, fmt.Errorf("invite code is required")
	}
	if _, err := svc.userRepo.GetUserByID(ctx, userID); err != nil {
		return nil, err
	}
	room, err := svc.chanRepo.GetRoomByInviteCode(ctx, inviteCode)
	if err != nil {
		return nil, fmt.Errorf("room invite not found: %w", err)
	}
	exists, err := svc.userRepo.IsChannelUserExist(ctx, room.ChannelID, userID)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := svc.chanRepo.AddRoomMember(ctx, room, userID, RoleMember); err != nil {
			return nil, err
		}
	}
	room.Role = RoleMember
	if userID == room.OwnerID {
		room.Role = RoleOwner
	}
	return room, nil
}
func (svc *ChannelServiceImpl) ListRooms(ctx context.Context, userID uint64) ([]Room, error) {
	return svc.chanRepo.ListRoomsByUser(ctx, userID)
}
func (svc *ChannelServiceImpl) OpenRoom(ctx context.Context, userID, channelID uint64) (*Channel, error) {
	exists, err := svc.userRepo.IsChannelUserExist(ctx, channelID, userID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrChannelOrUserNotFound
	}
	if _, err := svc.chanRepo.GetRoom(ctx, channelID); err != nil {
		return nil, err
	}
	token, err := common.NewJWT(channelID)
	if err != nil {
		return nil, err
	}
	return &Channel{ID: channelID, AccessToken: token}, nil
}
func (svc *ChannelServiceImpl) LeaveRoom(ctx context.Context, userID, channelID uint64) error {
	room, err := svc.chanRepo.GetRoom(ctx, channelID)
	if err != nil {
		return err
	}
	if room.OwnerID == userID {
		return fmt.Errorf("room owner cannot leave; transfer ownership is not implemented")
	}
	return svc.chanRepo.RemoveRoomMember(ctx, channelID, userID)
}

type ForwardServiceImpl struct {
	forwardRepo ForwardRepo
}

func NewForwardServiceImpl(forwardRepo ForwardRepo) *ForwardServiceImpl {
	return &ForwardServiceImpl{forwardRepo}
}

func (svc *ForwardServiceImpl) RegisterChannelSession(ctx context.Context, channelID, userID uint64, subscriber string) error {
	return svc.forwardRepo.RegisterChannelSession(ctx, channelID, userID, subscriber)
}
func (svc *ForwardServiceImpl) RemoveChannelSession(ctx context.Context, channelID, userID uint64) error {
	return svc.forwardRepo.RemoveChannelSession(ctx, channelID, userID)
}
