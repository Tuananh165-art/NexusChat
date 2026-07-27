package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
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
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type Profile struct {
	UserID           uint64    `json:"user_id,string"`
	Languages        []string  `json:"languages"`
	Interests        []string  `json:"interests"`
	AvoidTopics      []string  `json:"avoid_topics"`
	ConversationGoal string    `json:"conversation_goal"`
	Enabled          bool      `json:"enabled"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Feedback struct {
	ID        string    `json:"id"`
	UserID    uint64    `json:"user_id,string"`
	PeerID    uint64    `json:"peer_id,string"`
	ChannelID uint64    `json:"channel_id,string"`
	Rating    int       `json:"rating"`
	Tags      []string  `json:"tags"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RankedCandidate struct {
	UserID uint64  `json:"user_id,string"`
	Score  float64 `json:"score"`
}

type Service struct {
	config     *config.Config
	redis      redis.UniversalClient
	cassandra  *gocql.Session
	events     *realtime.EventBus
	httpServer *http.Server
	grpcServer *grpc.Server
	stopOnce   sync.Once
}

func NewService(cfg *config.Config) (*Service, error) {
	common.JwtSecret = cfg.Chat.JWT.Secret
	common.JwtExpirationSecond = cfg.Chat.JWT.ExpirationSecond
	rdb, err := infra.NewRedisClient(cfg)
	if err != nil {
		return nil, err
	}
	session, err := infra.NewCassandraSession(cfg)
	if err != nil {
		_ = rdb.Close()
		return nil, err
	}
	events, err := realtime.NewEventBus(common.GetServerAddrs(cfg.Kafka.Addrs), cfg.Kafka.Version)
	if err != nil {
		session.Close()
		_ = rdb.Close()
		return nil, err
	}
	return &Service{config: cfg, redis: rdb, cassandra: session, events: events}, nil
}

func normalize(values []string, max int) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == max {
			break
		}
	}
	sort.Strings(result)
	return result
}

func encode(values []string) string {
	body, _ := json.Marshal(values)
	return string(body)
}

func decode(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	return values
}

func (s *Service) getProfile(ctx context.Context, userID uint64) (Profile, error) {
	var profile Profile
	var languages, interests, avoid string
	err := s.cassandra.Query("SELECT user_id, languages, interests, avoid_topics, conversation_goal, enabled, updated_at FROM discovery_profiles_by_user WHERE user_id = ?", userID).
		WithContext(ctx).Scan(&profile.UserID, &languages, &interests, &avoid, &profile.ConversationGoal, &profile.Enabled, &profile.UpdatedAt)
	profile.Languages, profile.Interests, profile.AvoidTopics = decode(languages), decode(interests), decode(avoid)
	return profile, err
}

func (s *Service) putProfile(ctx context.Context, profile Profile) error {
	profile.Languages = normalize(profile.Languages, 10)
	profile.Interests = normalize(profile.Interests, 30)
	profile.AvoidTopics = normalize(profile.AvoidTopics, 20)
	profile.ConversationGoal = strings.TrimSpace(profile.ConversationGoal)
	profile.UpdatedAt = time.Now().UTC()
	if err := s.cassandra.Query("INSERT INTO discovery_profiles_by_user (user_id, languages, interests, avoid_topics, conversation_goal, enabled, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		profile.UserID, encode(profile.Languages), encode(profile.Interests), encode(profile.AvoidTopics), profile.ConversationGoal, profile.Enabled, profile.UpdatedAt).WithContext(ctx).Exec(); err != nil {
		return err
	}
	body, _ := json.Marshal(profile)
	_ = s.redis.Set(ctx, "discovery:profile:"+strconv.FormatUint(profile.UserID, 10), body, time.Hour).Err()
	event, _ := realtime.NewEvent("discovery.profile.updated", "discovery-service", strconv.FormatUint(profile.UserID, 10), profile)
	return realtime.PublishDurably(ctx, s.cassandra, s.events, "discovery", realtime.DiscoveryEventsTopic, event)
}

func overlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	union := make(map[string]struct{}, len(a)+len(b))
	for _, value := range a {
		union[value] = struct{}{}
	}
	intersection := 0
	for _, value := range b {
		if _, ok := union[value]; ok {
			intersection++
		}
		union[value] = struct{}{}
	}
	return float64(intersection) / float64(len(union))
}

func (s *Service) reputation(ctx context.Context, userID uint64) float64 {
	value, err := s.redis.Get(ctx, "discovery:rating:"+strconv.FormatUint(userID, 10)).Float64()
	if err != nil {
		return 0.5
	}
	return math.Max(0, math.Min(1, value/5))
}

func (s *Service) Rank(ctx context.Context, userID uint64, candidates []uint64) []RankedCandidate {
	source, _ := s.getProfile(ctx, userID)
	result := make([]RankedCandidate, 0, len(candidates))
	for _, candidateID := range candidates {
		if candidateID == 0 || candidateID == userID {
			continue
		}
		candidate, err := s.getProfile(ctx, candidateID)
		if err != nil || !candidate.Enabled {
			result = append(result, RankedCandidate{UserID: candidateID, Score: 10})
			continue
		}
		if overlap(source.AvoidTopics, candidate.Interests) > 0 || overlap(candidate.AvoidTopics, source.Interests) > 0 {
			continue
		}
		score := overlap(source.Interests, candidate.Interests)*40 +
			overlap(source.Languages, candidate.Languages)*20 +
			s.reputation(ctx, candidateID)*15
		if source.ConversationGoal != "" && source.ConversationGoal == candidate.ConversationGoal {
			score += 15
		}
		score += 10
		result = append(result, RankedCandidate{UserID: candidateID, Score: math.Round(score*100) / 100})
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Score > result[j].Score })
	return result
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
	group := engine.Group("/api/discovery")
	group.Use(realtime.RequireUserID(realtime.RedisSessionValidator(s.redis)))
	group.GET("/profile", s.getProfileAPI)
	group.PUT("/profile", s.putProfileAPI)
	group.DELETE("/profile", s.deleteProfileAPI)
	group.GET("/interests", s.getInterestsAPI)
	group.PUT("/interests", s.putInterestsAPI)
	group.GET("/recommendations", s.recommendationsAPI)
	group.GET("/match-history", s.matchHistoryAPI)
	group.POST("/feedback", s.createFeedbackAPI)
	group.PUT("/feedback/:id", s.updateFeedbackAPI)
	group.DELETE("/feedback/:id", s.deleteFeedbackAPI)
	group.GET("/stats", s.statsAPI)
	return engine
}

func (s *Service) getProfileAPI(c *gin.Context) {
	profile, err := s.getProfile(c.Request.Context(), realtime.IdentityFrom(c).UserID)
	if err != nil {
		c.JSON(http.StatusOK, Profile{UserID: realtime.IdentityFrom(c).UserID, Enabled: true, Languages: []string{}, Interests: []string{}, AvoidTopics: []string{}})
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (s *Service) putProfileAPI(c *gin.Context) {
	var profile Profile
	if c.ShouldBindJSON(&profile) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid profile"})
		return
	}
	profile.UserID = realtime.IdentityFrom(c).UserID
	if err := s.putProfile(c.Request.Context(), profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "save profile failed"})
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (s *Service) deleteProfileAPI(c *gin.Context) {
	id := realtime.IdentityFrom(c).UserID
	_ = s.cassandra.Query("DELETE FROM discovery_profiles_by_user WHERE user_id = ?", id).WithContext(c.Request.Context()).Exec()
	_ = s.redis.Del(c.Request.Context(), "discovery:profile:"+strconv.FormatUint(id, 10)).Err()
	c.Status(http.StatusNoContent)
}

func (s *Service) getInterestsAPI(c *gin.Context) {
	profile, _ := s.getProfile(c.Request.Context(), realtime.IdentityFrom(c).UserID)
	c.JSON(http.StatusOK, gin.H{"interests": profile.Interests, "avoid_topics": profile.AvoidTopics})
}

func (s *Service) putInterestsAPI(c *gin.Context) {
	var request struct {
		Interests   []string `json:"interests"`
		AvoidTopics []string `json:"avoid_topics"`
	}
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid interests"})
		return
	}
	id := realtime.IdentityFrom(c).UserID
	profile, _ := s.getProfile(c.Request.Context(), id)
	profile.UserID, profile.Interests, profile.AvoidTopics, profile.Enabled = id, request.Interests, request.AvoidTopics, true
	if err := s.putProfile(c.Request.Context(), profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "save interests failed"})
		return
	}
	c.JSON(http.StatusOK, profile)
}

func (s *Service) recommendationsAPI(c *gin.Context) {
	raw := strings.Split(c.Query("candidate_ids"), ",")
	candidates := make([]uint64, 0, len(raw))
	for _, value := range raw {
		id, _ := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if id != 0 {
			candidates = append(candidates, id)
		}
	}
	c.JSON(http.StatusOK, gin.H{"candidates": s.Rank(c.Request.Context(), realtime.IdentityFrom(c).UserID, candidates)})
}

func (s *Service) createFeedbackAPI(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var feedback Feedback
	if c.ShouldBindJSON(&feedback) != nil || feedback.PeerID == 0 || feedback.Rating < 1 || feedback.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid feedback"})
		return
	}
	feedback.ID, feedback.UserID, feedback.CreatedAt, feedback.UpdatedAt = uuid.NewString(), identity.UserID, time.Now().UTC(), time.Now().UTC()
	feedback.Tags = normalize(feedback.Tags, 10)
	if err := s.cassandra.Query("INSERT INTO discovery_feedback_by_user (user_id, created_at, feedback_id, peer_id, channel_id, rating, tags, comment, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		feedback.UserID, feedback.CreatedAt, feedback.ID, feedback.PeerID, feedback.ChannelID, feedback.Rating, encode(feedback.Tags), feedback.Comment, feedback.UpdatedAt).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "create feedback failed"})
		return
	}
	_ = s.redis.Set(c.Request.Context(), "discovery:rating:"+strconv.FormatUint(feedback.PeerID, 10), feedback.Rating, 7*24*time.Hour).Err()
	event, _ := realtime.NewEvent("discovery.feedback.created", "discovery-service", feedback.ID, feedback)
	_ = realtime.PublishDurably(c.Request.Context(), s.cassandra, s.events, "discovery", realtime.DiscoveryEventsTopic, event)
	c.JSON(http.StatusCreated, feedback)
}

func (s *Service) updateFeedbackAPI(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request struct {
		Rating  int      `json:"rating"`
		Tags    []string `json:"tags"`
		Comment string   `json:"comment"`
	}
	if c.ShouldBindJSON(&request) != nil || request.Rating < 1 || request.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid feedback"})
		return
	}
	now := time.Now().UTC()
	if err := s.cassandra.Query("UPDATE discovery_feedback_by_user SET rating = ?, tags = ?, comment = ?, updated_at = ? WHERE user_id = ? AND created_at = ? AND feedback_id = ?",
		request.Rating, encode(normalize(request.Tags, 10)), request.Comment, now, identity.UserID, c.Query("created_at"), c.Param("id")).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "update feedback failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": c.Param("id"), "updated_at": now})
}

func (s *Service) deleteFeedbackAPI(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	_ = s.cassandra.Query("DELETE FROM discovery_feedback_by_user WHERE user_id = ? AND created_at = ? AND feedback_id = ?", identity.UserID, c.Query("created_at"), c.Param("id")).WithContext(c.Request.Context()).Exec()
	c.Status(http.StatusNoContent)
}

func (s *Service) matchHistoryAPI(c *gin.Context) {
	id := realtime.IdentityFrom(c).UserID
	iter := s.cassandra.Query("SELECT matched_at, match_id, peer_id, channel_id, score FROM discovery_matches_by_user WHERE user_id = ? LIMIT 50", id).WithContext(c.Request.Context()).Iter()
	items := make([]gin.H, 0)
	var at time.Time
	var matchID string
	var peerID, channelID uint64
	var score float64
	for iter.Scan(&at, &matchID, &peerID, &channelID, &score) {
		items = append(items, gin.H{"matched_at": at, "match_id": matchID, "peer_id": strconv.FormatUint(peerID, 10), "channel_id": strconv.FormatUint(channelID, 10), "score": score})
	}
	_ = iter.Close()
	c.JSON(http.StatusOK, gin.H{"matches": items})
}

func (s *Service) statsAPI(c *gin.Context) {
	id := realtime.IdentityFrom(c).UserID
	var count int
	_ = s.cassandra.Query("SELECT count(*) FROM discovery_matches_by_user WHERE user_id = ?", id).WithContext(c.Request.Context()).Scan(&count)
	rating := s.reputation(c.Request.Context(), id) * 5
	c.JSON(http.StatusOK, gin.H{"matches": count, "reputation": rating})
}

func uintField(request *structpb.Struct, name string) uint64 {
	value := request.GetFields()[name]
	if value == nil {
		return 0
	}
	if value.GetStringValue() != "" {
		parsed, _ := strconv.ParseUint(value.GetStringValue(), 10, 64)
		return parsed
	}
	return uint64(value.GetNumberValue())
}

func stringField(request *structpb.Struct, name string) string {
	if value := request.GetFields()[name]; value != nil {
		return value.GetStringValue()
	}
	return ""
}

func (s *Service) grpcMethods() map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error) {
	return map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error){
		"GetDiscoveryProfile": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			profile, err := s.getProfile(ctx, uintField(req, "user_id"))
			if err != nil {
				return structpb.NewStruct(map[string]any{"exists": false})
			}
			return structpb.NewStruct(map[string]any{"exists": true, "languages": encode(profile.Languages), "interests": encode(profile.Interests), "avoid_topics": encode(profile.AvoidTopics), "conversation_goal": profile.ConversationGoal})
		},
		"RankCandidates": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			candidates := make([]uint64, 0)
			for _, raw := range strings.Split(stringField(req, "candidate_ids"), ",") {
				id, _ := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
				if id != 0 {
					candidates = append(candidates, id)
				}
			}
			ranked := s.Rank(ctx, uintField(req, "user_id"), candidates)
			values := make([]any, 0, len(ranked))
			for _, item := range ranked {
				values = append(values, map[string]any{"user_id": strconv.FormatUint(item.UserID, 10), "score": item.Score})
			}
			return structpb.NewStruct(map[string]any{"candidates": values})
		},
		"RecordMatch": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			matchID, matchedAt := uuid.NewString(), time.Now().UTC()
			userID, peerID, channelID := uintField(req, "user_id"), uintField(req, "peer_id"), uintField(req, "channel_id")
			score := 0.0
			if value := req.GetFields()["score"]; value != nil {
				score = value.GetNumberValue()
			}
			for _, pair := range [][2]uint64{{userID, peerID}, {peerID, userID}} {
				_ = s.cassandra.Query("INSERT INTO discovery_matches_by_user (user_id, matched_at, match_id, peer_id, channel_id, score) VALUES (?, ?, ?, ?, ?, ?)", pair[0], matchedAt, matchID, pair[1], channelID, score).WithContext(ctx).Exec()
			}
			event, _ := realtime.NewEvent("discovery.match.created", "discovery-service", matchID, map[string]any{"user_id": userID, "peer_id": peerID, "channel_id": channelID, "score": score})
			_ = realtime.PublishDurably(ctx, s.cassandra, s.events, "discovery", realtime.DiscoveryEventsTopic, event)
			return structpb.NewStruct(map[string]any{"match_id": matchID})
		},
		"RecordConversationEnded": func(context.Context, *structpb.Struct) (*structpb.Struct, error) {
			return structpb.NewStruct(map[string]any{"recorded": true})
		},
		"GetRecentPeers": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			iter := s.cassandra.Query("SELECT peer_id FROM discovery_matches_by_user WHERE user_id = ? LIMIT 20", uintField(req, "user_id")).WithContext(ctx).Iter()
			peers := make([]any, 0)
			var id uint64
			for iter.Scan(&id) {
				peers = append(peers, strconv.FormatUint(id, 10))
			}
			_ = iter.Close()
			return structpb.NewStruct(map[string]any{"peer_ids": peers})
		},
	}
}

func (s *Service) Run(ctx context.Context) error {
	if err := common.NewObservabilityInjector(s.config).Register("discovery"); err != nil {
		return err
	}
	s.httpServer = &http.Server{Addr: ":" + s.config.Discovery.Http.Server.Port, Handler: common.NewOtelHttpHandler(s.routes(), "discovery_http"), ReadHeaderTimeout: 10 * time.Second}
	s.grpcServer = realtime.NewGRPCServer("nexuschat.discovery.v1.DiscoveryService", s.grpcMethods())
	go realtime.RelayOutbox(ctx, s.cassandra, s.events, "discovery")
	errorsCh := make(chan error, 2)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()
	go func() { errorsCh <- realtime.ServeGRPC(s.grpcServer, s.config.Discovery.Grpc.Server.Port) }()
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
		timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if s.httpServer != nil {
			joined = errors.Join(joined, s.httpServer.Shutdown(timeout))
		}
		if s.grpcServer != nil {
			s.grpcServer.GracefulStop()
		}
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
		slog.Error("initialize discovery", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := service.Run(ctx); err != nil {
		slog.Error("discovery stopped", "error", err)
		os.Exit(1)
	}
}
