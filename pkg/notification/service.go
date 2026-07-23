package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/config"
	"github.com/Tuananh165-art/NexusChat/pkg/infra"
	"github.com/Tuananh165-art/NexusChat/pkg/realtime"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type Notification struct {
	ID        string         `json:"id"`
	UserID    uint64         `json:"user_id,string"`
	ChannelID uint64         `json:"channel_id,string,omitempty"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Body      string         `json:"body"`
	Data      map[string]any `json:"data,omitempty"`
	Read      bool           `json:"read"`
	CreatedAt time.Time      `json:"created_at"`
}

type PushSubscription struct {
	Endpoint string `json:"endpoint" binding:"required"`
	Keys     struct {
		P256dh string `json:"p256dh" binding:"required"`
		Auth   string `json:"auth" binding:"required"`
	} `json:"keys" binding:"required"`
}

type Service struct {
	config     *config.Config
	redis      redis.UniversalClient
	cassandra  *gocql.Session
	events     *realtime.EventBus
	hub        *realtime.Hub
	pushQueue  chan Notification
	httpServer *http.Server
	grpcServer *grpc.Server
	cancel     context.CancelFunc
}

func NewService(cfg *config.Config) (*Service, error) {
	common.JwtSecret = cfg.Chat.JWT.Secret
	common.JwtExpirationSecond = cfg.Chat.JWT.ExpirationSecond
	redisClient, err := infra.NewRedisClient(cfg)
	if err != nil {
		return nil, err
	}
	session, err := infra.NewCassandraSession(cfg)
	if err != nil {
		_ = redisClient.Close()
		return nil, err
	}
	events, err := realtime.NewEventBus(common.GetServerAddrs(cfg.Kafka.Addrs), cfg.Kafka.Version)
	if err != nil {
		session.Close()
		_ = redisClient.Close()
		return nil, err
	}
	return &Service{config: cfg, redis: redisClient, cassandra: session, events: events, hub: realtime.NewHub(), pushQueue: make(chan Notification, 256)}, nil
}

func (s *Service) Create(ctx context.Context, item Notification, eventID string) (Notification, error) {
	if item.ID == "" {
		item.ID = uuid.NewString()
	}
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	if item.Data == nil {
		item.Data = map[string]any{}
	}
	dedupeApplied := false
	if eventID != "" {
		applied, err := s.cassandra.Query(
			"INSERT INTO processed_events (service, event_id, processed_at) VALUES (?, ?, ?) USING TTL ? IF NOT EXISTS",
			"notification", eventID, item.CreatedAt, 7*24*3600,
		).WithContext(ctx).ScanCAS()
		if err != nil {
			return Notification{}, err
		}
		if !applied {
			return item, nil
		}
		dedupeApplied = true
	}
	data, err := json.Marshal(item.Data)
	if err != nil {
		s.releaseDedupe(ctx, eventID, dedupeApplied)
		return Notification{}, err
	}
	retention := s.config.Notification.RetentionDay * 24 * 3600
	err = s.cassandra.Query(
		"INSERT INTO notifications_by_user (user_id, created_at, notification_id, channel_id, type, title, body, data, is_read) VALUES (?, now(), ?, ?, ?, ?, ?, ?, false) USING TTL ?",
		item.UserID, item.ID, item.ChannelID, item.Type, item.Title, item.Body, string(data), retention,
	).WithContext(ctx).Exec()
	if err != nil {
		s.releaseDedupe(ctx, eventID, dedupeApplied)
		return Notification{}, err
	}
	_ = s.redis.Incr(ctx, fmt.Sprintf("notification:unread:%d", item.UserID)).Err()
	_ = s.redis.Expire(ctx, fmt.Sprintf("notification:unread:%d", item.UserID), 24*time.Hour).Err()
	delivered := s.hub.Send(item.UserID, gin.H{"type": "notification.created", "data": item})
	if !delivered {
		s.enqueuePush(item)
	}
	event, _ := realtime.NewEvent("notification.created", "notification-service", strconv.FormatUint(item.UserID, 10), item)
	publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := realtime.PublishDurably(publishCtx, s.cassandra, s.events, "notification", realtime.NotificationEventsTopic, event); err != nil {
		slog.Error("publish notification.created", "error", err)
	}
	return item, nil
}

func (s *Service) releaseDedupe(ctx context.Context, eventID string, applied bool) {
	if !applied || eventID == "" {
		return
	}
	_ = s.cassandra.Query("DELETE FROM processed_events WHERE service = ? AND event_id = ?", "notification", eventID).WithContext(ctx).Exec()
}

func (s *Service) enqueuePush(item Notification) {
	select {
	case s.pushQueue <- item:
	default:
		slog.Warn("drop web push because queue is full", "user_id", item.UserID)
	}
}

func (s *Service) HandleEvent(ctx context.Context, event realtime.Event) error {
	switch event.EventType {
	case "chat.message.created":
		var payload struct {
			ChannelID uint64 `json:"channel_id"`
			UserID    uint64 `json:"user_id"`
			Payload   string `json:"payload"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		iter := s.cassandra.Query("SELECT user_id FROM channels WHERE id = ?", payload.ChannelID).WithContext(ctx).Iter()
		var target uint64
		for iter.Scan(&target) {
			if target == payload.UserID {
				continue
			}
			_, err := s.Create(ctx, Notification{
				UserID: target, ChannelID: payload.ChannelID, Type: "message",
				Title: "New message", Body: truncate(payload.Payload, 160),
				Data: map[string]any{"sender_id": strconv.FormatUint(payload.UserID, 10)},
			}, event.EventID+":"+strconv.FormatUint(target, 10))
			if err != nil {
				_ = iter.Close()
				return err
			}
		}
		return iter.Close()
	case "call.ringing":
		var payload struct {
			CallID       string `json:"call_id"`
			ChannelID    uint64 `json:"channel_id"`
			CallerID     uint64 `json:"caller_id"`
			TargetUserID uint64 `json:"target_user_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		_, err := s.Create(ctx, Notification{
			UserID: payload.TargetUserID, ChannelID: payload.ChannelID,
			Type: "call", Title: "Incoming call", Body: "A contact is calling you",
			Data: map[string]any{"call_id": payload.CallID, "caller_id": strconv.FormatUint(payload.CallerID, 10)},
		}, event.EventID)
		return err
	}
	return nil
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func (s *Service) list(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	iter := s.cassandra.Query(
		"SELECT notification_id, channel_id, type, title, body, data, is_read, dateOf(created_at) FROM notifications_by_user WHERE user_id = ? LIMIT ?",
		identity.UserID, limit,
	).WithContext(c.Request.Context()).Iter()
	items := make([]Notification, 0, limit)
	var item Notification
	var data string
	for iter.Scan(&item.ID, &item.ChannelID, &item.Type, &item.Title, &item.Body, &data, &item.Read, &item.CreatedAt) {
		item.UserID = identity.UserID
		item.Data = map[string]any{}
		_ = json.Unmarshal([]byte(data), &item.Data)
		items = append(items, item)
	}
	if err := iter.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to list notifications"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"notifications": items})
}

func (s *Service) unreadCount(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	count, err := s.redis.Get(c.Request.Context(), fmt.Sprintf("notification:unread:%d", identity.UserID)).Int64()
	if err != nil && err != redis.Nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to read count"})
		return
	}
	if err == redis.Nil {
		iter := s.cassandra.Query("SELECT is_read FROM notifications_by_user WHERE user_id = ? LIMIT 1000", identity.UserID).
			WithContext(c.Request.Context()).Iter()
		var read bool
		for iter.Scan(&read) {
			if !read {
				count++
			}
		}
		_ = iter.Close()
		_ = s.redis.Set(c.Request.Context(), fmt.Sprintf("notification:unread:%d", identity.UserID), count, 24*time.Hour).Err()
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (s *Service) markRead(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	id := c.Param("id")
	var createdAt gocql.UUID
	err := s.cassandra.Query(
		"SELECT created_at FROM notifications_by_user WHERE user_id = ? AND notification_id = ? ALLOW FILTERING",
		identity.UserID, id,
	).WithContext(c.Request.Context()).Scan(&createdAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "notification not found"})
		return
	}
	if err := s.cassandra.Query(
		"UPDATE notifications_by_user SET is_read = true WHERE user_id = ? AND created_at = ? AND notification_id = ?",
		identity.UserID, createdAt, id,
	).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to mark notification"})
		return
	}
	_ = s.redis.Decr(c.Request.Context(), fmt.Sprintf("notification:unread:%d", identity.UserID)).Err()
	c.Status(http.StatusNoContent)
}

func (s *Service) readAll(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	iter := s.cassandra.Query(
		"SELECT created_at, notification_id, is_read FROM notifications_by_user WHERE user_id = ?",
		identity.UserID,
	).WithContext(c.Request.Context()).Iter()
	var createdAt gocql.UUID
	var id string
	var read bool
	for iter.Scan(&createdAt, &id, &read) {
		if !read {
			_ = s.cassandra.Query(
				"UPDATE notifications_by_user SET is_read = true WHERE user_id = ? AND created_at = ? AND notification_id = ?",
				identity.UserID, createdAt, id,
			).WithContext(c.Request.Context()).Exec()
		}
	}
	_ = iter.Close()
	_ = s.redis.Set(c.Request.Context(), fmt.Sprintf("notification:unread:%d", identity.UserID), 0, 24*time.Hour).Err()
	c.Status(http.StatusNoContent)
}

func (s *Service) subscribe(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request PushSubscription
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid push subscription"})
		return
	}
	if err := s.cassandra.Query(
		"INSERT INTO push_subscriptions_by_user (user_id, endpoint, p256dh, auth, created_at) VALUES (?, ?, ?, ?, ?)",
		identity.UserID, request.Endpoint, request.Keys.P256dh, request.Keys.Auth, time.Now().UTC(),
	).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to save subscription"})
		return
	}
	c.Status(http.StatusCreated)
}

func (s *Service) publicKey(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"public_key": s.config.Notification.VAPID.PublicKey})
}

func (s *Service) unsubscribe(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request struct {
		Endpoint string `json:"endpoint" binding:"required"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid endpoint"})
		return
	}
	if err := s.cassandra.Query(
		"DELETE FROM push_subscriptions_by_user WHERE user_id = ? AND endpoint = ?",
		identity.UserID, request.Endpoint,
	).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to remove subscription"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) preferences(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	channelID, _ := strconv.ParseUint(c.DefaultQuery("channel_id", strconv.FormatUint(identity.ChannelID, 10)), 10, 64)
	var preference string
	err := s.cassandra.Query(
		"SELECT preference FROM notification_preferences WHERE user_id = ? AND channel_id = ?",
		identity.UserID, channelID,
	).WithContext(c.Request.Context()).Scan(&preference)
	if err == gocql.ErrNotFound {
		preference = "all"
		err = nil
	}
	if err != nil {
		slog.Warn("read notification preferences failed; fallback to all", "error", err, "user_id", identity.UserID, "channel_id", channelID)
		preference = "all"
	}
	c.JSON(http.StatusOK, gin.H{"pref": preference})
}

func (s *Service) updatePreferences(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	channelID, _ := strconv.ParseUint(c.DefaultQuery("channel_id", strconv.FormatUint(identity.ChannelID, 10)), 10, 64)
	preference := c.Query("pref")
	if preference != "all" && preference != "mentions" && preference != "mute" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid preference"})
		return
	}
	if err := s.cassandra.Query(
		"INSERT INTO notification_preferences (user_id, channel_id, preference, updated_at) VALUES (?, ?, ?, ?)",
		identity.UserID, channelID, preference, time.Now().UTC(),
	).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to update preferences"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) sendPush(ctx context.Context, item Notification) {
	cfg := s.config.Notification.VAPID
	if cfg.PublicKey == "" || cfg.PrivateKey == "" {
		return
	}
	if response, err := realtime.CallStructRPC(
		ctx,
		s.config.Notification.Grpc.Client.Presence.Endpoint,
		"nexuschat.presence.v1.PresenceService",
		"GetPresence",
		map[string]any{"user_id": strconv.FormatUint(item.UserID, 10)},
	); err == nil && realtime.StructBool(response, "online") {
		return
	}
	body, _ := json.Marshal(item)
	iter := s.cassandra.Query(
		"SELECT endpoint, p256dh, auth FROM push_subscriptions_by_user WHERE user_id = ?",
		item.UserID,
	).WithContext(ctx).Iter()
	var endpoint, p256dh, auth string
	for iter.Scan(&endpoint, &p256dh, &auth) {
		subscription := &webpush.Subscription{
			Endpoint: endpoint,
			Keys:     webpush.Keys{P256dh: p256dh, Auth: auth},
		}
		response, err := webpush.SendNotification(body, subscription, &webpush.Options{
			Subscriber: cfg.Subject, VAPIDPublicKey: cfg.PublicKey, VAPIDPrivateKey: cfg.PrivateKey, TTL: 60,
		})
		if err != nil {
			slog.Error("send web push", "error", err)
			continue
		}
		_ = response.Body.Close()
		if response.StatusCode == http.StatusGone || response.StatusCode == http.StatusNotFound {
			_ = s.cassandra.Query(
				"DELETE FROM push_subscriptions_by_user WHERE user_id = ? AND endpoint = ?",
				item.UserID, endpoint,
			).WithContext(ctx).Exec()
		}
	}
	_ = iter.Close()
}

func (s *Service) runPushWorkers(ctx context.Context, workers int) {
	for i := 0; i < workers; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case item := <-s.pushQueue:
					pushCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
					s.sendPush(pushCtx, item)
					cancel()
				}
			}
		}()
	}
}

func (s *Service) websocket(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	conn, err := websocketUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &realtime.Client{UserID: identity.UserID, Device: c.DefaultQuery("device_id", "browser"), Conn: conn, Send: make(chan []byte, 64)}
	s.hub.Add(client)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for message := range client.Send {
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
	s.hub.Remove(client)
	close(client.Send)
	_ = conn.Close()
	<-done
}

var websocketUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (s *Service) routes() *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery(), common.CorsMiddleware())
	engine.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	engine.GET("/ready", func(c *gin.Context) {
		if err := s.redis.Ping(c.Request.Context()).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
	group := engine.Group("/api/notifications")
	group.Use(realtime.RequireIdentity(s.cassandra))
	group.GET("", s.list)
	group.GET("/unread-count", s.unreadCount)
	group.PUT("/:id/read", s.markRead)
	group.PUT("/read-all", s.readAll)
	group.GET("/preferences", s.preferences)
	group.PUT("/preferences", s.updatePreferences)
	group.POST("/push-subscriptions", s.subscribe)
	group.DELETE("/push-subscriptions", s.unsubscribe)
	group.GET("/ws", s.websocket)
	engine.GET("/api/notifications/push-public-key", s.publicKey)
	return engine
}

func (s *Service) grpcMethods() map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error) {
	return map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error){
		"CreateNotification": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			fields := request.GetFields()
			userID, _ := strconv.ParseUint(fields["user_id"].GetStringValue(), 10, 64)
			channelID, _ := strconv.ParseUint(fields["channel_id"].GetStringValue(), 10, 64)
			item, err := s.Create(ctx, Notification{
				UserID: userID, ChannelID: channelID,
				Type: fields["type"].GetStringValue(), Title: fields["title"].GetStringValue(), Body: fields["body"].GetStringValue(),
			}, fields["event_id"].GetStringValue())
			if err != nil {
				return nil, err
			}
			body, _ := json.Marshal(item)
			var response map[string]any
			_ = json.Unmarshal(body, &response)
			return structpb.NewStruct(response)
		},
		"GetUnreadCount": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			userID, _ := strconv.ParseUint(request.GetFields()["user_id"].GetStringValue(), 10, 64)
			count, err := s.redis.Get(ctx, fmt.Sprintf("notification:unread:%d", userID)).Int64()
			if err == redis.Nil {
				count = 0
				err = nil
			}
			return structpb.NewStruct(map[string]any{"count": count})
		},
		"GetPreferences": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			fields := request.GetFields()
			userID, _ := strconv.ParseUint(fields["user_id"].GetStringValue(), 10, 64)
			channelID, _ := strconv.ParseUint(fields["channel_id"].GetStringValue(), 10, 64)
			var preference string
			err := s.cassandra.Query("SELECT preference FROM notification_preferences WHERE user_id = ? AND channel_id = ?", userID, channelID).WithContext(ctx).Scan(&preference)
			if err == gocql.ErrNotFound {
				preference = "all"
				err = nil
			}
			if err != nil {
				return nil, err
			}
			return structpb.NewStruct(map[string]any{"preference": preference})
		},
		"UpdatePreferences": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			fields := request.GetFields()
			userID, _ := strconv.ParseUint(fields["user_id"].GetStringValue(), 10, 64)
			channelID, _ := strconv.ParseUint(fields["channel_id"].GetStringValue(), 10, 64)
			preference := fields["preference"].GetStringValue()
			if err := s.cassandra.Query("INSERT INTO notification_preferences (user_id, channel_id, preference, updated_at) VALUES (?, ?, ?, ?)", userID, channelID, preference, time.Now().UTC()).WithContext(ctx).Exec(); err != nil {
				return nil, err
			}
			return structpb.NewStruct(map[string]any{"preference": preference})
		},
	}
}

func (s *Service) Run(ctx context.Context) error {
	if err := common.NewObservabilityInjector(s.config).Register("notification"); err != nil {
		return err
	}
	consumeCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go realtime.RelayOutbox(consumeCtx, s.cassandra, s.events, "notification")
	s.runPushWorkers(consumeCtx, 4)
	go func() {
		if err := s.events.Consume(
			consumeCtx, "notification-service-v1",
			[]string{realtime.ChatEventsTopic, realtime.CallEventsTopic}, 4, s.HandleEvent,
		); err != nil && consumeCtx.Err() == nil {
			slog.Error("notification consumer stopped", "error", err)
		}
	}()
	s.httpServer = &http.Server{
		Addr: ":" + s.config.Notification.Http.Server.Port, Handler: common.NewOtelHttpHandler(s.routes(), "notification_http"),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.grpcServer = realtime.NewGRPCServer("nexuschat.notification.v1.NotificationService", s.grpcMethods())
	errorsCh := make(chan error, 2)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()
	go func() { errorsCh <- realtime.ServeGRPC(s.grpcServer, s.config.Notification.Grpc.Server.Port) }()
	select {
	case <-ctx.Done():
		return s.Close(context.Background())
	case err := <-errorsCh:
		_ = s.Close(context.Background())
		return err
	}
}

func (s *Service) Close(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var joined error
	if s.httpServer != nil {
		joined = errors.Join(joined, s.httpServer.Shutdown(timeoutCtx))
	}
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	s.hub.Close()
	joined = errors.Join(joined, s.events.Close())
	s.cassandra.Close()
	joined = errors.Join(joined, s.redis.Close())
	return joined
}

func Main() {
	cfg, err := config.NewConfig()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	service, err := NewService(cfg)
	if err != nil {
		slog.Error("initialize notification", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := service.Run(ctx); err != nil {
		slog.Error("notification stopped", "error", err)
		os.Exit(1)
	}
}
