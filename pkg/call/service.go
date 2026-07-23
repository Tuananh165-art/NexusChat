package call

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

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

type State string

const (
	StateRinging   State = "ringing"
	StateAccepted  State = "accepted"
	StateConnected State = "connected"
	StateEnded     State = "ended"
	StateRejected  State = "rejected"
	StateMissed    State = "missed"
	StateFailed    State = "failed"
)

type Call struct {
	ID         string     `json:"id"`
	ChannelID  uint64     `json:"channel_id,string"`
	CallerID   uint64     `json:"caller_id,string"`
	CalleeID   uint64     `json:"callee_id,string"`
	State      State      `json:"state"`
	Media      string     `json:"media"`
	CreatedAt  time.Time  `json:"created_at"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	EndReason  string     `json:"end_reason,omitempty"`
}

type Signal struct {
	Type         string          `json:"type"`
	CallID       string          `json:"call_id"`
	TargetUserID uint64          `json:"target_user_id,string,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
}

type Service struct {
	config     *config.Config
	redis      redis.UniversalClient
	cassandra  *gocql.Session
	events     *realtime.EventBus
	hub        *realtime.Hub
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
	return &Service{config: cfg, redis: redisClient, cassandra: session, events: events, hub: realtime.NewHub()}, nil
}

func (s *Service) callKey(id string) string         { return "call:" + id }
func (s *Service) userCallKey(userID uint64) string { return fmt.Sprintf("call:user:%d", userID) }

func (s *Service) authorizeMember(ctx context.Context, channelID, userID uint64) error {
	if response, err := realtime.CallStructRPC(
		ctx,
		s.config.Call.Grpc.Client.Chat.Endpoint,
		"nexuschat.chat.v1.AuthorizationService",
		"AuthorizeChannelMember",
		map[string]any{"channel_id": strconv.FormatUint(channelID, 10), "user_id": strconv.FormatUint(userID, 10)},
	); err == nil && realtime.StructBool(response, "authorized") {
		return nil
	}
	// One-release fallback keeps calls available while older chat pods roll out.
	var member uint64
	err := s.cassandra.Query(
		"SELECT user_id FROM channels WHERE id = ? AND user_id = ? LIMIT 1",
		channelID, userID,
	).WithContext(ctx).Scan(&member)
	if err != nil || member != userID {
		return errors.New("user is not a channel member")
	}
	return nil
}

func (s *Service) Create(ctx context.Context, channelID, callerID, calleeID uint64, media string) (Call, error) {
	if callerID == calleeID {
		return Call{}, errors.New("cannot call yourself")
	}
	if media != "audio" && media != "video" {
		media = "video"
	}
	if err := s.authorizeMember(ctx, channelID, callerID); err != nil {
		return Call{}, err
	}
	if err := s.authorizeMember(ctx, channelID, calleeID); err != nil {
		return Call{}, err
	}
	call := Call{
		ID: uuid.NewString(), ChannelID: channelID, CallerID: callerID, CalleeID: calleeID,
		State: StateRinging, Media: media, CreatedAt: time.Now().UTC(),
	}
	presence, presenceErr := realtime.CallStructRPC(
		ctx,
		s.config.Call.Grpc.Client.Presence.Endpoint,
		"nexuschat.presence.v1.PresenceService",
		"GetPresence",
		map[string]any{"user_id": strconv.FormatUint(calleeID, 10)},
	)
	if presenceErr != nil {
		slog.Debug("presence lookup unavailable; call will still ring", "error", presenceErr)
	}
	ttl := time.Duration(s.config.Call.ActiveTTLSecond) * time.Second
	callerLocked, err := s.redis.SetNX(ctx, s.userCallKey(callerID), call.ID, ttl).Result()
	if err != nil {
		return Call{}, err
	}
	if !callerLocked {
		return Call{}, errors.New("caller is busy")
	}
	calleeLocked, err := s.redis.SetNX(ctx, s.userCallKey(calleeID), call.ID, ttl).Result()
	if err != nil || !calleeLocked {
		_ = s.redis.Del(ctx, s.userCallKey(callerID)).Err()
		if err != nil {
			return Call{}, err
		}
		return Call{}, errors.New("callee is busy")
	}
	body, _ := json.Marshal(call)
	pipe := s.redis.TxPipeline()
	pipe.Set(ctx, s.callKey(call.ID), body, ttl)
	deadline := call.CreatedAt.Add(time.Duration(s.config.Call.RingTimeoutSecond) * time.Second)
	pipe.ZAdd(ctx, "calls:ringing:deadlines", redis.Z{Score: float64(deadline.Unix()), Member: call.ID})
	if _, err := pipe.Exec(ctx); err != nil {
		_ = s.releaseLocks(ctx, call)
		return Call{}, err
	}
	if err := s.persist(ctx, call); err != nil {
		_ = s.releaseLocks(ctx, call)
		return Call{}, err
	}
	payload := map[string]any{
		"call_id": call.ID, "channel_id": call.ChannelID, "caller_id": call.CallerID,
		"target_user_id": call.CalleeID, "media": call.Media,
	}
	if presence != nil {
		payload["callee_online"] = realtime.StructBool(presence, "online")
	}
	if err := s.publish(ctx, "call.ringing", call.ID, payload); err != nil {
		slog.Error("publish call.ringing", "error", err)
	}
	s.hub.Send(calleeID, gin.H{"type": "call.invite", "data": call})
	return call, nil
}

func (s *Service) Get(ctx context.Context, callID string) (Call, error) {
	body, err := s.redis.Get(ctx, s.callKey(callID)).Bytes()
	if err != nil {
		return Call{}, err
	}
	var call Call
	if err := json.Unmarshal(body, &call); err != nil {
		return Call{}, err
	}
	return call, nil
}

func validTransition(from, to State) bool {
	switch from {
	case StateRinging:
		return to == StateAccepted || to == StateRejected || to == StateMissed || to == StateFailed || to == StateEnded
	case StateAccepted:
		return to == StateConnected || to == StateEnded || to == StateFailed
	case StateConnected:
		return to == StateEnded || to == StateFailed
	default:
		return false
	}
}

func (s *Service) Transition(ctx context.Context, callID string, actor uint64, state State, reason string) (Call, error) {
	call, err := s.Get(ctx, callID)
	if err != nil {
		return Call{}, err
	}
	if actor != call.CallerID && actor != call.CalleeID && actor != 0 {
		return Call{}, errors.New("not a call participant")
	}
	if !validTransition(call.State, state) {
		if call.State == state {
			return call, nil
		}
		return Call{}, fmt.Errorf("invalid call transition %s -> %s", call.State, state)
	}
	now := time.Now().UTC()
	call.State = state
	if state == StateAccepted {
		call.AcceptedAt = &now
	}
	if state == StateEnded || state == StateRejected || state == StateMissed || state == StateFailed {
		call.EndedAt = &now
		call.EndReason = reason
	}
	body, _ := json.Marshal(call)
	if err := s.redis.Set(ctx, s.callKey(call.ID), body, time.Duration(s.config.Call.ActiveTTLSecond)*time.Second).Err(); err != nil {
		return Call{}, err
	}
	if err := s.persist(ctx, call); err != nil {
		return Call{}, err
	}
	_ = s.redis.ZRem(ctx, "calls:ringing:deadlines", call.ID).Err()
	if call.EndedAt != nil {
		_ = s.releaseLocks(ctx, call)
	}
	eventType := "call." + string(state)
	if state == StateEnded || state == StateRejected || state == StateMissed || state == StateFailed {
		eventType = "call.ended"
	}
	_ = s.publish(ctx, eventType, call.ID, call)
	s.hub.Send(call.CallerID, gin.H{"type": eventType, "data": call})
	s.hub.Send(call.CalleeID, gin.H{"type": eventType, "data": call})
	return call, nil
}

func (s *Service) releaseLocks(ctx context.Context, call Call) error {
	script := redis.NewScript(`
local value = redis.call("GET", KEYS[1])
if value == ARGV[1] then return redis.call("DEL", KEYS[1]) end
return 0`)
	_, firstErr := script.Run(ctx, s.redis, []string{s.userCallKey(call.CallerID)}, call.ID).Result()
	_, secondErr := script.Run(ctx, s.redis, []string{s.userCallKey(call.CalleeID)}, call.ID).Result()
	return errors.Join(firstErr, secondErr)
}

func (s *Service) persist(ctx context.Context, call Call) error {
	retention := s.config.Call.RetentionDay * 24 * 3600
	acceptedAt, endedAt := any(nil), any(nil)
	if call.AcceptedAt != nil {
		acceptedAt = *call.AcceptedAt
	}
	if call.EndedAt != nil {
		endedAt = *call.EndedAt
	}
	for _, userID := range []uint64{call.CallerID, call.CalleeID} {
		err := s.cassandra.Query(
			"INSERT INTO calls_by_user (user_id, created_at, call_id, channel_id, caller_id, callee_id, state, media, accepted_at, ended_at, end_reason) VALUES (?, now(), ?, ?, ?, ?, ?, ?, ?, ?, ?) USING TTL ?",
			userID, call.ID, call.ChannelID, call.CallerID, call.CalleeID, string(call.State), call.Media, acceptedAt, endedAt, call.EndReason, retention,
		).WithContext(ctx).Exec()
		if err != nil {
			return err
		}
	}
	return s.cassandra.Query(
		"INSERT INTO calls_by_channel (channel_id, created_at, call_id, caller_id, callee_id, state, media, accepted_at, ended_at, end_reason) VALUES (?, now(), ?, ?, ?, ?, ?, ?, ?, ?) USING TTL ?",
		call.ChannelID, call.ID, call.CallerID, call.CalleeID, string(call.State), call.Media, acceptedAt, endedAt, call.EndReason, retention,
	).WithContext(ctx).Exec()
}

func (s *Service) publish(ctx context.Context, eventType, aggregateID string, payload any) error {
	event, err := realtime.NewEvent(eventType, "call-service", aggregateID, payload)
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return realtime.PublishDurably(publishCtx, s.cassandra, s.events, "call", realtime.CallEventsTopic, event)
}

func (s *Service) createCall(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request struct {
		CalleeID uint64 `json:"callee_id,string" binding:"required"`
		Media    string `json:"media"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid call request"})
		return
	}
	call, err := s.Create(c.Request.Context(), identity.ChannelID, identity.UserID, request.CalleeID, request.Media)
	if err != nil {
		status := http.StatusConflict
		if strings.Contains(err.Error(), "member") {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, call)
}

func (s *Service) getCall(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	call, err := s.Get(c.Request.Context(), c.Param("id"))
	if err != nil || (identity.UserID != call.CallerID && identity.UserID != call.CalleeID) {
		c.JSON(http.StatusNotFound, gin.H{"message": "call not found"})
		return
	}
	c.JSON(http.StatusOK, call)
}

func (s *Service) endCall(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&request)
	call, err := s.Transition(c.Request.Context(), c.Param("id"), identity.UserID, StateEnded, request.Reason)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, call)
}

func (s *Service) history(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	iter := s.cassandra.Query(
		"SELECT call_id, channel_id, caller_id, callee_id, state, media, dateOf(created_at), accepted_at, ended_at, end_reason FROM calls_by_user WHERE user_id = ? LIMIT 50",
		identity.UserID,
	).WithContext(c.Request.Context()).Iter()
	items := make([]Call, 0, 50)
	var item Call
	for iter.Scan(&item.ID, &item.ChannelID, &item.CallerID, &item.CalleeID, &item.State, &item.Media, &item.CreatedAt, &item.AcceptedAt, &item.EndedAt, &item.EndReason) {
		items = append(items, item)
	}
	if err := iter.Close(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "failed to load call history"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"calls": items})
}

func (s *Service) iceConfig(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	urls := strings.Split(s.config.Call.TURN.URLs, ",")
	username := s.config.Call.TURN.Username
	credential := s.config.Call.TURN.Credential
	if secret := s.config.Call.TURN.SharedSecret; secret != "" {
		expiry := time.Now().Add(time.Duration(s.config.Call.TURN.TTLSecond) * time.Second).Unix()
		username = fmt.Sprintf("%d:%d", expiry, identity.UserID)
		mac := hmac.New(sha1.New, []byte(secret))
		_, _ = mac.Write([]byte(username))
		credential = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	}
	c.JSON(http.StatusOK, gin.H{"ice_servers": []gin.H{{"urls": urls, "username": username, "credential": credential}}})
}

func (s *Service) websocket(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	conn, err := callUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &realtime.Client{UserID: identity.UserID, Device: c.DefaultQuery("device_id", "browser"), Conn: conn, Send: make(chan []byte, 128)}
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
		_, body, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var signal Signal
		if json.Unmarshal(body, &signal) != nil {
			continue
		}
		call, err := s.Get(c.Request.Context(), signal.CallID)
		if err != nil || (identity.UserID != call.CallerID && identity.UserID != call.CalleeID) {
			continue
		}
		target := call.CalleeID
		if identity.UserID == call.CalleeID {
			target = call.CallerID
		}
		switch signal.Type {
		case "accept":
			call, err = s.Transition(c.Request.Context(), call.ID, identity.UserID, StateAccepted, "")
			if err == nil {
				signal.TargetUserID = target
				s.hub.Send(target, signal)
			}
		case "reject":
			_, _ = s.Transition(c.Request.Context(), call.ID, identity.UserID, StateRejected, "rejected")
		case "hangup":
			_, _ = s.Transition(c.Request.Context(), call.ID, identity.UserID, StateEnded, "hangup")
		case "connected":
			_, _ = s.Transition(c.Request.Context(), call.ID, identity.UserID, StateConnected, "")
		case "offer", "answer", "ice_candidate":
			signal.TargetUserID = target
			s.hub.Send(target, signal)
		}
	}
	s.hub.Remove(client)
	close(client.Send)
	_ = conn.Close()
	<-done
}

var callUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (s *Service) timeoutWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			ids, err := s.redis.ZRangeByScore(ctx, "calls:ringing:deadlines", &redis.ZRangeBy{
				Min: "-inf", Max: strconv.FormatInt(now.Unix(), 10), Offset: 0, Count: 100,
			}).Result()
			if err != nil {
				continue
			}
			for _, id := range ids {
				locked, _ := s.redis.SetNX(ctx, "call:timeout-lock:"+id, "1", 10*time.Second).Result()
				if !locked {
					continue
				}
				call, getErr := s.Get(ctx, id)
				if getErr == nil && call.State == StateRinging {
					_, _ = s.Transition(ctx, id, 0, StateMissed, "ring_timeout")
				}
				_ = s.redis.ZRem(ctx, "calls:ringing:deadlines", id).Err()
			}
		}
	}
}

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
	group := engine.Group("/api/calls")
	group.Use(realtime.RequireIdentity(s.cassandra))
	group.POST("", s.createCall)
	group.GET("/history", s.history)
	group.GET("/ice-config", s.iceConfig)
	group.GET("/ws", s.websocket)
	group.GET("/:id", s.getCall)
	group.POST("/:id/end", s.endCall)
	return engine
}

func (s *Service) grpcMethods() map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error) {
	return map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error){
		"GetActiveCall": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			userID, _ := strconv.ParseUint(request.GetFields()["user_id"].GetStringValue(), 10, 64)
			callID, err := s.redis.Get(ctx, s.userCallKey(userID)).Result()
			if err == redis.Nil {
				return structpb.NewStruct(map[string]any{"active": false})
			}
			if err != nil {
				return nil, err
			}
			return structpb.NewStruct(map[string]any{"active": true, "call_id": callID})
		},
		"AuthorizeParticipant": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			userID, _ := strconv.ParseUint(request.GetFields()["user_id"].GetStringValue(), 10, 64)
			call, err := s.Get(ctx, request.GetFields()["call_id"].GetStringValue())
			authorized := err == nil && (call.CallerID == userID || call.CalleeID == userID)
			return structpb.NewStruct(map[string]any{"authorized": authorized})
		},
		"EndCall": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			userID, _ := strconv.ParseUint(request.GetFields()["user_id"].GetStringValue(), 10, 64)
			call, err := s.Transition(ctx, request.GetFields()["call_id"].GetStringValue(), userID, StateEnded, "grpc")
			if err != nil {
				return nil, err
			}
			return structpb.NewStruct(map[string]any{"call_id": call.ID, "state": string(call.State)})
		},
	}
}

func (s *Service) Run(ctx context.Context) error {
	if err := common.NewObservabilityInjector(s.config).Register("call"); err != nil {
		return err
	}
	workerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	go s.timeoutWorker(workerCtx)
	go realtime.RelayOutbox(workerCtx, s.cassandra, s.events, "call")
	s.httpServer = &http.Server{
		Addr: ":" + s.config.Call.Http.Server.Port, Handler: common.NewOtelHttpHandler(s.routes(), "call_http"),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.grpcServer = realtime.NewGRPCServer("nexuschat.call.v1.CallService", s.grpcMethods())
	errorsCh := make(chan error, 2)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()
	go func() { errorsCh <- realtime.ServeGRPC(s.grpcServer, s.config.Call.Grpc.Server.Port) }()
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
		slog.Error("initialize call", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := service.Run(ctx); err != nil {
		slog.Error("call stopped", "error", err)
		os.Exit(1)
	}
}
