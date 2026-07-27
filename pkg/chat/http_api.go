package chat

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

type CreateRoomRequest struct {
	Name      string   `json:"name"`
	MemberIDs []string `json:"member_ids"`
}

type UpdateRoomRequest struct {
	Avatar string `json:"avatar"`
}

type JoinRoomRequest struct {
	InviteCode string `json:"invite_code"`
}

type DirectChatRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type RoomPresenter struct {
	ChannelID   string `json:"channel_id"`
	Name        string `json:"name"`
	Avatar      string `json:"avatar,omitempty"`
	OwnerID     string `json:"owner_id"`
	InviteCode  string `json:"invite_code"`
	MemberCount int    `json:"member_count"`
	Role        string `json:"role,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
	UpdatedAt   int64  `json:"updated_at,omitempty"`
}

type RoomsPresenter struct {
	Rooms []RoomPresenter `json:"rooms"`
}

type OpenRoomPresenter struct {
	ChannelID   string `json:"channel_id"`
	AccessToken string `json:"access_token"`
	Kind        string `json:"kind"`
	Title       string `json:"title,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
	PeerUserID  string `json:"peer_user_id,omitempty"`
	MemberCount int    `json:"member_count,omitempty"`
}

func (r *HttpServer) currentUserID(c *gin.Context) (uint64, error) {
	sid, err := sessionIDFromRequest(c.Request)
	if err != nil {
		return 0, err
	}
	return r.userSvc.GetUserIDBySession(c.Request.Context(), sid)
}

func sessionIDFromRequest(req *http.Request) (string, error) {
	cookie, err := req.Cookie(common.SessionIdCookieName)
	if err != nil {
		return "", err
	}
	sid, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(sid) == "" {
		return "", errors.New("empty session cookie")
	}
	return sid, nil
}

func (r *HttpServer) requireChannelMember() gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
		if !ok {
			response(c, http.StatusUnauthorized, common.ErrUnauthorized)
			return
		}
		userID, err := r.currentUserID(c)
		if err != nil || userID == 0 {
			response(c, http.StatusUnauthorized, common.ErrUnauthorized)
			return
		}
		exists, err := r.userSvc.IsChannelUserExist(c.Request.Context(), channelID, userID)
		if err != nil || !exists {
			response(c, http.StatusForbidden, common.ErrUnauthorized)
			return
		}
		if kind, kindErr := r.chanSvc.GetChannelKind(c.Request.Context(), channelID); kindErr == nil && kind == "direct" {
			peerID, peerErr := r.chanSvc.GetDirectPeer(c.Request.Context(), channelID, userID)
			friends, friendErr := r.userSvc.IsFriend(c.Request.Context(), userID, peerID)
			blocked, blockErr := usersBlocked(c.Request.Context(), userID, peerID)
			if peerErr != nil || friendErr != nil || !friends || blockErr != nil || blocked {
				response(c, http.StatusForbidden, common.ErrUnauthorized)
				return
			}
		}
		c.Next()
	}
}

func parseMemberIDs(values []string) ([]uint64, error) {
	members := make([]uint64, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		id, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return nil, err
		}
		members = append(members, id)
	}
	return members, nil
}

func roomPresenter(room Room) RoomPresenter {
	return RoomPresenter{
		ChannelID:   strconv.FormatUint(room.ChannelID, 10),
		Name:        room.Name,
		Avatar:      room.Avatar,
		OwnerID:     strconv.FormatUint(room.OwnerID, 10),
		InviteCode:  room.InviteCode,
		MemberCount: room.MemberCount,
		Role:        string(room.Role),
		CreatedAt:   room.CreatedAt.UnixMilli(),
		UpdatedAt:   room.UpdatedAt.UnixMilli(),
	}
}

func (r *HttpServer) ListRooms(c *gin.Context) {
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	rooms, err := r.chanSvc.ListRooms(c.Request.Context(), userID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	presenters := make([]RoomPresenter, 0, len(rooms))
	for _, room := range rooms {
		presenters = append(presenters, roomPresenter(room))
	}
	c.JSON(http.StatusOK, RoomsPresenter{Rooms: presenters})
}

func (r *HttpServer) CreateRoom(c *gin.Context) {
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	memberIDs, err := parseMemberIDs(req.MemberIDs)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	room, err := r.chanSvc.CreateRoom(c.Request.Context(), userID, req.Name, memberIDs)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusBadRequest, err)
		return
	}
	_ = r.msgSvc.BroadcastActionMessage(c.Request.Context(), room.ChannelID, userID, JoinedMessage)
	c.JSON(http.StatusCreated, roomPresenter(*room))
}

func (r *HttpServer) JoinRoom(c *gin.Context) {
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req JoinRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	room, err := r.chanSvc.JoinRoom(c.Request.Context(), userID, req.InviteCode)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusBadRequest, err)
		return
	}
	_ = r.msgSvc.BroadcastActionMessage(c.Request.Context(), room.ChannelID, userID, JoinedMessage)
	c.JSON(http.StatusOK, roomPresenter(*room))
}

func (r *HttpServer) OpenRoom(c *gin.Context) {
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	channelID, err := strconv.ParseUint(c.Param("channelId"), 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	channel, err := r.chanSvc.OpenRoom(c.Request.Context(), userID, channelID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusForbidden, err)
		return
	}
	c.JSON(http.StatusOK, OpenRoomPresenter{
		ChannelID:   strconv.FormatUint(channel.ID, 10),
		AccessToken: channel.AccessToken,
		Kind:        "group",
		Title:       channel.Room.Name,
		Avatar:      channel.Room.Avatar,
		MemberCount: channel.Room.MemberCount,
	})
}

func (r *HttpServer) UpdateRoom(c *gin.Context) {
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	channelID, err := strconv.ParseUint(c.Param("channelId"), 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	var req UpdateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	room, err := r.chanSvc.UpdateRoomAvatar(c.Request.Context(), userID, channelID, req.Avatar)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusForbidden, err)
		return
	}
	c.JSON(http.StatusOK, roomPresenter(*room))
}

func (r *HttpServer) LeaveRoom(c *gin.Context) {
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	channelID, err := strconv.ParseUint(c.Param("channelId"), 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	if err := r.chanSvc.LeaveRoom(c.Request.Context(), userID, channelID); err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusBadRequest, err)
		return
	}
	_ = r.msgSvc.BroadcastActionMessage(c.Request.Context(), channelID, userID, LeavedMessage)
	c.JSON(http.StatusOK, common.SuccessMessage{Message: "ok"})
}

// @Summary Start a chat
// @Description Websocket initialization endpoint for starting a chat
// @Tags chat
// @Produce json
// @Param uid query int true "user id"
// @Param access_token query string true "access token of the channel"
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat [get]
func (r *HttpServer) StartChat(c *gin.Context) {
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	if uid := strings.TrimSpace(c.Query("uid")); uid != "" {
		requestedUserID, parseErr := strconv.ParseUint(uid, 10, 64)
		if parseErr != nil || requestedUserID != userID {
			response(c, http.StatusUnauthorized, common.ErrUnauthorized)
			return
		}
	}
	_, err = r.userSvc.GetUser(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			response(c, http.StatusNotFound, ErrUserNotFound)
			return
		}
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}

	if strings.TrimSpace(c.Query("ticket")) == "" {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}

	if err := r.mc.HandleRequest(c.Writer, c.Request); err != nil {
		r.logger.Error("upgrade websocket error: " + err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
}

// @Summary Forward auth
// @Description Traefik forward auth endpoint for channel authentication
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Success 200 {none} nil
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/forwardauth [get]
func (r *HttpServer) ForwardAuth(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	c.Writer.Header().Set(common.ChannelIdHeader, strconv.FormatUint(channelID, 10))
	c.Status(http.StatusOK)
}

// @Summary Get channel users
// @Description Get all users of a channel
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Success 200 {object} UserIDsPresenter
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/users [get]
func (r *HttpServer) GetChannelUsers(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	userIDs, err := r.userSvc.GetChannelUserIDs(c.Request.Context(), channelID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	userIDsPresenter := []string{}
	for _, userID := range userIDs {
		userIDsPresenter = append(userIDsPresenter, strconv.FormatUint(userID, 10))
	}
	c.JSON(http.StatusOK, &UserIDsPresenter{
		UserIDs: userIDsPresenter,
	})
}

// @Summary Get online users
// @Description Get all online users of a channel
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Success 200 {object} UserIDsPresenter
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/users/online [get]
func (r *HttpServer) GetOnlineUsers(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	userIDs, err := r.userSvc.GetOnlineUserIDs(c.Request.Context(), channelID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	userIDsPresenter := []string{}
	for _, userID := range userIDs {
		userIDsPresenter = append(userIDsPresenter, strconv.FormatUint(userID, 10))
	}
	c.JSON(http.StatusOK, &UserIDsPresenter{
		UserIDs: userIDsPresenter,
	})
}

// @Summary List channel messages
// @Description List messages of a channel
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Param ps query string false "page state"
// @Success 200 {object} MessagesPresenter
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/channel/messages [get]
func (r *HttpServer) IssueWebSocketTicket(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok || channelID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	accessToken := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	if accessToken == "" {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	ticket, err := r.chanSvc.IssueWebSocketTicket(c.Request.Context(), userID, channelID, accessToken)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusForbidden, common.ErrUnauthorized)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": ticket, "expires_in": 60})
}

func (r *HttpServer) ListMessages(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	pageState := c.Query("ps")
	msgs, nextPageState, lastReadMessageID, err := r.msgSvc.ListMessages(c.Request.Context(), channelID, userID, pageState)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	msgsPresenter := []MessagePresenter{}
	for _, msg := range msgs {
		msgsPresenter = append(msgsPresenter, *msg.ToPresenter())
	}
	c.JSON(http.StatusOK, &MessagesPresenter{
		NextPageState:     nextPageState,
		Messages:          msgsPresenter,
		LastReadMessageID: strconv.FormatUint(lastReadMessageID, 10),
	})
}

func (r *HttpServer) GetReadState(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	messageID, err := r.msgSvc.GetLastReadMessageID(c.Request.Context(), channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"channel_id": strconv.FormatUint(channelID, 10), "user_id": strconv.FormatUint(userID, 10), "last_read_message_id": strconv.FormatUint(messageID, 10)})
}

func (r *HttpServer) MarkReadState(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var request struct {
		MessageID string `json:"message_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	messageID, err := strconv.ParseUint(strings.TrimSpace(request.MessageID), 10, 64)
	if err != nil || messageID == 0 {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	if err := r.msgSvc.MarkMessageRead(c.Request.Context(), channelID, userID, messageID); err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"channel_id": strconv.FormatUint(channelID, 10), "user_id": strconv.FormatUint(userID, 10), "last_read_message_id": strconv.FormatUint(messageID, 10)})
}

// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Param Cookie header string true "session cookie of the owner or admin"
// @Success 204 {object} common.SuccessMessage
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 404 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/channel [delete]
func (r *HttpServer) DeleteChannel(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	role, err := r.chanSvc.GetRole(c.Request.Context(), channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	if !HasPermission(role, PermManageRoles) {
		response(c, http.StatusForbidden, errors.New("only an owner or admin can delete a channel"))
		return
	}

	err = r.msgSvc.BroadcastActionMessage(c.Request.Context(), channelID, userID, LeavedMessage)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	err = r.chanSvc.DeleteChannel(c.Request.Context(), channelID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusNoContent, common.SuccessMessage{
		Message: "ok",
	})
}

// @Summary Get pinned messages
// @Description Get all pinned messages of a channel
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Success 200 {array} PinnedMessagePresenter
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/channel/pins [get]
func (r *HttpServer) GetPinnedMessages(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	pins, err := r.msgSvc.GetPinnedMessages(c.Request.Context(), channelID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	presenters := []PinnedMessagePresenter{}
	for _, p := range pins {
		presenters = append(presenters, PinnedMessagePresenter{
			MessageID: strconv.FormatUint(p.MessageID, 10),
			PinnedBy:  strconv.FormatUint(p.PinnedBy, 10),
			PinnedAt:  p.PinnedAt,
		})
	}
	c.JSON(http.StatusOK, presenters)
}

// @Summary Search messages
// @Description Search messages in a channel
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Param q query string true "search query"
// @Param limit query int false "limit"
// @Success 200 {object} MessagesPresenter
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/channel/search [get]
func (r *HttpServer) SearchMessages(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	query := c.Query("q")
	if query == "" {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	msgs, err := r.msgSvc.SearchMessages(c.Request.Context(), channelID, query, limit)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	msgsPresenter := []MessagePresenter{}
	for _, msg := range msgs {
		msgsPresenter = append(msgsPresenter, *msg.ToPresenter())
	}
	c.JSON(http.StatusOK, &MessagesPresenter{Messages: msgsPresenter})
}

// @Summary List media messages
// @Description List media messages in a channel filtered by type
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Param type query string false "media type (image, video, audio, document, all)"
// @Param limit query int false "limit"
// @Success 200 {object} MessagesPresenter
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/channel/media [get]
func (r *HttpServer) ListMediaMessages(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	mediaType := c.DefaultQuery("type", "all")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	msgs, err := r.msgSvc.ListMediaMessages(c.Request.Context(), channelID, mediaType, limit)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	msgsPresenter := []MessagePresenter{}
	for _, msg := range msgs {
		msgsPresenter = append(msgsPresenter, *msg.ToPresenter())
	}
	c.JSON(http.StatusOK, &MessagesPresenter{Messages: msgsPresenter})
}

// @Summary Get my role
// @Description Get the role of the authenticated user in the channel
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Success 200 {object} map[string]string
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/role [get]
func (r *HttpServer) GetMyRole(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	role, err := r.chanSvc.GetRole(c.Request.Context(), channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"role": string(role)})
}

// @Summary Assign role
// @Description Assign a role to a user in the channel
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Param uid query string true "user id to assign role"
// @Param role query string true "role to assign (owner, admin, member)"
// @Success 200 {object} common.SuccessMessage
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 403 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/role [put]
func (r *HttpServer) AssignRole(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	adminUserID, err := r.currentUserID(c)
	if err != nil || adminUserID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	adminRole, err := r.chanSvc.GetRole(c.Request.Context(), channelID, adminUserID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	if !HasPermission(adminRole, PermManageRoles) {
		response(c, http.StatusForbidden, errors.New("insufficient permissions"))
		return
	}
	targetUID := c.Query("uid")
	targetUserID, err := strconv.ParseUint(targetUID, 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	roleStr := c.Query("role")
	role := Role(roleStr)
	if role != RoleOwner && role != RoleAdmin && role != RoleMember {
		response(c, http.StatusBadRequest, errors.New("invalid role"))
		return
	}
	if err := r.chanSvc.AssignRole(c.Request.Context(), channelID, targetUserID, role); err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, common.SuccessMessage{Message: "ok"})
}

// @Summary Rewrite text with AI
// @Description Forward a rewrite request to the independent AI service
// @Tags chat
// @Accept json
// @Produce json
// @param Authorization header string true "channel authorization"
// @Param request body AIRewriteRequest true "rewrite request"
// @Success 200 {object} AIRewriteResponse
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 502 {object} common.ErrResponse
// @Router /chat/ai/rewrite [post]
func (r *HttpServer) RewriteWithAI(c *gin.Context) {
	if _, ok := c.Request.Context().Value(common.ChannelKey).(uint64); !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req AIRewriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	result, err := r.aiSvc.Rewrite(c.Request.Context(), &req)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (r *HttpServer) HandleChatOnConnect(sess *melody.Session) {
	ctx := context.Background()
	reject := func(reason string) {
		r.logger.Error(reason)
		_ = sess.Close()
	}
	ticket := strings.TrimSpace(sess.Request.URL.Query().Get("ticket"))
	userID, channelID, accessToken, err := r.chanSvc.ConsumeWebSocketTicket(ctx, ticket)
	if err != nil || userID == 0 || channelID == 0 {
		reject("websocket ticket invalid or already used")
		return
	}
	sid, err := sessionIDFromRequest(sess.Request)
	if err != nil {
		reject("websocket session cookie missing")
		return
	}
	sessionUserID, err := r.userSvc.GetUserIDBySession(ctx, sid)
	if err != nil || sessionUserID == 0 || sessionUserID != userID {
		reject("websocket session does not match ticket")
		return
	}
	if raw := strings.TrimSpace(sess.Request.URL.Query().Get("uid")); raw != "" {
		requested, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil || requested != userID {
			reject("websocket user id does not match session")
			return
		}
	}
	authResult, err := common.Auth(&common.AuthPayload{AccessToken: accessToken})
	if err != nil || authResult.Expired || authResult.ChannelID != channelID {
		reject("websocket channel ticket token invalid")
		return
	}

	exists, err := r.userSvc.IsChannelUserExist(ctx, channelID, userID)
	if err != nil || !exists {
		reject("websocket user is not a channel member")
		return
	}
	kind, err := r.chanSvc.GetChannelKind(ctx, channelID)
	if err != nil {
		reject("websocket channel metadata unavailable")
		return
	}
	if kind == "direct" {
		peerID, peerErr := r.chanSvc.GetDirectPeer(ctx, channelID, userID)
		friends, friendErr := r.userSvc.IsFriend(ctx, userID, peerID)
		blocked, blockErr := usersBlocked(ctx, userID, peerID)
		if peerErr != nil || friendErr != nil || !friends || blockErr != nil || blocked {
			reject("websocket direct chat is no longer authorized")
			return
		}
	}
	existingRole, _ := r.chanSvc.GetRole(ctx, channelID, userID)
	if existingRole == RoleMember {
		onlineUserIDs, _ := r.userSvc.GetOnlineUserIDs(ctx, channelID)
		if len(onlineUserIDs) <= 1 {
			_ = r.chanSvc.AssignRole(ctx, channelID, userID, RoleOwner)
		}
	}
	if err := r.initializeChatSession(sess, channelID, userID); err != nil {
		reject(err.Error())
		return
	}
	if err := r.msgSvc.BroadcastConnectMessage(ctx, channelID, userID); err != nil {
		r.logger.Error(err.Error())
	}
}

func (r *HttpServer) initializeChatSession(sess *melody.Session, channelID, userID uint64) error {
	ctx := context.Background()
	firstSession := r.acquireSession(channelID, userID)
	if firstSession {
		if err := r.userSvc.AddOnlineUser(ctx, channelID, userID); err != nil {
			r.releaseSession(channelID, userID)
			return err
		}
		if err := r.forwardSvc.RegisterChannelSession(ctx, channelID, userID, r.msgSubscriber.subscriberID); err != nil {
			r.releaseSession(channelID, userID)
			return err
		}
	}
	sess.Set(sessCidKey, channelID)
	sess.Set(sessUidKey, userID)
	return nil
}

func (r *HttpServer) acquireSession(channelID, userID uint64) bool {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	key := strconv.FormatUint(channelID, 10) + ":" + strconv.FormatUint(userID, 10)
	count := r.sessionCounts[key]
	r.sessionCounts[key] = count + 1
	return count == 0
}

func (r *HttpServer) releaseSession(channelID, userID uint64) bool {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	key := strconv.FormatUint(channelID, 10) + ":" + strconv.FormatUint(userID, 10)
	count := r.sessionCounts[key]
	if count <= 1 {
		delete(r.sessionCounts, key)
		return true
	}
	r.sessionCounts[key] = count - 1
	return false
}

func (r *HttpServer) HandleChatOnMessage(sess *melody.Session, data []byte) {
	msgPresenter, err := DecodeToMessagePresenter(data)
	if err != nil {
		r.logger.Error(err.Error())
		return
	}
	channelID, channelOK := sess.Get(sessCidKey)
	userID, userOK := sess.Get(sessUidKey)
	if !channelOK || !userOK {
		return
	}
	channel, channelTypeOK := channelID.(uint64)
	user, userTypeOK := userID.(uint64)
	if !channelTypeOK || !userTypeOK || channel == 0 || user == 0 {
		return
	}
	// The authenticated connection owns both identity fields. Do not parse a
	// client-supplied channel token or user id for each message.
	msg := &Message{Event: msgPresenter.Event, ChannelID: channel, UserID: user, Payload: msgPresenter.Payload, Time: msgPresenter.Time}
	if err := r.msgSvc.AuthorizeInteraction(context.Background(), msg.ChannelID, msg.UserID); err != nil {
		r.logger.Error(err.Error())
		return
	}
	switch msg.Event {
	case EventText:
		parentID := uint64(0)
		if msgPresenter.ParentID != "" {
			parentID, _ = strconv.ParseUint(msgPresenter.ParentID, 10, 64)
		}
		if err := r.msgSvc.BroadcastTextMessage(context.Background(), msg.ChannelID, msg.UserID, msg.Payload, parentID); err != nil {
			r.logger.Error(err.Error())
		}
	case EventAction:
		if err := r.msgSvc.BroadcastActionMessage(context.Background(), msg.ChannelID, msg.UserID, Action(msg.Payload)); err != nil {
			r.logger.Error(err.Error())
		}
	case EventFile:
		if err := r.msgSvc.BroadcastFileMessage(context.Background(), msg.ChannelID, msg.UserID, msg.Payload); err != nil {
			r.logger.Error(err.Error())
		}
	case EventEdit:
		parts := splitEditPayload(msg.Payload)
		if parts == nil {
			r.logger.Error("invalid edit payload format")
			return
		}
		messageID, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			r.logger.Error(err.Error())
			return
		}
		if err := r.msgSvc.EditMessage(context.Background(), msg.ChannelID, msg.UserID, messageID, parts[1]); err != nil {
			r.logger.Error(err.Error())
		}
	case EventDelete:
		messageID, err := strconv.ParseUint(msg.Payload, 10, 64)
		if err != nil {
			r.logger.Error(err.Error())
			return
		}
		if err := r.msgSvc.DeleteMessageForAll(context.Background(), msg.ChannelID, msg.UserID, messageID); err != nil {
			r.logger.Error(err.Error())
		}
	case EventReaction:
		parts := splitReactionPayload(msg.Payload)
		if parts == nil {
			r.logger.Error("invalid reaction payload format")
			return
		}
		messageID, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			r.logger.Error(err.Error())
			return
		}
		emoji := parts[1]
		action := parts[2]
		if action == "add" {
			if err := r.msgSvc.AddReaction(context.Background(), msg.ChannelID, msg.UserID, messageID, emoji); err != nil {
				r.logger.Error(err.Error())
			}
		} else if action == "remove" {
			if err := r.msgSvc.RemoveReaction(context.Background(), msg.ChannelID, msg.UserID, messageID, emoji); err != nil {
				r.logger.Error(err.Error())
			}
		}
	case EventPin:
		parts := splitPinPayload(msg.Payload)
		if parts == nil {
			r.logger.Error("invalid pin payload format")
			return
		}
		messageID, err := strconv.ParseUint(parts[0], 10, 64)
		if err != nil {
			r.logger.Error(err.Error())
			return
		}
		action := parts[1]
		if action == "pin" {
			if err := r.msgSvc.PinMessage(context.Background(), msg.ChannelID, msg.UserID, messageID); err != nil {
				r.logger.Error(err.Error())
			}
		} else if action == "unpin" {
			if err := r.msgSvc.UnpinMessage(context.Background(), msg.ChannelID, messageID); err != nil {
				r.logger.Error(err.Error())
			}
		}
	default:
		r.logger.Error("invailid event type: " + strconv.Itoa(msg.Event))
	}
}

func (r *HttpServer) HandleChatOnClose(sess *melody.Session, i int, s string) error {
	uidValue, uidOK := sess.Get(sessUidKey)
	channelValue, channelOK := sess.Get(sessCidKey)
	if !uidOK || !channelOK {
		return nil
	}
	userID, ok := uidValue.(uint64)
	if !ok {
		return nil
	}
	channelID, ok := channelValue.(uint64)
	if !ok {
		return nil
	}
	lastSession := r.releaseSession(channelID, userID)
	if lastSession {
		if err := r.userSvc.DeleteOnlineUser(context.Background(), channelID, userID); err != nil {
			r.logger.Error(err.Error())
			return err
		}
		// Keep the forwarder registration. Another replica/device may still own
		// a live connection for the same user; the subscriber filters stale sessions.
		return r.msgSvc.BroadcastActionMessage(context.Background(), channelID, userID, OfflineMessage)
	}
	return nil
}

func (r *HttpServer) CreateDirectChat(c *gin.Context) {
	userID, err := r.currentUserID(c)
	if err != nil || userID == 0 {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	var req DirectChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	targetUserID, err := strconv.ParseUint(strings.TrimSpace(req.UserID), 10, 64)
	if err != nil || targetUserID == 0 || targetUserID == userID {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	channel, err := r.chanSvc.CreateDirectChannel(c.Request.Context(), userID, targetUserID)
	if err != nil {
		r.logger.Error(err.Error())
		if errors.Is(err, ErrUserNotFound) {
			response(c, http.StatusNotFound, ErrUserNotFound)
			return
		}
		if errors.Is(err, ErrDirectChatRequiresFriend) {
			response(c, http.StatusForbidden, err)
			return
		}
		response(c, http.StatusBadRequest, err)
		return
	}
	c.JSON(http.StatusOK, OpenRoomPresenter{
		ChannelID:   strconv.FormatUint(channel.ID, 10),
		AccessToken: channel.AccessToken,
		Kind:        "direct",
		PeerUserID:  strconv.FormatUint(targetUserID, 10),
	})
}
