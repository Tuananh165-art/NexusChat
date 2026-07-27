package safety

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
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

type Decision struct {
	ID        string    `json:"id"`
	ChannelID uint64    `json:"channel_id,string"`
	MessageID uint64    `json:"message_id,string"`
	UserID    uint64    `json:"user_id,string"`
	Score     int       `json:"score"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type Report struct {
	ID           string    `json:"id"`
	ReporterID   uint64    `json:"reporter_id,string"`
	TargetUserID uint64    `json:"target_user_id,string"`
	ChannelID    uint64    `json:"channel_id,string"`
	MessageID    uint64    `json:"message_id,string"`
	Reason       string    `json:"reason"`
	Details      string    `json:"details"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Rule struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Pattern   string    `json:"pattern"`
	Score     int       `json:"score"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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

var (
	urlPattern   = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+`)
	scamPattern  = regexp.MustCompile(`(?i)\b(?:seed phrase|private key|free money|guaranteed profit|chuyển khoản|trúng thưởng|mã otp)\b`)
	toxicPattern = regexp.MustCompile(`(?i)\b(?:kill yourself|đồ ngu|cút đi|fuck you|địt mẹ)\b`)
)

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

func (s *Service) Moderate(ctx context.Context, channelID, messageID, userID uint64, content string) (Decision, error) {
	now := time.Now().UTC()
	decision := Decision{
		ID: uuid.NewString(), ChannelID: channelID, MessageID: messageID,
		UserID: userID, Action: "allow", CreatedAt: now,
	}
	muteKey := fmt.Sprintf("safety:mute:%d", userID)
	if ttl, err := s.redis.TTL(ctx, muteKey).Result(); err == nil && ttl > 0 {
		decision.Score, decision.Action, decision.Reason = 100, "block", "temporarily rate limited"
		return decision, s.persistDecision(ctx, decision)
	}
	rateKey := fmt.Sprintf("safety:rate:%d:%d", userID, now.Unix()/s.config.Safety.RateLimit.WindowSecond)
	count, err := s.redis.Incr(ctx, rateKey).Result()
	if err == nil {
		_ = s.redis.Expire(ctx, rateKey, time.Duration(s.config.Safety.RateLimit.WindowSecond+2)*time.Second).Err()
		if count > s.config.Safety.RateLimit.MaxMessages {
			_ = s.redis.Set(ctx, muteKey, "1", time.Duration(s.config.Safety.RateLimit.MuteSecond)*time.Second).Err()
			decision.Score, decision.Action, decision.Reason = 90, "block", "message flood detected"
		}
	}
	normalized := strings.TrimSpace(content)
	if len(normalized) > 0 {
		repeatKey := fmt.Sprintf("safety:repeat:%d:%x", userID, []byte(strings.ToLower(normalized)))
		repeats, repeatErr := s.redis.Incr(ctx, repeatKey).Result()
		if repeatErr == nil {
			_ = s.redis.Expire(ctx, repeatKey, 2*time.Minute).Err()
			if repeats >= 4 && decision.Score < 75 {
				decision.Score, decision.Action, decision.Reason = 75, "block", "repeated spam"
			}
		}
	}
	if scamPattern.MatchString(normalized) && decision.Score < 85 {
		decision.Score, decision.Action, decision.Reason = 85, "block", "possible scam"
	} else if toxicPattern.MatchString(normalized) && decision.Score < 60 {
		decision.Score, decision.Action, decision.Reason = 60, "warn", "possible harassment"
	} else if urlPattern.MatchString(normalized) && decision.Score < 45 {
		decision.Score, decision.Action, decision.Reason = 45, "warn", "external link"
	}
	for _, rule := range s.enabledRules(ctx) {
		matched, compileErr := regexp.MatchString(rule.Pattern, normalized)
		if compileErr == nil && matched && rule.Score > decision.Score {
			decision.Score, decision.Reason = rule.Score, rule.Name
		}
	}
	switch {
	case decision.Score >= s.config.Safety.BlockScore:
		decision.Action = "block"
	case decision.Score >= s.config.Safety.WarnScore:
		decision.Action = "warn"
	default:
		decision.Action = "allow"
	}
	return decision, s.persistDecision(ctx, decision)
}

func (s *Service) enabledRules(ctx context.Context) []Rule {
	iter := s.cassandra.Query("SELECT rule_id, name, pattern, score, enabled, created_at, updated_at FROM safety_rules ALLOW FILTERING").WithContext(ctx).Iter()
	rules := make([]Rule, 0)
	var rule Rule
	for iter.Scan(&rule.ID, &rule.Name, &rule.Pattern, &rule.Score, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt) {
		if rule.Enabled {
			rules = append(rules, rule)
		}
		rule = Rule{}
	}
	_ = iter.Close()
	return rules
}

func (s *Service) persistDecision(ctx context.Context, decision Decision) error {
	if decision.Score < s.config.Safety.WarnScore {
		return nil
	}
	if err := s.cassandra.Query(
		"INSERT INTO safety_decisions_by_channel (channel_id, created_at, decision_id, message_id, user_id, score, action, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		decision.ChannelID, decision.CreatedAt, decision.ID, decision.MessageID, decision.UserID, decision.Score, decision.Action, decision.Reason,
	).WithContext(ctx).Exec(); err != nil {
		return err
	}
	_ = s.cassandra.Query(
		"INSERT INTO safety_decisions_by_user (user_id, created_at, decision_id, channel_id, message_id, score, action, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		decision.UserID, decision.CreatedAt, decision.ID, decision.ChannelID, decision.MessageID, decision.Score, decision.Action, decision.Reason,
	).WithContext(ctx).Exec()
	_ = s.redis.Set(ctx, fmt.Sprintf("safety:risk:%d", decision.UserID), decision.Score, 24*time.Hour).Err()
	event, err := realtime.NewEvent("safety.message."+decision.Action+"ed", "safety-service", decision.ID, decision)
	if err == nil {
		_ = realtime.PublishDurably(ctx, s.cassandra, s.events, "safety", realtime.SafetyEventsTopic, event)
	}
	return nil
}

func (s *Service) routes() *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery(), common.CorsMiddleware())
	engine.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	engine.GET("/ready", s.ready)
	group := engine.Group("/api/safety")
	group.Use(realtime.RequireIdentity(s.cassandra, realtime.RedisSessionValidator(s.redis)))
	group.POST("/reports", s.createReport)
	group.GET("/reports", s.listReports)
	group.GET("/reports/:id", s.getReport)
	group.PUT("/reports/:id/status", s.updateReport)
	group.POST("/reports/:id/appeals", s.createAppeal)
	group.GET("/blocks", s.listBlocks)
	group.POST("/blocks", s.createBlock)
	group.DELETE("/blocks/:userId", s.deleteBlock)
	group.GET("/decisions", s.listDecisions)
	group.GET("/rules", s.listRules)
	group.POST("/rules", s.createRule)
	group.PUT("/rules/:id", s.updateRule)
	group.DELETE("/rules/:id", s.deleteRule)
	group.GET("/risk/:userId", s.getRisk)
	return engine
}

func (s *Service) isModerator(userID uint64) bool {
	for _, raw := range strings.Split(os.Getenv("SAFETY_MODERATOR_USER_IDS"), ",") {
		id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
		if err == nil && id != 0 && id == userID {
			return true
		}
	}
	return false
}

func (s *Service) requireModerator(c *gin.Context) bool {
	if !s.isModerator(realtime.IdentityFrom(c).UserID) {
		c.JSON(http.StatusForbidden, gin.H{"message": "moderator permission required"})
		return false
	}
	return true
}

func (s *Service) ready(c *gin.Context) {
	if err := s.redis.Ping(c.Request.Context()).Err(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func canAccessReport(identity realtime.Identity, report Report) bool {
	return identity.ChannelID == report.ChannelID &&
		(identity.UserID == report.ReporterID || identity.UserID == report.TargetUserID)
}

func (s *Service) createReport(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request struct {
		TargetUserID uint64 `json:"target_user_id,string"`
		MessageID    uint64 `json:"message_id,string"`
		Reason       string `json:"reason"`
		Details      string `json:"details"`
	}
	if c.ShouldBindJSON(&request) != nil || request.TargetUserID == 0 || strings.TrimSpace(request.Reason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid report"})
		return
	}
	now := time.Now().UTC()
	report := Report{ID: uuid.NewString(), ReporterID: identity.UserID, TargetUserID: request.TargetUserID, ChannelID: identity.ChannelID, MessageID: request.MessageID, Reason: request.Reason, Details: request.Details, Status: "open", CreatedAt: now, UpdatedAt: now}
	if err := s.cassandra.Query("INSERT INTO safety_reports_by_id (report_id, reporter_id, target_user_id, channel_id, message_id, reason, details, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		report.ID, report.ReporterID, report.TargetUserID, report.ChannelID, report.MessageID, report.Reason, report.Details, report.Status, report.CreatedAt, report.UpdatedAt).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "create report failed"})
		return
	}
	_ = s.cassandra.Query("INSERT INTO safety_reports_by_status (status, created_at, report_id, reporter_id, target_user_id, channel_id, message_id, reason) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		report.Status, report.CreatedAt, report.ID, report.ReporterID, report.TargetUserID, report.ChannelID, report.MessageID, report.Reason).WithContext(c.Request.Context()).Exec()
	event, _ := realtime.NewEvent("safety.report.created", "safety-service", report.ID, report)
	_ = realtime.PublishDurably(c.Request.Context(), s.cassandra, s.events, "safety", realtime.SafetyEventsTopic, event)
	c.JSON(http.StatusCreated, report)
}

func (s *Service) scanReport(ctx context.Context, id string) (Report, error) {
	var report Report
	err := s.cassandra.Query("SELECT report_id, reporter_id, target_user_id, channel_id, message_id, reason, details, status, created_at, updated_at FROM safety_reports_by_id WHERE report_id = ?", id).
		WithContext(ctx).Scan(&report.ID, &report.ReporterID, &report.TargetUserID, &report.ChannelID, &report.MessageID, &report.Reason, &report.Details, &report.Status, &report.CreatedAt, &report.UpdatedAt)
	return report, err
}

func (s *Service) getReport(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	report, err := s.scanReport(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "report not found"})
		return
	}
	if !s.isModerator(identity.UserID) && !canAccessReport(identity, report) {
		c.JSON(http.StatusForbidden, gin.H{"message": "report access denied"})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (s *Service) listReports(c *gin.Context) {
	if !s.requireModerator(c) {
		return
	}
	status := c.DefaultQuery("status", "open")
	iter := s.cassandra.Query("SELECT report_id, reporter_id, target_user_id, channel_id, message_id, reason, created_at FROM safety_reports_by_status WHERE status = ? LIMIT 100", status).WithContext(c.Request.Context()).Iter()
	reports := make([]Report, 0)
	var item Report
	for iter.Scan(&item.ID, &item.ReporterID, &item.TargetUserID, &item.ChannelID, &item.MessageID, &item.Reason, &item.CreatedAt) {
		item.Status = status
		reports = append(reports, item)
		item = Report{}
	}
	_ = iter.Close()
	c.JSON(http.StatusOK, gin.H{"reports": reports})
}

func (s *Service) updateReport(c *gin.Context) {
	if !s.requireModerator(c) {
		return
	}
	var request struct {
		Status string `json:"status"`
	}
	if c.ShouldBindJSON(&request) != nil || (request.Status != "reviewing" && request.Status != "resolved" && request.Status != "rejected") {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid status"})
		return
	}
	report, err := s.scanReport(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "report not found"})
		return
	}
	report.Status, report.UpdatedAt = request.Status, time.Now().UTC()
	_ = s.cassandra.Query("UPDATE safety_reports_by_id SET status = ?, updated_at = ? WHERE report_id = ?", report.Status, report.UpdatedAt, report.ID).WithContext(c.Request.Context()).Exec()
	event, _ := realtime.NewEvent("safety.report."+report.Status, "safety-service", report.ID, report)
	_ = realtime.PublishDurably(c.Request.Context(), s.cassandra, s.events, "safety", realtime.SafetyEventsTopic, event)
	c.JSON(http.StatusOK, report)
}

func (s *Service) createAppeal(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	report, err := s.scanReport(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "report not found"})
		return
	}
	if identity.UserID != report.TargetUserID || identity.ChannelID != report.ChannelID {
		c.JSON(http.StatusForbidden, gin.H{"message": "appeal access denied"})
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if c.ShouldBindJSON(&request) != nil || strings.TrimSpace(request.Reason) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "reason is required"})
		return
	}
	id, now := uuid.NewString(), time.Now().UTC()
	if err := s.cassandra.Query("INSERT INTO safety_appeals_by_report (report_id, created_at, appeal_id, user_id, reason, status) VALUES (?, ?, ?, ?, ?, ?)",
		c.Param("id"), now, id, identity.UserID, request.Reason, "open").WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "create appeal failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "status": "open"})
}

func (s *Service) createBlock(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request struct {
		UserID uint64 `json:"user_id,string"`
	}
	if c.ShouldBindJSON(&request) != nil || request.UserID == 0 || request.UserID == identity.UserID {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid user"})
		return
	}
	now := time.Now().UTC()
	if err := s.cassandra.Query("INSERT INTO safety_blocks_by_user (user_id, blocked_user_id, created_at) VALUES (?, ?, ?)", identity.UserID, request.UserID, now).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "block failed"})
		return
	}
	_ = s.redis.SAdd(c.Request.Context(), fmt.Sprintf("safety:block:%d", identity.UserID), request.UserID).Err()
	event, _ := realtime.NewEvent("safety.user.blocked", "safety-service", strconv.FormatUint(identity.UserID, 10), gin.H{"user_id": identity.UserID, "blocked_user_id": request.UserID})
	_ = realtime.PublishDurably(c.Request.Context(), s.cassandra, s.events, "safety", realtime.SafetyEventsTopic, event)
	c.Status(http.StatusNoContent)
}

func (s *Service) deleteBlock(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	blocked, err := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid user"})
		return
	}
	_ = s.cassandra.Query("DELETE FROM safety_blocks_by_user WHERE user_id = ? AND blocked_user_id = ?", identity.UserID, blocked).WithContext(c.Request.Context()).Exec()
	_ = s.redis.SRem(c.Request.Context(), fmt.Sprintf("safety:block:%d", identity.UserID), blocked).Err()
	c.Status(http.StatusNoContent)
}

func (s *Service) listBlocks(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	iter := s.cassandra.Query("SELECT blocked_user_id, created_at FROM safety_blocks_by_user WHERE user_id = ?", identity.UserID).WithContext(c.Request.Context()).Iter()
	items := make([]gin.H, 0)
	var id uint64
	var created time.Time
	for iter.Scan(&id, &created) {
		items = append(items, gin.H{"user_id": strconv.FormatUint(id, 10), "created_at": created})
	}
	_ = iter.Close()
	c.JSON(http.StatusOK, gin.H{"blocks": items})
}

func (s *Service) listDecisions(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	iter := s.cassandra.Query("SELECT decision_id, created_at, message_id, user_id, score, action, reason FROM safety_decisions_by_channel WHERE channel_id = ? LIMIT 100", identity.ChannelID).WithContext(c.Request.Context()).Iter()
	items := make([]Decision, 0)
	var item Decision
	for iter.Scan(&item.ID, &item.CreatedAt, &item.MessageID, &item.UserID, &item.Score, &item.Action, &item.Reason) {
		item.ChannelID = identity.ChannelID
		items = append(items, item)
		item = Decision{}
	}
	_ = iter.Close()
	c.JSON(http.StatusOK, gin.H{"decisions": items})
}

func (s *Service) listRules(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"rules": s.enabledRules(c.Request.Context())})
}

func (s *Service) createRule(c *gin.Context) {
	if !s.requireModerator(c) {
		return
	}
	var rule Rule
	if c.ShouldBindJSON(&rule) != nil || rule.Name == "" || rule.Pattern == "" || rule.Score < 1 || rule.Score > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid rule"})
		return
	}
	if _, err := regexp.Compile(rule.Pattern); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid regex"})
		return
	}
	rule.ID, rule.CreatedAt, rule.UpdatedAt = uuid.NewString(), time.Now().UTC(), time.Now().UTC()
	if err := s.cassandra.Query("INSERT INTO safety_rules (rule_id, name, pattern, score, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		rule.ID, rule.Name, rule.Pattern, rule.Score, rule.Enabled, rule.CreatedAt, rule.UpdatedAt).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "create rule failed"})
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (s *Service) updateRule(c *gin.Context) {
	if !s.requireModerator(c) {
		return
	}
	var rule Rule
	if c.ShouldBindJSON(&rule) != nil || rule.Name == "" || rule.Pattern == "" || rule.Score < 1 || rule.Score > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid rule"})
		return
	}
	if _, err := regexp.Compile(rule.Pattern); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid regex"})
		return
	}
	rule.ID, rule.UpdatedAt = c.Param("id"), time.Now().UTC()
	if err := s.cassandra.Query("UPDATE safety_rules SET name = ?, pattern = ?, score = ?, enabled = ?, updated_at = ? WHERE rule_id = ?",
		rule.Name, rule.Pattern, rule.Score, rule.Enabled, rule.UpdatedAt, rule.ID).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "update rule failed"})
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (s *Service) deleteRule(c *gin.Context) {
	if !s.requireModerator(c) {
		return
	}
	_ = s.cassandra.Query("DELETE FROM safety_rules WHERE rule_id = ?", c.Param("id")).WithContext(c.Request.Context()).Exec()
	c.Status(http.StatusNoContent)
}

func (s *Service) getRisk(c *gin.Context) {
	if !s.requireModerator(c) {
		return
	}
	id, err := strconv.ParseUint(c.Param("userId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid user"})
		return
	}
	score, _ := s.redis.Get(c.Request.Context(), fmt.Sprintf("safety:risk:%d", id)).Int()
	c.JSON(http.StatusOK, gin.H{"user_id": strconv.FormatUint(id, 10), "score": score})
}

func (s *Service) isBlocked(ctx context.Context, a, b uint64) bool {
	var blocked uint64
	if err := s.cassandra.Query("SELECT blocked_user_id FROM safety_blocks_by_user WHERE user_id = ? AND blocked_user_id = ? LIMIT 1", a, b).WithContext(ctx).Scan(&blocked); err == nil {
		return true
	}
	return s.cassandra.Query("SELECT blocked_user_id FROM safety_blocks_by_user WHERE user_id = ? AND blocked_user_id = ? LIMIT 1", b, a).WithContext(ctx).Scan(&blocked) == nil
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
		"ModerateMessage": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			decision, err := s.Moderate(ctx, uintField(req, "channel_id"), uintField(req, "message_id"), uintField(req, "user_id"), stringField(req, "content"))
			if err != nil {
				return nil, err
			}
			return structpb.NewStruct(map[string]any{"decision_id": decision.ID, "score": decision.Score, "action": decision.Action, "reason": decision.Reason})
		},
		"IsUserBlocked": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			return structpb.NewStruct(map[string]any{"blocked": s.isBlocked(ctx, uintField(req, "user_id"), uintField(req, "peer_id"))})
		},
		"BatchFilterCandidates": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			userID := uintField(req, "user_id")
			allowed := make([]any, 0)
			for _, raw := range strings.Split(stringField(req, "candidate_ids"), ",") {
				id, _ := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
				if id != 0 && id != userID && !s.isBlocked(ctx, userID, id) {
					risk, _ := s.redis.Get(ctx, fmt.Sprintf("safety:risk:%d", id)).Int()
					if risk < s.config.Safety.BlockScore {
						allowed = append(allowed, strconv.FormatUint(id, 10))
					}
				}
			}
			return structpb.NewStruct(map[string]any{"candidate_ids": allowed})
		},
	}
}

func (s *Service) Run(ctx context.Context) error {
	if err := common.NewObservabilityInjector(s.config).Register("safety"); err != nil {
		return err
	}
	s.httpServer = &http.Server{Addr: ":" + s.config.Safety.Http.Server.Port, Handler: common.NewOtelHttpHandler(s.routes(), "safety_http"), ReadHeaderTimeout: 10 * time.Second}
	s.grpcServer = realtime.NewGRPCServer("nexuschat.safety.v1.SafetyService", s.grpcMethods())
	go realtime.RelayOutbox(ctx, s.cassandra, s.events, "safety")
	errorsCh := make(chan error, 2)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()
	go func() { errorsCh <- realtime.ServeGRPC(s.grpcServer, s.config.Safety.Grpc.Server.Port) }()
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
		slog.Error("initialize safety", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := service.Run(ctx); err != nil {
		slog.Error("safety stopped", "error", err)
		os.Exit(1)
	}
}

func EncodeDecision(decision Decision) string {
	body, _ := json.Marshal(decision)
	return string(body)
}
