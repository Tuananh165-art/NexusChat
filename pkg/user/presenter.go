package user

type CreateLocalUserRequest struct {
	Name string `json:"name" binding:"required"`
}

type SignupRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	DisplayName string `json:"display_name"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type AuthResponse struct {
	User UserPresenter `json:"user"`
}

type UpdateUserProfileRequest struct {
	Name    string `json:"name" binding:"required"`
	Picture string `json:"picture"`
}

type GetUserRequest struct {
	Uid string `form:"uid" binding:"required"`
}

type UserPresenter struct {
	ID       string `json:"id"`
	Username string `json:"username,omitempty"`
	Handle   string `json:"handle,omitempty"`
	Name     string `json:"name"`
	Picture  string `json:"picture"`
}

type UserSearchPresenter struct {
	Users []UserPresenter `json:"users"`
}

type FriendRequestPresenter struct {
	FromUserID string `json:"from_user_id"`
	ToUserID   string `json:"to_user_id"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"created_at"`
}

type CreateFriendRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type NotificationPresenter struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	ActorID   string `json:"actor_id"`
	Payload   string `json:"payload"`
	Read      bool   `json:"read"`
	CreatedAt int64  `json:"created_at"`
}

type GoogleUserPresenter struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}
