package user

import (
	"time"

	notificationpkg "github.com/Tuananh165-art/NexusChat/pkg/notification"
)

type User struct {
	ID            uint64   `json:"id"`
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	Picture       string   `json:"picture"`
	AuthType      AuthType `json:"auth_type"`
	Username      string   `json:"username,omitempty"`
	Handle        string   `json:"handle,omitempty"`
	PasswordHash  string   `json:"password_hash,omitempty"`
	GoogleSubject string   `json:"google_subject,omitempty"`
}

type FriendRequest struct {
	FromUserID uint64 `json:"from_user_id"`
	ToUserID   uint64 `json:"to_user_id"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"created_at"`
}

type Notification struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	ActorID   uint64 `json:"actor_id"`
	Payload   string `json:"payload"`
	Read      bool   `json:"read"`
	CreatedAt int64  `json:"created_at"`
}

type AuthType string

const (
	LocalAuth  AuthType = "local"
	GoogleAuth AuthType = "google"
)

func (n *Notification) Intent(userID uint64) notificationpkg.Intent {
	return notificationpkg.Intent{ID: n.ID, UserID: userID, Type: n.Type, ActorID: n.ActorID, Payload: n.Payload, CreatedAt: time.UnixMilli(n.CreatedAt).UTC()}
}
