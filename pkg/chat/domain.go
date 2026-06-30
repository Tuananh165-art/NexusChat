package chat

import (
	"encoding/json"
	"strconv"
)

const (
	EventText = iota
	EventAction
	EventSeen
	EventFile
	EventEdit
	EventDelete
	EventReaction
	EventPin
)

type Action string

var (
	WaitingMessage   Action = "waiting"
	JoinedMessage    Action = "joined"
	IsTypingMessage  Action = "istyping"
	EndTypingMessage Action = "endtyping"
	OfflineMessage   Action = "offline"
	LeavedMessage    Action = "leaved"
)

type Message struct {
	MessageID      uint64 `json:"message_id"`
	Event          int    `json:"event"`
	ChannelID      uint64 `json:"channel_id"`
	UserID         uint64 `json:"user_id"`
	Payload        string `json:"payload"`
	Seen           bool   `json:"seen"`
	Time           int64  `json:"time"`
	EditedAt       int64  `json:"edited_at,omitempty"`
	DeletedForAll  bool   `json:"deleted_for_all,omitempty"`
	DeletedBy      uint64 `json:"deleted_by,omitempty"`
	Reactions      []ReactionSummary `json:"reactions,omitempty"`
	IsPinned       bool   `json:"is_pinned,omitempty"`
	ParentID       uint64 `json:"parent_id,omitempty"`
	ReplyPreview   *ReplyPreview     `json:"reply_preview,omitempty"`
}

type ReplyPreview struct {
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
	Payload   string `json:"payload"`
}

type Channel struct {
	ID          uint64
	AccessToken string
}

type User struct {
	ID   uint64
	Name string
}

type Reaction struct {
	ChannelID uint64 `json:"channel_id"`
	MessageID uint64 `json:"message_id"`
	Emoji     string `json:"emoji"`
	UserID    uint64 `json:"user_id"`
	CreatedAt int64  `json:"created_at"`
}

type PinnedMessage struct {
	ChannelID uint64 `json:"channel_id"`
	MessageID uint64 `json:"message_id"`
	PinnedBy  uint64 `json:"pinned_by"`
	PinnedAt  int64  `json:"pinned_at"`
}

type ReactionSummary struct {
	Emoji string   `json:"emoji"`
	Count int      `json:"count"`
	Users []string `json:"user_ids"`
}

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
)

type ChannelRole struct {
	ChannelID  uint64 `json:"channel_id"`
	UserID     uint64 `json:"user_id"`
	Role       Role   `json:"role"`
	AssignedAt int64  `json:"assigned_at"`
}

type Permission string

const (
	PermSendMessage    Permission = "send_message"
	PermDeleteAny      Permission = "delete_any_message"
	PermPinMessage     Permission = "pin_message"
	PermManageRoles    Permission = "manage_roles"
	PermUploadFile     Permission = "upload_file"
	PermReact          Permission = "react"
)

var RolePermissions = map[Role][]Permission{
	RoleOwner:  {PermSendMessage, PermDeleteAny, PermPinMessage, PermManageRoles, PermUploadFile, PermReact},
	RoleAdmin:  {PermSendMessage, PermDeleteAny, PermPinMessage, PermUploadFile, PermReact},
	RoleMember: {PermSendMessage, PermUploadFile, PermReact},
}

func HasPermission(role Role, perm Permission) bool {
	perms, ok := RolePermissions[role]
	if !ok {
		return false
	}
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

func (m *Message) Encode() []byte {
	result, _ := json.Marshal(m)
	return result
}

func (m *Message) ToPresenter() *MessagePresenter {
	reactions := []ReactionSummaryPresenter{}
	for _, r := range m.Reactions {
		reactions = append(reactions, ReactionSummaryPresenter{
			Emoji:   r.Emoji,
			Count:   r.Count,
			UserIDs: r.Users,
		})
	}
	var replyPreview *ReplyPreviewPresenter
	if m.ReplyPreview != nil {
		replyPreview = &ReplyPreviewPresenter{
			MessageID: m.ReplyPreview.MessageID,
			UserID:    m.ReplyPreview.UserID,
			Payload:   m.ReplyPreview.Payload,
		}
	}
	return &MessagePresenter{
		MessageID:     strconv.FormatUint(m.MessageID, 10),
		Event:         m.Event,
		UserID:        strconv.FormatUint(m.UserID, 10),
		Payload:       m.Payload,
		Seen:          m.Seen,
		Time:          m.Time,
		EditedAt:      m.EditedAt,
		DeletedForAll: m.DeletedForAll,
		DeletedBy:     strconv.FormatUint(m.DeletedBy, 10),
		Reactions:     reactions,
		IsPinned:      m.IsPinned,
		ParentID:      strconv.FormatUint(m.ParentID, 10),
		ReplyPreview:  replyPreview,
	}
}
