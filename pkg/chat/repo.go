package chat

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/gocql/gocql"
	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/config"

	"github.com/go-kit/kit/endpoint"
	"github.com/Tuananh165-art/NexusChat/pkg/transport"
	forwarderpb "github.com/Tuananh165-art/NexusChat/proto/forwarder"
	userpb "github.com/Tuananh165-art/NexusChat/proto/user"
)

var (
	MessagePubTopic = "rc.msg.pub"
)

type UserRepo interface {
	AddUserToChannel(ctx context.Context, channelID uint64, userID uint64) error
	GetUserByID(ctx context.Context, userID uint64) (*User, error)
	GetChannelUserIDs(ctx context.Context, channelID uint64) ([]uint64, error)
}

type MessageRepo interface {
	InsertMessage(ctx context.Context, msg *Message) error
	MarkMessageSeen(ctx context.Context, channelID, messageID uint64) error
	PublishMessage(ctx context.Context, msg *Message) error
	ListMessages(ctx context.Context, channelID uint64, pageStateBase64 string) ([]*Message, string, error)
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

type ChannelRepo interface {
	CreateChannel(ctx context.Context, channelID uint64) (*Channel, error)
	DeleteChannel(ctx context.Context, channelID uint64) error
	AssignRole(ctx context.Context, channelID, userID uint64, role Role) error
	GetRole(ctx context.Context, channelID, userID uint64) (Role, error)
}

type ForwardRepo interface {
	RegisterChannelSession(ctx context.Context, channelID, userID uint64, subscriber string) error
	RemoveChannelSession(ctx context.Context, channelID, userID uint64) error
}

type UserRepoImpl struct {
	s       *gocql.Session
	getUser endpoint.Endpoint
}

func NewUserRepoImpl(s *gocql.Session, userConn *UserClientConn) *UserRepoImpl {
	return &UserRepoImpl{
		s: s,
		getUser: transport.NewGrpcEndpoint(
			userConn.Conn,
			"user",
			"user.UserService",
			"GetUser",
			&userpb.GetUserResponse{},
		),
	}
}
func (repo *UserRepoImpl) AddUserToChannel(ctx context.Context, channelID uint64, userID uint64) error {
	if err := repo.s.Query("INSERT INTO channels (id, user_id) VALUES (?, ?)",
		channelID, userID).WithContext(ctx).Exec(); err != nil {
		return err
	}
	return nil
}
func (repo *UserRepoImpl) GetUserByID(ctx context.Context, userID uint64) (*User, error) {
	res, err := repo.getUser(ctx, &userpb.GetUserRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, err
	}
	pbUser := res.(*userpb.GetUserResponse)
	if !pbUser.Exist {
		return nil, ErrUserNotFound
	}
	return &User{
		ID:   pbUser.User.Id,
		Name: pbUser.User.Name,
	}, nil
}
func (repo *UserRepoImpl) GetChannelUserIDs(ctx context.Context, channelID uint64) ([]uint64, error) {
	iter := repo.s.Query("SELECT user_id FROM channels WHERE id = ?", channelID).WithContext(ctx).Idempotent(true).Iter()
	var userIDs []uint64
	var userID uint64
	for iter.Scan(&userID) {
		userIDs = append(userIDs, userID)
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return userIDs, nil
}

type MessageRepoImpl struct {
	s           *gocql.Session
	p           message.Publisher
	maxMessages int64
	pagination  int
}

func NewMessageRepoImpl(config *config.Config, s *gocql.Session, p message.Publisher) *MessageRepoImpl {
	return &MessageRepoImpl{s, p, config.Chat.Message.MaxNum, config.Chat.Message.PaginationNum}
}

func (repo *MessageRepoImpl) InsertMessage(ctx context.Context, msg *Message) error {
	var messageNum int64
	err := repo.s.Query("SELECT msgnum FROM chanmsg_counters WHERE channel_id = ? LIMIT 1", msg.ChannelID).
		WithContext(ctx).Idempotent(true).Scan(&messageNum)
	if err != nil {
		if err == gocql.ErrNotFound {
			messageNum = 0
		} else {
			return err
		}
	}
	if messageNum >= repo.maxMessages {
		return ErrExceedMessageNumLimits
	}
	if err := repo.s.Query("INSERT INTO messages (id, event, channel_id, user_id, payload, seen, timestamp, parent_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		msg.MessageID,
		msg.Event,
		msg.ChannelID,
		msg.UserID,
		msg.Payload,
		false,
		msg.Time,
		msg.ParentID).WithContext(ctx).Exec(); err != nil {
		return err
	}
	return repo.s.Query("UPDATE chanmsg_counters SET msgnum = msgnum + 1 WHERE channel_id = ?", msg.ChannelID).WithContext(ctx).Exec()
}
func (repo *MessageRepoImpl) MarkMessageSeen(ctx context.Context, channelID, messageID uint64) error {
	if err := repo.s.Query("UPDATE messages SET seen = ? WHERE channel_id = ? AND id = ?", true, channelID, messageID).
		WithContext(ctx).Idempotent(true).Exec(); err != nil {
		return err
	}
	return nil
}
func (repo *MessageRepoImpl) PublishMessage(ctx context.Context, msg *Message) error {
	return repo.p.Publish(MessagePubTopic, message.NewMessage(
		watermill.NewUUID(),
		msg.Encode(),
	))
}
func (repo *MessageRepoImpl) ListMessages(ctx context.Context, channelID uint64, pageStateBase64 string) ([]*Message, string, error) {
	var messages []*Message
	pageState, err := b64.URLEncoding.DecodeString(pageStateBase64)
	if err != nil {
		return nil, "", err
	}
	iter := repo.s.Query(`SELECT id, event, channel_id, user_id, payload, seen, timestamp, edited_at, deleted_for_all, deleted_by, parent_id FROM messages WHERE channel_id = ?`, channelID).
		WithContext(ctx).Idempotent(true).PageSize(repo.pagination).PageState(pageState).Iter()
	nextPageStateBase64 := b64.URLEncoding.EncodeToString(iter.PageState())
	scanner := iter.Scanner()

	for scanner.Next() {
		var message Message
		if err = scanner.Scan(
			&message.MessageID,
			&message.Event,
			&message.ChannelID,
			&message.UserID,
			&message.Payload,
			&message.Seen,
			&message.Time,
			&message.EditedAt,
			&message.DeletedForAll,
			&message.DeletedBy,
			&message.ParentID); err != nil {
			return nil, "", err
		}
		messages = append(messages, &message)
	}
	err = scanner.Err()
	if err != nil {
		return nil, "", err
	}

	for _, msg := range messages {
		if msg.ParentID > 0 {
			parent, err := repo.GetMessage(ctx, channelID, msg.ParentID)
			if err == nil && parent != nil {
				msg.ReplyPreview = &ReplyPreview{
					MessageID: strconv.FormatUint(parent.MessageID, 10),
					UserID:    strconv.FormatUint(parent.UserID, 10),
					Payload:   parent.Payload,
				}
			}
		}
	}

	return messages, nextPageStateBase64, nil
}
func (repo *MessageRepoImpl) EditMessage(ctx context.Context, channelID, messageID uint64, newPayload string, editedAt int64) error {
	if err := repo.s.Query("UPDATE messages SET payload = ?, edited_at = ? WHERE channel_id = ? AND id = ?",
		newPayload, editedAt, channelID, messageID).WithContext(ctx).Exec(); err != nil {
		return err
	}
	return nil
}
func (repo *MessageRepoImpl) DeleteMessageForAll(ctx context.Context, channelID, messageID, deletedBy uint64) error {
	if err := repo.s.Query("UPDATE messages SET deleted_for_all = ?, deleted_by = ?, payload = ? WHERE channel_id = ? AND id = ?",
		true, deletedBy, "", channelID, messageID).WithContext(ctx).Exec(); err != nil {
		return err
	}
	return nil
}
func (repo *MessageRepoImpl) GetMessage(ctx context.Context, channelID, messageID uint64) (*Message, error) {
	var msg Message
	err := repo.s.Query("SELECT id, event, channel_id, user_id, payload, seen, timestamp, edited_at, deleted_for_all, deleted_by FROM messages WHERE channel_id = ? AND id = ? LIMIT 1",
		channelID, messageID).WithContext(ctx).Idempotent(true).Scan(
		&msg.MessageID,
		&msg.Event,
		&msg.ChannelID,
		&msg.UserID,
		&msg.Payload,
		&msg.Seen,
		&msg.Time,
		&msg.EditedAt,
		&msg.DeletedForAll,
		&msg.DeletedBy)
	if err != nil {
		if err == gocql.ErrNotFound {
			return nil, ErrMessageNotFound
		}
		return nil, err
	}
	return &msg, nil
}
func (repo *MessageRepoImpl) AddReaction(ctx context.Context, channelID, messageID, userID uint64, emoji string) error {
	return repo.s.Query("INSERT INTO message_reactions (channel_id, message_id, emoji, user_id, created_at) VALUES (?, ?, ?, ?, ?)",
		channelID, messageID, emoji, userID, time.Now()).WithContext(ctx).Exec()
}
func (repo *MessageRepoImpl) RemoveReaction(ctx context.Context, channelID, messageID, userID uint64, emoji string) error {
	return repo.s.Query("DELETE FROM message_reactions WHERE channel_id = ? AND message_id = ? AND emoji = ? AND user_id = ?",
		channelID, messageID, emoji, userID).WithContext(ctx).Exec()
}
func (repo *MessageRepoImpl) GetReactions(ctx context.Context, channelID, messageID uint64) ([]ReactionSummary, error) {
	iter := repo.s.Query("SELECT emoji, user_id FROM message_reactions WHERE channel_id = ? AND message_id = ?",
		channelID, messageID).WithContext(ctx).Idempotent(true).Iter()
	emojiMap := make(map[string][]uint64)
	var emoji string
	var userID uint64
	for iter.Scan(&emoji, &userID) {
		emojiMap[emoji] = append(emojiMap[emoji], userID)
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	var summaries []ReactionSummary
	for e, users := range emojiMap {
		userStrs := make([]string, len(users))
		for i, u := range users {
			userStrs[i] = strconv.FormatUint(u, 10)
		}
		summaries = append(summaries, ReactionSummary{
			Emoji: e,
			Count: len(users),
			Users: userStrs,
		})
	}
	return summaries, nil
}
func (repo *MessageRepoImpl) PinMessage(ctx context.Context, channelID, messageID, pinnedBy uint64) error {
	return repo.s.Query("INSERT INTO pinned_messages (channel_id, pinned_at, message_id, pinned_by) VALUES (?, now(), ?, ?)",
		channelID, messageID, pinnedBy).WithContext(ctx).Exec()
}
func (repo *MessageRepoImpl) UnpinMessage(ctx context.Context, channelID, messageID uint64) error {
	iter := repo.s.Query("SELECT pinned_at FROM pinned_messages WHERE channel_id = ? ALLOW FILTERING",
		channelID).WithContext(ctx).Iter()
	var pinnedAt gocql.UUID
	for iter.Scan(&pinnedAt) {
		var mid uint64
		repo.s.Query("SELECT message_id FROM pinned_messages WHERE channel_id = ? AND pinned_at = ? LIMIT 1",
			channelID, pinnedAt).WithContext(ctx).Scan(&mid)
		if mid == messageID {
			return repo.s.Query("DELETE FROM pinned_messages WHERE channel_id = ? AND pinned_at = ?",
				channelID, pinnedAt).WithContext(ctx).Exec()
		}
	}
	return iter.Close()
}
func (repo *MessageRepoImpl) GetPinnedMessages(ctx context.Context, channelID uint64) ([]PinnedMessage, error) {
	iter := repo.s.Query("SELECT message_id, pinned_by, pinned_at FROM pinned_messages WHERE channel_id = ?",
		channelID).WithContext(ctx).Idempotent(true).Iter()
	var pins []PinnedMessage
	var msg PinnedMessage
	var pinnedAt gocql.UUID
	for iter.Scan(&msg.MessageID, &msg.PinnedBy, &pinnedAt) {
		msg.ChannelID = channelID
		msg.PinnedAt = pinnedAt.Time().UnixMilli()
		pins = append(pins, msg)
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return pins, nil
}
func (repo *MessageRepoImpl) SearchMessages(ctx context.Context, channelID uint64, query string, limit int) ([]*Message, error) {
	var messages []*Message
	iter := repo.s.Query("SELECT id, event, channel_id, user_id, payload, seen, timestamp, edited_at, deleted_for_all, deleted_by, parent_id FROM messages WHERE channel_id = ? ALLOW FILTERING", channelID).
		WithContext(ctx).Idempotent(true).Iter()
	scanner := iter.Scanner()
	count := 0
	for scanner.Next() && count < limit {
		var msg Message
		if err := scanner.Scan(&msg.MessageID, &msg.Event, &msg.ChannelID, &msg.UserID, &msg.Payload, &msg.Seen, &msg.Time, &msg.EditedAt, &msg.DeletedForAll, &msg.DeletedBy, &msg.ParentID); err != nil {
			return nil, err
		}
		if msg.DeletedForAll {
			continue
		}
		if msg.Event == EventText && containsIgnoreCase(msg.Payload, query) {
			messages = append(messages, &msg)
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return messages, nil
}
func (repo *MessageRepoImpl) ListMediaMessages(ctx context.Context, channelID uint64, mediaType string, limit int) ([]*Message, error) {
	var messages []*Message
	iter := repo.s.Query("SELECT id, event, channel_id, user_id, payload, seen, timestamp, edited_at, deleted_for_all, deleted_by, parent_id FROM messages WHERE channel_id = ? ALLOW FILTERING", channelID).
		WithContext(ctx).Idempotent(true).Iter()
	scanner := iter.Scanner()
	count := 0
	for scanner.Next() && count < limit {
		var msg Message
		if err := scanner.Scan(&msg.MessageID, &msg.Event, &msg.ChannelID, &msg.UserID, &msg.Payload, &msg.Seen, &msg.Time, &msg.EditedAt, &msg.DeletedForAll, &msg.DeletedBy, &msg.ParentID); err != nil {
			return nil, err
		}
		if msg.DeletedForAll || msg.Event != EventFile {
			continue
		}
		if mediaType == "" || mediaType == "all" {
			messages = append(messages, &msg)
			count++
			continue
		}
		ext := ""
		var fp struct {
			ObjectKey string `json:"object_key"`
		}
		if err := json.Unmarshal([]byte(msg.Payload), &fp); err == nil {
			parts := strings.Split(fp.ObjectKey, ".")
			if len(parts) > 1 {
				ext = strings.ToLower(parts[len(parts)-1])
			}
		}
		match := false
		switch mediaType {
		case "image":
			match = ext == "jpg" || ext == "jpeg" || ext == "png" || ext == "gif" || ext == "webp"
		case "video":
			match = ext == "mp4" || ext == "webm" || ext == "mov"
		case "audio":
			match = ext == "mp3" || ext == "wav" || ext == "ogg" || ext == "m4a"
		case "document":
			match = ext == "pdf" || ext == "doc" || ext == "docx" || ext == "txt" || ext == "xls" || ext == "xlsx"
		}
		if match {
			messages = append(messages, &msg)
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return messages, nil
}

type ChannelRepoImpl struct {
	s *gocql.Session
}

func NewChannelRepoImpl(s *gocql.Session) *ChannelRepoImpl {
	return &ChannelRepoImpl{s}
}

func (repo *ChannelRepoImpl) CreateChannel(ctx context.Context, channelID uint64) (*Channel, error) {
	if err := repo.s.Query("INSERT INTO channels (id, user_id) VALUES (?, ?)",
		channelID, 0).WithContext(ctx).Exec(); err != nil {
		return nil, err
	}
	accessToken, err := common.NewJWT(channelID)
	if err != nil {
		return nil, fmt.Errorf("error create JWT: %w", err)
	}
	return &Channel{
		ID:          channelID,
		AccessToken: accessToken,
	}, nil
}
func (repo *ChannelRepoImpl) DeleteChannel(ctx context.Context, channelID uint64) error {
	if err := repo.s.Query("DELETE FROM channels WHERE id = ?", channelID).
		WithContext(ctx).Exec(); err != nil {
		return err
	}
	return nil
}
func (repo *ChannelRepoImpl) AssignRole(ctx context.Context, channelID, userID uint64, role Role) error {
	return repo.s.Query("INSERT INTO channel_roles (channel_id, user_id, role, assigned_at) VALUES (?, ?, ?, ?)",
		channelID, userID, string(role), time.Now()).WithContext(ctx).Exec()
}
func (repo *ChannelRepoImpl) GetRole(ctx context.Context, channelID, userID uint64) (Role, error) {
	var role string
	err := repo.s.Query("SELECT role FROM channel_roles WHERE channel_id = ? AND user_id = ? LIMIT 1",
		channelID, userID).WithContext(ctx).Idempotent(true).Scan(&role)
	if err != nil {
		if err == gocql.ErrNotFound {
			return RoleMember, nil
		}
		return "", err
	}
	return Role(role), nil
}

type ForwardRepoImpl struct {
	registerChannelSession endpoint.Endpoint
	removeChannelSession   endpoint.Endpoint
}

func NewForwardRepoImpl(forwarderConn *ForwarderClientConn) *ForwardRepoImpl {
	return &ForwardRepoImpl{
		registerChannelSession: transport.NewGrpcEndpoint(
			forwarderConn.Conn,
			"forwarder",
			"forwarder.ForwardService",
			"RegisterChannelSession",
			&forwarderpb.RegisterChannelSessionResponse{},
		),
		removeChannelSession: transport.NewGrpcEndpoint(
			forwarderConn.Conn,
			"forwarder",
			"forwarder.ForwardService",
			"RemoveChannelSession",
			&forwarderpb.RemoveChannelSessionResponse{},
		),
	}
}

func (repo *ForwardRepoImpl) RegisterChannelSession(ctx context.Context, channelID, userID uint64, subscriber string) error {
	_, err := repo.registerChannelSession(ctx, &forwarderpb.RegisterChannelSessionRequest{
		ChannelId:  channelID,
		UserId:     userID,
		Subscriber: subscriber,
	})
	if err != nil {
		return err
	}
	return nil
}

func (repo *ForwardRepoImpl) RemoveChannelSession(ctx context.Context, channelID, userID uint64) error {
	_, err := repo.removeChannelSession(ctx, &forwarderpb.RemoveChannelSessionRequest{
		ChannelId: channelID,
		UserId:    userID,
	})
	if err != nil {
		return err
	}
	return nil
}
