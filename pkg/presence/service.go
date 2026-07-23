package presence

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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/Tuananh165-art/NexusChat/pkg/config"
	"github.com/Tuananh165-art/NexusChat/pkg/infra"
	"github.com/Tuananh165-art/NexusChat/pkg/realtime"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type Status struct {
	UserID   uint64    `json:"user_id,string"`
	Online   bool      `json:"online"`
	LastSeen time.Time `json:"last_seen"`
	Devices  int       `json:"devices"`
}

type Service struct {
	config     *config.Config
	redis      redis.UniversalClient
	cassandra  *gocql.Session
	events     *realtime.EventBus
	hub        *realtime.Hub
	httpServer *http.Server
	grpcServer *grpc.Server
	stop       chan struct{}
	stopOnce   sync.Once
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
	return &Service{
		config: cfg, redis: redisClient, cassandra: session,
		events: events, hub: realtime.NewHub(), stop: make(chan struct{}),
	}, nil
}

func (s *Service) presenceKey(userID uint64, device string) string {
	return fmt.Sprintf("presence:user:%d:device:%s", userID, device)
}

func (s *Service) devicesKey(userID uint64) string {
	return fmt.Sprintf("presence:user:%d:devices", userID)
}

func (s *Service) lastSeenKey(userID uint64) string {
	return fmt.Sprintf("presence:user:%d:last_seen", userID)
}

func (s *Service) Touch(ctx context.Context, userID uint64, device string) (Status, error) {
	if device == "" {
		device = "browser"
	}
	key := s.presenceKey(userID, device)
	wasOnline, err := s.isOnline(ctx, userID)
	if err != nil {
		return Status{}, err
	}
	now := time.Now().UTC()
	pipe := s.redis.TxPipeline()
	pipe.Set(ctx, key, now.Format(time.RFC3339Nano), time.Duration(s.config.Presence.TTLSecond)*time.Second)
	pipe.SAdd(ctx, s.devicesKey(userID), device)
	pipe.Expire(ctx, s.devicesKey(userID), time.Duration(activeTTL(s.config.Presence))*time.Second)
	pipe.Set(ctx, s.lastSeenKey(userID), now.Format(time.RFC3339Nano), 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return Status{}, err
	}
	status, err := s.Get(ctx, userID)
	if err == nil && !wasOnline && status.Online {
		s.publishTransition(ctx, status)
	}
	return status, err
}

func (s *Service) Disconnect(ctx context.Context, userID uint64, device string) (Status, error) {
	if device == "" {
		device = "browser"
	}
	if err := s.redis.Del(ctx, s.presenceKey(userID, device)).Err(); err != nil {
		return Status{}, err
	}
	_ = s.redis.SRem(ctx, s.devicesKey(userID), device).Err()
	status, err := s.Get(ctx, userID)
	if err == nil && !status.Online {
		status.LastSeen = time.Now().UTC()
		_ = s.redis.Set(ctx, s.lastSeenKey(userID), status.LastSeen.Format(time.RFC3339Nano), 0).Err()
		s.publishTransition(ctx, status)
	}
	return status, err
}

func (s *Service) isOnline(ctx context.Context, userID uint64) (bool, error) {
	status, err := s.Get(ctx, userID)
	return status.Online, err
}

func (s *Service) Get(ctx context.Context, userID uint64) (Status, error) {
	devices, err := s.redis.SMembers(ctx, s.devicesKey(userID)).Result()
	if err != nil && err != redis.Nil {
		return Status{}, err
	}
	alive := 0
	for _, device := range devices {
		exists, existsErr := s.redis.Exists(ctx, s.presenceKey(userID, device)).Result()
		if existsErr != nil {
			return Status{}, existsErr
		}
		if exists > 0 {
			alive++
		} else {
			_ = s.redis.SRem(ctx, s.devicesKey(userID), device).Err()
		}
	}
	lastSeen := time.Time{}
	raw, _ := s.redis.Get(ctx, s.lastSeenKey(userID)).Result()
	if raw != "" {
		lastSeen, _ = time.Parse(time.RFC3339Nano, raw)
	}
	return Status{UserID: userID, Online: alive > 0, LastSeen: lastSeen, Devices: alive}, nil
}

func (s *Service) publishTransition(ctx context.Context, status Status) {
	event, err := realtime.NewEvent("presence.status.changed", "presence-service", strconv.FormatUint(status.UserID, 10), status)
	if err != nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := realtime.PublishDurably(publishCtx, s.cassandra, s.events, "presence", realtime.PresenceEventsTopic, event); err != nil {
		slog.Error("publish presence transition", "error", err)
	}
	s.hub.Broadcast(gin.H{"type": "presence.status.changed", "data": status})
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
	group := engine.Group("/api/presence")
	group.Use(realtime.RequireIdentity(s.cassandra))
	group.GET("/users", s.getUsers)
	group.GET("/ws", s.websocket)
	return engine
}

func (s *Service) getUsers(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	rawIDs := strings.Split(c.Query("ids"), ",")
	if len(rawIDs) == 1 && rawIDs[0] == "" {
		rawIDs = []string{strconv.FormatUint(identity.UserID, 10)}
	}
	if len(rawIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "at most 100 users"})
		return
	}
	statuses := make([]Status, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			continue
		}
		var member uint64
		if err := s.cassandra.Query("SELECT user_id FROM channels WHERE id = ? AND user_id = ? LIMIT 1", identity.ChannelID, id).
			WithContext(c.Request.Context()).Scan(&member); err != nil {
			continue
		}
		status, err := s.Get(c.Request.Context(), id)
		if err == nil {
			statuses = append(statuses, status)
		}
	}
	c.JSON(http.StatusOK, gin.H{"users": statuses})
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool { return true },
}

func (s *Service) websocket(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	device := c.DefaultQuery("device_id", "browser")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &realtime.Client{UserID: identity.UserID, Device: device, Conn: conn, Send: make(chan []byte, 64)}
	s.hub.Add(client)
	if _, err := s.Touch(c.Request.Context(), identity.UserID, device); err != nil {
		_ = conn.Close()
		s.hub.Remove(client)
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for message := range client.Send {
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}()
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(time.Duration(s.config.Presence.TTLSecond) * 2 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(time.Duration(s.config.Presence.TTLSecond) * 2 * time.Second))
		_, err := s.Touch(context.Background(), identity.UserID, device)
		return err
	})
	for {
		_, body, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var message struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(body, &message) == nil && message.Type == "heartbeat" {
			_, _ = s.Touch(c.Request.Context(), identity.UserID, device)
		}
	}
	s.hub.Remove(client)
	close(client.Send)
	_ = conn.Close()
	<-done
	_, _ = s.Disconnect(context.Background(), identity.UserID, device)
}

func (s *Service) grpcMethods() map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error) {
	getUint := func(request *structpb.Struct, field string) uint64 {
		value := request.GetFields()[field]
		if value == nil {
			return 0
		}
		parsed, _ := strconv.ParseUint(value.GetStringValue(), 10, 64)
		if parsed == 0 {
			parsed = uint64(value.GetNumberValue())
		}
		return parsed
	}
	getString := func(request *structpb.Struct, field string) string {
		value := request.GetFields()[field]
		if value == nil {
			return ""
		}
		return value.GetStringValue()
	}
	toStruct := func(status Status) (*structpb.Struct, error) {
		body, _ := json.Marshal(status)
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		return structpb.NewStruct(value)
	}
	return map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error){
		"GetPresence": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			status, err := s.Get(ctx, getUint(request, "user_id"))
			if err != nil {
				return nil, err
			}
			return toStruct(status)
		},
		"BatchGetPresence": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			raw := getString(request, "user_ids")
			result := make([]any, 0)
			for _, part := range strings.Split(raw, ",") {
				userID, parseErr := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
				if parseErr != nil {
					continue
				}
				status, getErr := s.Get(ctx, userID)
				if getErr == nil {
					result = append(result, map[string]any{
						"user_id": strconv.FormatUint(status.UserID, 10), "online": status.Online,
						"last_seen": status.LastSeen.Format(time.RFC3339Nano), "devices": status.Devices,
					})
				}
			}
			return structpb.NewStruct(map[string]any{"users": result})
		},
		"TouchPresence": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			status, err := s.Touch(ctx, getUint(request, "user_id"), getString(request, "device_id"))
			if err != nil {
				return nil, err
			}
			return toStruct(status)
		},
		"DisconnectDevice": func(ctx context.Context, request *structpb.Struct) (*structpb.Struct, error) {
			status, err := s.Disconnect(ctx, getUint(request, "user_id"), getString(request, "device_id"))
			if err != nil {
				return nil, err
			}
			return toStruct(status)
		},
	}
}

func (s *Service) Run(ctx context.Context) error {
	if err := common.NewObservabilityInjector(s.config).Register("presence"); err != nil {
		return err
	}
	s.httpServer = &http.Server{
		Addr:              ":" + s.config.Presence.Http.Server.Port,
		Handler:           common.NewOtelHttpHandler(s.routes(), "presence_http"),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go realtime.RelayOutbox(ctx, s.cassandra, s.events, "presence")
	s.grpcServer = realtime.NewGRPCServer("nexuschat.presence.v1.PresenceService", s.grpcMethods())
	errorsCh := make(chan error, 2)
	go func() {
		err := s.httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()
	go func() {
		if err := realtime.ServeGRPC(s.grpcServer, s.config.Presence.Grpc.Server.Port); err != nil {
			errorsCh <- err
		}
	}()
	select {
	case <-ctx.Done():
		return s.Close(context.Background())
	case err := <-errorsCh:
		_ = s.Close(context.Background())
		return err
	}
}

func (s *Service) Close(ctx context.Context) error {
	var joined error
	s.stopOnce.Do(func() {
		close(s.stop)
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
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
	})
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
		slog.Error("initialize presence", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := service.Run(ctx); err != nil {
		slog.Error("presence stopped", "error", err)
		os.Exit(1)
	}
}

func activeTTL(p *config.PresenceConfig) int64 {
	if p.TTLSecond < 45 {
		return 120
	}
	return p.TTLSecond * 4
}
