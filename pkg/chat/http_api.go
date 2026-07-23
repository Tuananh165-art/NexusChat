package chat

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

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
	uid := c.Query("uid")
	userID, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
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

	accessToken := c.Query("access_token")
	authResult, err := common.Auth(&common.AuthPayload{
		AccessToken: accessToken,
	})
	if err != nil {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	if authResult.Expired {
		r.logger.Error(common.ErrTokenExpired.Error())
		response(c, http.StatusUnauthorized, common.ErrTokenExpired)
	}
	channelID := authResult.ChannelID
	exist, err := r.userSvc.IsChannelUserExist(c.Request.Context(), channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	if !exist {
		response(c, http.StatusNotFound, ErrChannelOrUserNotFound)
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
func (r *HttpServer) ListMessages(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	pageState := c.Query("ps")
	msgs, nextPageState, err := r.msgSvc.ListMessages(c.Request.Context(), channelID, pageState)
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
		NextPageState: nextPageState,
		Messages:      msgsPresenter,
	})
}

// @Summary Delete channel
// @Description Delete a channel
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Param delby query string true "id of the user that performs the deletion"
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
	uid := c.Query("delby")
	userID, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}

	exist, err := r.userSvc.IsChannelUserExist(c.Request.Context(), channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	if !exist {
		response(c, http.StatusBadRequest, ErrChannelOrUserNotFound)
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
	uid := c.Query("uid")
	userID, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
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
	adminUID := c.Query("admin")
	adminUserID, err := strconv.ParseUint(adminUID, 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
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

// @Summary Get notification preferences
// @Description Get notification preferences for the authenticated user
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Success 200 {object} map[string]string
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/notification/prefs [get]
func (r *HttpServer) GetNotificationPrefs(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	uid := c.Query("uid")
	userID, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	pref, err := r.userSvc.GetNotificationPref(c.Request.Context(), channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		response(c, http.StatusInternalServerError, common.ErrServer)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pref": pref})
}

// @Summary Set notification preferences
// @Description Set notification preferences for the authenticated user
// @Tags chat
// @Produce json
// @param Authorization header string true "channel authorization"
// @Param uid query string true "user id"
// @Param pref query string true "preference (all, mentions, mute)"
// @Success 200 {object} common.SuccessMessage
// @Failure 400 {object} common.ErrResponse
// @Failure 401 {object} common.ErrResponse
// @Failure 500 {object} common.ErrResponse
// @Router /chat/notification/prefs [put]
func (r *HttpServer) SetNotificationPrefs(c *gin.Context) {
	channelID, ok := c.Request.Context().Value(common.ChannelKey).(uint64)
	if !ok {
		response(c, http.StatusUnauthorized, common.ErrUnauthorized)
		return
	}
	uid := c.Query("uid")
	userID, err := strconv.ParseUint(uid, 10, 64)
	if err != nil {
		response(c, http.StatusBadRequest, common.ErrInvalidParam)
		return
	}
	pref := c.Query("pref")
	if pref != "all" && pref != "mentions" && pref != "mute" {
		response(c, http.StatusBadRequest, errors.New("invalid preference; use all, mentions, or mute"))
		return
	}
	if err := r.userSvc.SetNotificationPref(c.Request.Context(), channelID, userID, pref); err != nil {
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
	userID, err := strconv.ParseUint(sess.Request.URL.Query().Get("uid"), 10, 64)
	if err != nil {
		r.logger.Error(err.Error())
		return
	}
	accessToken := sess.Request.URL.Query().Get("access_token")
	authResult, err := common.Auth(&common.AuthPayload{
		AccessToken: accessToken,
	})
	if err != nil {
		r.logger.Error(err.Error())
		return
	}
	if authResult.Expired {
		r.logger.Error(common.ErrTokenExpired.Error())
		return
	}
	channelID := authResult.ChannelID
	existingRole, _ := r.chanSvc.GetRole(context.Background(), channelID, userID)
	if existingRole == RoleMember {
		onlineUserIDs, _ := r.userSvc.GetOnlineUserIDs(context.Background(), channelID)
		if len(onlineUserIDs) <= 1 {
			r.chanSvc.AssignRole(context.Background(), channelID, userID, RoleOwner)
		}
	}
	err = r.initializeChatSession(sess, channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		return
	}
	if err := r.msgSvc.BroadcastConnectMessage(context.Background(), channelID, userID); err != nil {
		r.logger.Error(err.Error())
		return
	}
}

func (r *HttpServer) initializeChatSession(sess *melody.Session, channelID, userID uint64) error {
	ctx := context.Background()
	if err := r.userSvc.AddOnlineUser(ctx, channelID, userID); err != nil {
		return err
	}
	if err := r.forwardSvc.RegisterChannelSession(ctx, channelID, userID, r.msgSubscriber.subscriberID); err != nil {
		return err
	}
	sess.Set(sessCidKey, channelID)
	return nil
}

func (r *HttpServer) HandleChatOnMessage(sess *melody.Session, data []byte) {
	msgPresenter, err := DecodeToMessagePresenter(data)
	if err != nil {
		r.logger.Error(err.Error())
		return
	}
	msg, err := msgPresenter.ToMessage(sess.Request.URL.Query().Get("access_token"))
	if err != nil {
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
	case EventSeen:
		messageID, err := strconv.ParseUint(msg.Payload, 10, 64)
		if err != nil {
			r.logger.Error(err.Error())
			return
		}
		if err := r.msgSvc.MarkMessageSeen(context.Background(), msg.ChannelID, msg.UserID, messageID); err != nil {
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
	userID, err := strconv.ParseUint(sess.Request.URL.Query().Get("uid"), 10, 64)
	if err != nil {
		r.logger.Error(err.Error())
		return err
	}
	accessToken := sess.Request.URL.Query().Get("access_token")
	authResult, err := common.Auth(&common.AuthPayload{
		AccessToken: accessToken,
	})
	if err != nil {
		r.logger.Error(err.Error())
		return err
	}
	if authResult.Expired {
		r.logger.Error(common.ErrTokenExpired.Error())
		return common.ErrTokenExpired
	}
	channelID := authResult.ChannelID
	err = r.userSvc.DeleteOnlineUser(context.Background(), channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		return err
	}
	err = r.forwardSvc.RemoveChannelSession(context.Background(), channelID, userID)
	if err != nil {
		r.logger.Error(err.Error())
		return err
	}
	return r.msgSvc.BroadcastActionMessage(context.Background(), channelID, userID, OfflineMessage)
}
