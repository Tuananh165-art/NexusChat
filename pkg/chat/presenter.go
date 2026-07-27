package chat

import (
	"encoding/json"
	"strconv"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
)

type MessagePresenter struct {
	MessageID     string                     `json:"message_id"`
	Event         int                        `json:"event"`
	UserID        string                     `json:"user_id"`
	Payload       string                     `json:"payload"`
	Time          int64                      `json:"time"`
	EditedAt      int64                      `json:"edited_at,omitempty"`
	DeletedForAll bool                       `json:"deleted_for_all,omitempty"`
	DeletedBy     string                     `json:"deleted_by,omitempty"`
	Reactions     []ReactionSummaryPresenter `json:"reactions,omitempty"`
	IsPinned      bool                       `json:"is_pinned,omitempty"`
	ParentID      string                     `json:"parent_id,omitempty"`
	ReplyPreview  *ReplyPreviewPresenter     `json:"reply_preview,omitempty"`
}

type ReplyPreviewPresenter struct {
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
	Payload   string `json:"payload"`
}

type ReactionSummaryPresenter struct {
	Emoji   string   `json:"emoji"`
	Count   int      `json:"count"`
	UserIDs []string `json:"user_ids"`
}

type PinnedMessagePresenter struct {
	MessageID string `json:"message_id"`
	PinnedBy  string `json:"pinned_by"`
	PinnedAt  int64  `json:"pinned_at"`
}

type UserPresenter struct {
	ID   string `json:"id"`
	Name string `json:"name" binding:"required"`
}

type UserIDsPresenter struct {
	UserIDs []string `json:"user_ids"`
}

type MessagesPresenter struct {
	NextPageState     string             `json:"next_ps"`
	LastReadMessageID string             `json:"last_read_message_id"`
	Messages          []MessagePresenter `json:"messages"`
}

func (m *MessagePresenter) Encode() []byte {
	result, _ := json.Marshal(m)
	return result
}

func (m *MessagePresenter) ToMessage(accessToken string) (*Message, error) {
	authResult, err := common.Auth(&common.AuthPayload{
		AccessToken: accessToken,
	})
	if err != nil {
		return nil, err
	}
	if authResult.Expired {
		return nil, common.ErrTokenExpired
	}
	channelID := authResult.ChannelID
	userID, err := strconv.ParseUint(m.UserID, 10, 64)
	if err != nil {
		return nil, err
	}
	return &Message{
		Event:     m.Event,
		ChannelID: channelID,
		UserID:    userID,
		Payload:   m.Payload,
		Time:      m.Time,
	}, nil
}
