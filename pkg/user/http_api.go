package user

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/gin-gonic/gin"
)

// @Summary Create a local user
// @Description Register a new local user
// @Tags user
// @Produce json
// @Param user body CreateLocalUserRequest true "new user"
// @Success 201 {object} UserPresenter
// @Failure 400 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /user [post]
func (r *HttpServer) CreateLocalUser(c *gin.Context) {
	var req CreateLocalUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > 30 {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	user, err := r.userSvc.CreateUser(c.Request.Context(), &User{Name: name, AuthType: LocalAuth})
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	if err := r.loginUser(c, user, http.StatusCreated); err != nil {
		r.logger.Error(err.Error())
	}
}

func (r *HttpServer) Signup(c *gin.Context) {
	var req SignupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	user, err := r.userSvc.Signup(c.Request.Context(), req.Username, req.Password, req.DisplayName)
	if err != nil {
		switch {
		case errors.Is(err, ErrUsernameTaken):
			response(c, http.StatusConflict, err)
		case errors.Is(err, ErrHandleTaken), errors.Is(err, ErrInvalidUsername), errors.Is(err, ErrInvalidPassword), errors.Is(err, ErrInvalidDisplayName):
			response(c, http.StatusBadRequest, err)
		default:
			r.logger.Error(err.Error())
			response(c, http.StatusInternalServerError, common.ErrServer)
		}
		return
	}
	_ = r.loginUser(c, user, http.StatusCreated)
}

func (r *HttpServer) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	user, err := r.userSvc.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		response(c, http.StatusUnauthorized, ErrInvalidCredentials)
		return
	}
	_ = r.loginUser(c, user, http.StatusOK)
}

func (r *HttpServer) Logout(c *gin.Context) {
	sid, _ := common.GetCookie(c, common.SessionIdCookieName)
	if err := r.userSvc.Logout(c.Request.Context(), sid); err != nil {
		r.logger.Error(err.Error())
	}
	common.ClearCookie(c, common.SessionIdCookieName, r.authCookieConfig.Path, r.authCookieConfig.Domain, r.authCookieConfig.Secure, r.authCookieConfig.HttpOnly, r.authCookieConfig.SameSite)
	c.Status(http.StatusNoContent)
}

func (r *HttpServer) loginUser(c *gin.Context, user *User, status int) error {
	sid, err := r.userSvc.SetUserSession(c.Request.Context(), user.ID)
	if err != nil {
		response(c, http.StatusInternalServerError, common.ErrServer)
		return err
	}
	common.SetAuthCookie(c, sid, r.authCookieConfig.MaxAge, r.authCookieConfig.Path, r.authCookieConfig.Domain, r.authCookieConfig.Secure, r.authCookieConfig.HttpOnly, r.authCookieConfig.SameSite)
	c.JSON(status, AuthResponse{User: userPresenter(user)})
	return nil
}

func userPresenter(user *User) UserPresenter {
	return UserPresenter{ID: strconv.FormatUint(user.ID, 10), Username: user.Username, Handle: user.Handle, Name: user.Name, Picture: user.Picture}
}

// @Summary Update self user profile
// @Description Update self user display name and avatar picture
// @Tags user
// @Produce json
// @Param Cookie header string true "session id cookie"
// @Param user body UpdateUserProfileRequest true "updated user profile"
// @Success 200 {object} UserPresenter
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /user/me [put]
func (r *HttpServer) UpdateUserMe(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var updateReq UpdateUserProfileRequest
	if err := c.ShouldBindJSON(&updateReq); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	name := strings.TrimSpace(updateReq.Name)
	if name == "" || len([]rune(name)) > 30 || len(updateReq.Picture) > 1024*1024 {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	user, err := r.userSvc.UpdateUserProfile(c.Request.Context(), userID, name, updateReq.Picture)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response(c, http.StatusNotFound, ErrUserNotFound)
			return
		}
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, userPresenter(user))
}

// @Summary Get user
// @Description Get user information
// @Tags user
// @Produce json
// @Param uid query string true "target user id"
// @Param Cookie header string true "session id cookie"
// @Success 200 {object} UserPresenter
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /user [get]
func (r *HttpServer) GetUser(c *gin.Context) {
	if _, ok := c.Request.Context().Value(common.UserKey).(uint64); !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req GetUserRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	userID, err := strconv.ParseUint(req.Uid, 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	user, err := r.userSvc.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response(c, http.StatusNotFound, ErrUserNotFound)
			return
		}
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, userPresenter(user))
}

// @Summary Get self user
// @Description Get self user information
// @Tags user
// @Produce json
// @Param Cookie header string true "session id cookie"
// @Success 200 {object} UserPresenter
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /user/me [get]
func (r *HttpServer) GetUserMe(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	user, err := r.userSvc.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response(c, http.StatusNotFound, ErrUserNotFound)
			return
		}
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, userPresenter(user))
}

// @Summary OAuth Google login
// @Description OAuth Google login endpoint
// @Tags user
// @Success 307
// @Router /user/oauth2/google/login [get]
func (r *HttpServer) OAuthGoogleLogin(c *gin.Context) {
	if strings.TrimSpace(r.googleOauthConfig.ClientID) == "" || strings.TrimSpace(r.googleOauthConfig.ClientSecret) == "" {
		r.oauthFailure(c, "google_not_configured")
		return
	}
	oauthState, err := common.GenerateStateOauthCookie(c, r.oauthCookieConfig.MaxAge, r.oauthCookieConfig.Path, r.oauthCookieConfig.Domain, r.oauthCookieConfig.Secure, r.oauthCookieConfig.HttpOnly, r.oauthCookieConfig.SameSite)
	if err != nil {
		r.logger.Error(err.Error())
		r.oauthFailure(c, "oauth_state_failed")
		return
	}
	c.Redirect(http.StatusTemporaryRedirect, r.googleOauthConfig.AuthCodeURL(oauthState))
}

func (r *HttpServer) OAuthGoogleCallback(c *gin.Context) {
	defer common.ClearCookie(c, common.OAuthStateCookieName, r.oauthCookieConfig.Path, r.oauthCookieConfig.Domain, r.oauthCookieConfig.Secure, r.oauthCookieConfig.HttpOnly, r.oauthCookieConfig.SameSite)
	fail := func(code string) { r.oauthFailure(c, code) }
	oauthState, err := common.GetCookie(c, common.OAuthStateCookieName)
	if err != nil {
		r.logger.Error(err.Error())
		fail("missing_oauth_state")
		return
	}
	if c.Query("state") == "" || c.Query("state") != oauthState {
		r.logger.Error("invalid oauth google state")
		fail("invalid_oauth_state")
		return
	}
	if providerError := c.Query("error"); providerError != "" {
		fail("google_" + providerError)
		return
	}
	code := c.Query("code")
	if code == "" {
		fail("missing_google_code")
		return
	}
	token, err := r.googleOauthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		r.logger.Error("google code exchange: " + err.Error())
		fail("google_code_exchange_failed")
		return
	}
	googleUser, err := r.userSvc.GetGoogleUser(c.Request.Context(), token.AccessToken)
	if err != nil {
		r.logger.Error(err.Error())
		fail("google_profile_failed")
		return
	}
	user, err := r.userSvc.GetOrCreateUserByOAuth(c.Request.Context(), &User{Email: googleUser.Email, Name: googleUser.Name, Picture: googleUser.Picture, AuthType: GoogleAuth, GoogleSubject: googleUser.Subject})
	if err != nil {
		r.logger.Error(err.Error())
		fail("google_account_failed")
		return
	}
	sid, err := r.userSvc.SetUserSession(c.Request.Context(), user.ID)
	if err != nil {
		r.logger.Error(err.Error())
		fail("session_failed")
		return
	}
	common.SetAuthCookie(c, sid, r.authCookieConfig.MaxAge, r.authCookieConfig.Path, r.authCookieConfig.Domain, r.authCookieConfig.Secure, r.authCookieConfig.HttpOnly, r.authCookieConfig.SameSite)
	c.Redirect(http.StatusTemporaryRedirect, "/")
}

func (r *HttpServer) oauthFailure(c *gin.Context, code string) {
	c.Redirect(http.StatusTemporaryRedirect, "/?oauth_error="+code)
}

func (r *HttpServer) SearchUsers(c *gin.Context) {
	requesterID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	query := strings.TrimSpace(c.Query("q"))
	if query == "" || len([]rune(query)) > 50 {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}
	users, err := r.userSvc.SearchUsers(c.Request.Context(), requesterID, query, limit)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	presenter := UserSearchPresenter{Users: make([]UserPresenter, 0, len(users))}
	for _, item := range users {
		presenter.Users = append(presenter.Users, userPresenter(item))
	}
	c.JSON(http.StatusOK, presenter)
}

func (r *HttpServer) CreateFriendRequest(c *gin.Context) {
	fromUserID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req CreateFriendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	toUserID, err := strconv.ParseUint(strings.TrimSpace(req.UserID), 10, 64)
	if err != nil || toUserID == 0 {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	request, err := r.userSvc.SendFriendRequest(c.Request.Context(), fromUserID, toUserID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrUserNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, ErrAlreadyFriends) || errors.Is(err, ErrFriendRequestExists) {
			status = http.StatusConflict
		}
		if errors.Is(err, ErrBlocked) || errors.Is(err, ErrSafetyUnavailable) {
			status = http.StatusForbidden
		}
		if errors.Is(err, ErrNotificationDelivery) {
			status = http.StatusServiceUnavailable
		}
		response(c, status, err)
		return
	}
	c.JSON(http.StatusCreated, friendRequestPresenter(*request))
}

func (r *HttpServer) ListFriendRequests(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	requests, err := r.userSvc.GetFriendRequests(c.Request.Context(), userID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	result := make([]FriendRequestPresenter, 0, len(requests))
	for _, request := range requests {
		result = append(result, friendRequestPresenter(*request))
	}
	c.JSON(http.StatusOK, gin.H{"requests": result})
}

func (r *HttpServer) GetFriendshipStatus(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	peerID, err := strconv.ParseUint(strings.TrimSpace(c.Param("userId")), 10, 64)
	if err != nil || peerID == 0 || peerID == userID {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	status, err := r.userSvc.GetRelationshipStatus(c.Request.Context(), userID, peerID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": status, "accepted": status == "accepted"})
}

func (r *HttpServer) AcceptFriendRequest(c *gin.Context) {
	r.updateFriendRequest(c, true)
}

func (r *HttpServer) DeclineFriendRequest(c *gin.Context) {
	r.updateFriendRequest(c, false)
}

func (r *HttpServer) updateFriendRequest(c *gin.Context, accept bool) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	fromUserID, err := strconv.ParseUint(c.Param("fromUserId"), 10, 64)
	if err != nil || fromUserID == 0 {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	if accept {
		err = r.userSvc.AcceptFriendRequest(c.Request.Context(), userID, fromUserID)
	} else {
		err = r.userSvc.DeclineFriendRequest(c.Request.Context(), userID, fromUserID)
	}
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrFriendRequestNotFound) {
			status = http.StatusNotFound
		}
		if errors.Is(err, ErrNotificationDelivery) {
			status = http.StatusServiceUnavailable
		}
		response(c, status, err)
		return
	}
	c.JSON(http.StatusOK, common.SuccessMessage{Message: "ok"})
}

func friendRequestPresenter(request FriendRequest) FriendRequestPresenter {
	return FriendRequestPresenter{
		FromUserID: strconv.FormatUint(request.FromUserID, 10),
		ToUserID:   strconv.FormatUint(request.ToUserID, 10),
		Status:     request.Status,
		CreatedAt:  request.CreatedAt,
	}
}

func (r *HttpServer) ListFriends(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	friends, err := r.userSvc.ListFriends(c.Request.Context(), userID)
	if err != nil {
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	ids := make([]string, 0, len(friends))
	for _, id := range friends {
		ids = append(ids, strconv.FormatUint(id, 10))
	}
	c.JSON(http.StatusOK, gin.H{"friend_ids": ids})
}

func (r *HttpServer) CancelFriendRequest(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	toUserID, err := strconv.ParseUint(c.Param("toUserId"), 10, 64)
	if err != nil || toUserID == 0 || toUserID == userID {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	if err := r.userSvc.CancelFriendRequest(c.Request.Context(), userID, toUserID); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrFriendRequestNotFound) {
			status = http.StatusNotFound
		}
		response(c, status, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (r *HttpServer) Unfriend(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	friendID, err := strconv.ParseUint(c.Param("friendId"), 10, 64)
	if err != nil || friendID == 0 || friendID == userID {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	if err := r.userSvc.RemoveFriend(c.Request.Context(), userID, friendID); err != nil {
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.Status(http.StatusNoContent)
}

func (r *HttpServer) ListNotifications(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	notifications, err := r.userSvc.GetNotifications(c.Request.Context(), userID)
	if err != nil {
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	result := make([]NotificationPresenter, 0, len(notifications))
	for _, notification := range notifications {
		result = append(result, NotificationPresenter{ID: notification.ID, Type: notification.Type, ActorID: strconv.FormatUint(notification.ActorID, 10), Payload: notification.Payload, Read: notification.Read, CreatedAt: notification.CreatedAt})
	}
	c.JSON(http.StatusOK, gin.H{"notifications": result})
}

func (r *HttpServer) MarkNotificationRead(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	if err := r.userSvc.MarkNotificationRead(c.Request.Context(), userID, c.Param("notificationId")); err != nil {
		if errors.Is(err, ErrNotificationNotFound) {
			response(c, http.StatusNotFound, err)
			return
		}
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.Status(http.StatusNoContent)
}

func (r *HttpServer) MarkAllNotificationsRead(c *gin.Context) {
	userID, ok := c.Request.Context().Value(common.UserKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	if err := r.userSvc.MarkAllNotificationsRead(c.Request.Context(), userID); err != nil {
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.Status(http.StatusNoContent)
}
