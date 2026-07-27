package workspace

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
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"
)

type Item struct {
	ID         string     `json:"id"`
	ChannelID  uint64     `json:"channel_id,string"`
	OwnerID    uint64     `json:"owner_id,string"`
	Kind       string     `json:"kind"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	Status     string     `json:"status"`
	Priority   string     `json:"priority"`
	Assignees  []uint64   `json:"assignees"`
	Tags       []string   `json:"tags"`
	MessageID  uint64     `json:"message_id,string"`
	DueAt      *time.Time `json:"due_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

type Service struct {
	config     *config.Config
	redis      redis.UniversalClient
	cassandra  *gocql.Session
	events     *realtime.EventBus
	hub        *realtime.Hub
	httpServer *http.Server
	grpcServer *grpc.Server
	stopOnce   sync.Once
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

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
	return &Service{config: cfg, redis: rdb, cassandra: session, events: events, hub: realtime.NewHub()}, nil
}

func encodeIDs(ids []uint64) string {
	body, _ := json.Marshal(ids)
	return string(body)
}

func decodeIDs(raw string) []uint64 {
	var ids []uint64
	_ = json.Unmarshal([]byte(raw), &ids)
	return ids
}

func (s *Service) saveItem(ctx context.Context, item Item) error {
	if item.Status == "" {
		item.Status = "todo"
	}
	if item.Priority == "" {
		item.Priority = "normal"
	}
	if item.Kind != "task" && item.Kind != "note" && item.Kind != "bookmark" {
		return errors.New("kind must be task, note or bookmark")
	}
	if item.ID == "" {
		item.ID = uuid.NewString()
		item.CreatedAt = time.Now().UTC()
	}
	item.UpdatedAt = time.Now().UTC()
	if err := s.cassandra.Query("INSERT INTO workspace_items_by_channel (channel_id, updated_at, item_id, owner_id, kind, title, content, status, priority, assignees, tags, message_id, due_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ChannelID, item.UpdatedAt, item.ID, item.OwnerID, item.Kind, item.Title, item.Content, item.Status, item.Priority, encodeIDs(item.Assignees), item.Tags, item.MessageID, item.DueAt, item.CreatedAt).WithContext(ctx).Exec(); err != nil {
		return err
	}
	_ = s.cassandra.Query("INSERT INTO workspace_items_by_id (item_id, channel_id, updated_at, owner_id, kind, title, content, status, priority, assignees, tags, message_id, due_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		item.ID, item.ChannelID, item.UpdatedAt, item.OwnerID, item.Kind, item.Title, item.Content, item.Status, item.Priority, encodeIDs(item.Assignees), item.Tags, item.MessageID, item.DueAt, item.CreatedAt).WithContext(ctx).Exec()
	body, _ := json.Marshal(item)
	_ = s.redis.Set(ctx, "workspace:item:"+item.ID, body, 30*time.Minute).Err()
	eventType := "workspace.item.updated"
	if item.CreatedAt.Equal(item.UpdatedAt) {
		eventType = "workspace.item.created"
	}
	event, _ := realtime.NewEvent(eventType, "workspace-service", item.ID, item)
	if err := realtime.PublishDurably(ctx, s.cassandra, s.events, "workspace", realtime.WorkspaceEventsTopic, event); err != nil {
		return err
	}
	s.hub.BroadcastToChannel(item.ChannelID, gin.H{"type": eventType, "data": item})
	return nil
}

func (s *Service) getItem(ctx context.Context, id string) (Item, error) {
	var item Item
	var assignees string
	err := s.cassandra.Query("SELECT item_id, channel_id, owner_id, kind, title, content, status, priority, assignees, tags, message_id, due_at, created_at, updated_at FROM workspace_items_by_id WHERE item_id = ?", id).WithContext(ctx).
		Scan(&item.ID, &item.ChannelID, &item.OwnerID, &item.Kind, &item.Title, &item.Content, &item.Status, &item.Priority, &assignees, &item.Tags, &item.MessageID, &item.DueAt, &item.CreatedAt, &item.UpdatedAt)
	item.Assignees = decodeIDs(assignees)
	return item, err
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
	group := engine.Group("/api/workspace")
	group.Use(realtime.RequireIdentity(s.cassandra, realtime.RedisSessionValidator(s.redis)))
	group.GET("/items", s.listItems)
	group.POST("/items", s.createItem)
	group.GET("/items/:id", s.getItemAPI)
	group.PUT("/items/:id", s.updateItem)
	group.DELETE("/items/:id", s.deleteItem)
	group.PUT("/items/:id/status", s.updateStatus)
	group.PUT("/items/:id/assignees", s.updateAssignees)
	group.POST("/items/:id/checklist", s.createChecklist)
	group.PUT("/items/:id/checklist/:checklistId", s.updateChecklist)
	group.DELETE("/items/:id/checklist/:checklistId", s.deleteChecklist)
	group.GET("/boards/:channelId", s.listItems)
	group.GET("/bookmarks", s.listBookmarks)
	group.POST("/collections", s.createCollection)
	group.PUT("/collections/:id", s.updateCollection)
	group.DELETE("/collections/:id", s.deleteCollection)
	group.GET("/reminders/due", s.dueReminders)
	group.GET("/ws", s.websocket)
	return engine
}

func (s *Service) listItems(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	channelID := identity.ChannelID
	if value := c.Param("channelId"); value != "" {
		if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
			if identity.ChannelID != 0 && parsed != identity.ChannelID {
				c.JSON(http.StatusForbidden, gin.H{"message": "channel access denied"})
				return
			}
			channelID = parsed
		}
	}
	iter := s.cassandra.Query("SELECT item_id, updated_at, owner_id, kind, title, content, status, priority, assignees, tags, message_id, due_at, created_at FROM workspace_items_by_channel WHERE channel_id = ? LIMIT 200", channelID).WithContext(c.Request.Context()).Iter()
	items := make([]Item, 0)
	var item Item
	var assignees string
	for iter.Scan(&item.ID, &item.UpdatedAt, &item.OwnerID, &item.Kind, &item.Title, &item.Content, &item.Status, &item.Priority, &assignees, &item.Tags, &item.MessageID, &item.DueAt, &item.CreatedAt) {
		item.ChannelID, item.Assignees = channelID, decodeIDs(assignees)
		items = append(items, item)
		item, assignees = Item{}, ""
	}
	_ = iter.Close()
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Service) createItem(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var item Item
	if c.ShouldBindJSON(&item) != nil || strings.TrimSpace(item.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "title is required"})
		return
	}
	item.ChannelID, item.OwnerID = identity.ChannelID, identity.UserID
	// Generate the identifier before saving so the response and follow-up API
	// calls expose the same durable item identity. saveItem also accepts a value
	// because update paths intentionally persist a complete snapshot.
	if item.ID == "" {
		item.ID = uuid.NewString()
		item.CreatedAt = time.Now().UTC()
	}
	// The channel in the signed token is authoritative. Never allow a client
	// supplied query/body value to redirect a write into another channel.
	if err := s.saveItem(c.Request.Context(), item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (s *Service) getItemAPI(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	item, err := s.getItem(c.Request.Context(), c.Param("id"))
	if err != nil || item.ChannelID != identity.ChannelID {
		c.JSON(http.StatusNotFound, gin.H{"message": "item not found"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Service) updateItem(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	item, err := s.getItem(c.Request.Context(), c.Param("id"))
	if err != nil || item.ChannelID != identity.ChannelID || item.OwnerID != identity.UserID {
		c.JSON(http.StatusNotFound, gin.H{"message": "item not found"})
		return
	}
	var patch Item
	if c.ShouldBindJSON(&patch) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid item"})
		return
	}
	patch.ID, patch.ChannelID, patch.OwnerID, patch.CreatedAt = item.ID, item.ChannelID, item.OwnerID, item.CreatedAt
	if patch.Kind == "" {
		patch.Kind = item.Kind
	}
	if patch.Title == "" {
		patch.Title = item.Title
	}
	if patch.Content == "" {
		patch.Content = item.Content
	}
	if patch.Status == "" {
		patch.Status = item.Status
	}
	if patch.Priority == "" {
		patch.Priority = item.Priority
	}
	if len(patch.Assignees) == 0 {
		patch.Assignees = item.Assignees
	}
	if len(patch.Tags) == 0 {
		patch.Tags = item.Tags
	}
	if err := s.saveItem(c.Request.Context(), patch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, patch)
}

func (s *Service) deleteItem(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	item, err := s.getItem(c.Request.Context(), c.Param("id"))
	if err != nil || item.ChannelID != identity.ChannelID || item.OwnerID != identity.UserID {
		c.Status(http.StatusNotFound)
		return
	}
	_ = s.cassandra.Query("DELETE FROM workspace_items_by_id WHERE item_id = ?", item.ID).WithContext(c.Request.Context()).Exec()
	_ = s.cassandra.Query("DELETE FROM workspace_items_by_channel WHERE channel_id = ? AND updated_at = ? AND item_id = ?", item.ChannelID, item.UpdatedAt, item.ID).WithContext(c.Request.Context()).Exec()
	_ = s.redis.Del(c.Request.Context(), "workspace:item:"+item.ID).Err()
	event, _ := realtime.NewEvent("workspace.item.deleted", "workspace-service", item.ID, item)
	_ = realtime.PublishDurably(c.Request.Context(), s.cassandra, s.events, "workspace", realtime.WorkspaceEventsTopic, event)
	s.hub.BroadcastToChannel(item.ChannelID, gin.H{"type": "workspace.item.deleted", "data": item})
	c.Status(http.StatusNoContent)
}

func (s *Service) updateStatus(c *gin.Context) {
	var request struct {
		Status string `json:"status"`
	}
	if c.ShouldBindJSON(&request) != nil || (request.Status != "todo" && request.Status != "in_progress" && request.Status != "done" && request.Status != "archived") {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid status"})
		return
	}
	identity := realtime.IdentityFrom(c)
	item, err := s.getItem(c.Request.Context(), c.Param("id"))
	if err != nil || item.ChannelID != identity.ChannelID || item.OwnerID != identity.UserID {
		c.Status(http.StatusNotFound)
		return
	}
	item.Status = request.Status
	if err := s.saveItem(c.Request.Context(), item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "update status failed"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Service) updateAssignees(c *gin.Context) {
	var request struct {
		Assignees []uint64 `json:"assignees"`
	}
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid assignees"})
		return
	}
	identity := realtime.IdentityFrom(c)
	item, err := s.getItem(c.Request.Context(), c.Param("id"))
	if err != nil || item.ChannelID != identity.ChannelID || item.OwnerID != identity.UserID {
		c.Status(http.StatusNotFound)
		return
	}
	item.Assignees = request.Assignees
	if err := s.saveItem(c.Request.Context(), item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "update assignees failed"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (s *Service) listBookmarks(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	iter := s.cassandra.Query("SELECT item_id, created_at, channel_id, title, tags FROM workspace_bookmarks_by_user WHERE user_id = ? LIMIT 100", identity.UserID).WithContext(c.Request.Context()).Iter()
	items := make([]gin.H, 0)
	var itemID string
	var created time.Time
	var channelID uint64
	var title string
	var tags []string
	for iter.Scan(&itemID, &created, &channelID, &title, &tags) {
		items = append(items, gin.H{"id": itemID, "created_at": created, "channel_id": strconv.FormatUint(channelID, 10), "title": title, "tags": tags})
	}
	_ = iter.Close()
	c.JSON(http.StatusOK, gin.H{"bookmarks": items})
}

func (s *Service) createCollection(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request struct {
		Name string `json:"name"`
	}
	if c.ShouldBindJSON(&request) != nil || strings.TrimSpace(request.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "name is required"})
		return
	}
	id := uuid.NewString()
	_ = s.cassandra.Query("INSERT INTO workspace_collections_by_user (user_id, collection_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)", identity.UserID, id, request.Name, time.Now().UTC(), time.Now().UTC()).WithContext(c.Request.Context()).Exec()
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": request.Name})
}

func (s *Service) updateCollection(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	var request struct {
		Name string `json:"name"`
	}
	if c.ShouldBindJSON(&request) != nil || strings.TrimSpace(request.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "name is required"})
		return
	}
	now := time.Now().UTC()
	if err := s.cassandra.Query("UPDATE workspace_collections_by_user SET name = ?, updated_at = ? WHERE user_id = ? AND collection_id = ?", request.Name, now, identity.UserID, c.Param("id")).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "update collection failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": c.Param("id"), "name": request.Name, "updated_at": now})
}

func (s *Service) deleteCollection(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	if err := s.cassandra.Query("DELETE FROM workspace_collections_by_user WHERE user_id = ? AND collection_id = ?", identity.UserID, c.Param("id")).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "delete collection failed"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Service) createChecklist(c *gin.Context) {
	var request struct {
		Text string `json:"text"`
	}
	if c.ShouldBindJSON(&request) != nil || strings.TrimSpace(request.Text) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "text is required"})
		return
	}
	identity := realtime.IdentityFrom(c)
	item, err := s.getItem(c.Request.Context(), c.Param("id"))
	if err != nil || item.ChannelID != identity.ChannelID || item.OwnerID != identity.UserID {
		c.JSON(http.StatusNotFound, gin.H{"message": "item not found"})
		return
	}
	id, now := uuid.NewString(), time.Now().UTC()
	if err := s.cassandra.Query("INSERT INTO workspace_checklists_by_item (item_id, checklist_id, text, done, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)", item.ID, id, request.Text, false, now, now).WithContext(c.Request.Context()).Exec(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": "create checklist failed"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "item_id": item.ID, "text": request.Text, "done": false, "created_at": now})
}

func (s *Service) updateChecklist(c *gin.Context) {
	var request struct {
		Text string `json:"text"`
		Done *bool  `json:"done"`
	}
	if c.ShouldBindJSON(&request) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "invalid checklist"})
		return
	}
	identity := realtime.IdentityFrom(c)
	item, err := s.getItem(c.Request.Context(), c.Param("id"))
	if err != nil || item.ChannelID != identity.ChannelID || item.OwnerID != identity.UserID {
		c.JSON(http.StatusNotFound, gin.H{"message": "item not found"})
		return
	}
	var text string
	var done bool
	if err := s.cassandra.Query("SELECT text, done FROM workspace_checklists_by_item WHERE item_id = ? AND checklist_id = ?", item.ID, c.Param("checklistId")).WithContext(c.Request.Context()).Scan(&text, &done); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "checklist not found"})
		return
	}
	if request.Text != "" {
		text = request.Text
	}
	if request.Done != nil {
		done = *request.Done
	}
	now := time.Now().UTC()
	_ = s.cassandra.Query("UPDATE workspace_checklists_by_item SET text = ?, done = ?, updated_at = ? WHERE item_id = ? AND checklist_id = ?", text, done, now, item.ID, c.Param("checklistId")).WithContext(c.Request.Context()).Exec()
	c.JSON(http.StatusOK, gin.H{"id": c.Param("checklistId"), "item_id": item.ID, "text": text, "done": done, "updated_at": now})
}

func (s *Service) deleteChecklist(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	item, err := s.getItem(c.Request.Context(), c.Param("id"))
	if err != nil || item.ChannelID != identity.ChannelID || item.OwnerID != identity.UserID {
		c.Status(http.StatusNotFound)
		return
	}
	_ = s.cassandra.Query("DELETE FROM workspace_checklists_by_item WHERE item_id = ? AND checklist_id = ?", item.ID, c.Param("checklistId")).WithContext(c.Request.Context()).Exec()
	c.Status(http.StatusNoContent)
}

func (s *Service) dueReminders(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	now := float64(time.Now().UTC().Unix())
	values, _ := s.redis.ZRangeByScore(c.Request.Context(), fmt.Sprintf("workspace:reminders:%d", identity.UserID), &redis.ZRangeBy{Min: "-inf", Max: fmt.Sprintf("%f", now), Offset: 0, Count: 100}).Result()
	c.JSON(http.StatusOK, gin.H{"user_id": strconv.FormatUint(identity.UserID, 10), "reminders": values})
}

func (s *Service) websocket(c *gin.Context) {
	identity := realtime.IdentityFrom(c)
	protocol := ""
	for _, value := range strings.Split(c.GetHeader("Sec-WebSocket-Protocol"), ",") {
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "nexuschat-channel.") {
			protocol = value
			break
		}
	}
	localUpgrader := upgrader
	if protocol != "" {
		localUpgrader.Subprotocols = []string{protocol}
	}
	conn, err := localUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &realtime.Client{UserID: identity.UserID, ChannelID: identity.ChannelID, Device: "workspace", Conn: conn, Send: make(chan []byte, 64)}
	s.hub.Add(client)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for body := range client.Send {
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteMessage(websocket.TextMessage, body) != nil {
				return
			}
		}
	}()
	conn.SetReadLimit(4096)
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

func uintField(req *structpb.Struct, name string) uint64 {
	value := req.GetFields()[name]
	if value == nil {
		return 0
	}
	if value.GetStringValue() != "" {
		id, _ := strconv.ParseUint(value.GetStringValue(), 10, 64)
		return id
	}
	return uint64(value.GetNumberValue())
}

func (s *Service) isChannelMember(ctx context.Context, channelID, userID uint64) (bool, error) {
	if channelID == 0 || userID == 0 {
		return false, nil
	}
	var member uint64
	err := s.cassandra.Query("SELECT user_id FROM channels WHERE id = ? AND user_id = ? LIMIT 1", channelID, userID).WithContext(ctx).Scan(&member)
	if errors.Is(err, gocql.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return member == userID, nil
}

func (s *Service) grpcItemAccess(ctx context.Context, req *structpb.Struct, item Item) (bool, error) {
	channelID, userID := uintField(req, "channel_id"), uintField(req, "user_id")
	if channelID == 0 || userID == 0 || item.ChannelID != channelID {
		return false, nil
	}
	member, err := s.isChannelMember(ctx, channelID, userID)
	if err != nil || !member {
		return false, err
	}
	return item.OwnerID == userID || contains(item.Assignees, userID), nil
}

func (s *Service) grpcMethods() map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error) {
	return map[string]func(context.Context, *structpb.Struct) (*structpb.Struct, error){
		"GetWorkspaceItem": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			item, err := s.getItem(ctx, req.GetFields()["item_id"].GetStringValue())
			if err != nil {
				return nil, err
			}
			authorized, err := s.grpcItemAccess(ctx, req, item)
			if err != nil || !authorized {
				return nil, fmt.Errorf("workspace item access denied")
			}
			body, _ := json.Marshal(item)
			var value map[string]any
			_ = json.Unmarshal(body, &value)
			return structpb.NewStruct(value)
		},
		"BatchGetWorkspaceItems": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			items := make([]any, 0)
			for _, id := range strings.Split(req.GetFields()["item_ids"].GetStringValue(), ",") {
				item, err := s.getItem(ctx, strings.TrimSpace(id))
				if err != nil {
					continue
				}
				authorized, accessErr := s.grpcItemAccess(ctx, req, item)
				if accessErr != nil {
					return nil, accessErr
				}
				if !authorized {
					continue
				}
				body, _ := json.Marshal(item)
				var value map[string]any
				_ = json.Unmarshal(body, &value)
				items = append(items, value)
			}
			return structpb.NewStruct(map[string]any{"items": items})
		},
		"CreateItemFromMessage": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			channelID, userID := uintField(req, "channel_id"), uintField(req, "user_id")
			member, err := s.isChannelMember(ctx, channelID, userID)
			if err != nil {
				return nil, err
			}
			if !member {
				return nil, fmt.Errorf("workspace channel access denied")
			}
			item := Item{ChannelID: channelID, OwnerID: userID, MessageID: uintField(req, "message_id"), Kind: "bookmark", Title: "Saved message", Content: req.GetFields()["content"].GetStringValue()}
			if err := s.saveItem(ctx, item); err != nil {
				return nil, err
			}
			return structpb.NewStruct(map[string]any{"item_id": item.ID})
		},
		"AuthorizeWorkspaceItem": func(ctx context.Context, req *structpb.Struct) (*structpb.Struct, error) {
			item, err := s.getItem(ctx, req.GetFields()["item_id"].GetStringValue())
			if err != nil {
				return nil, err
			}
			authorized, err := s.grpcItemAccess(ctx, req, item)
			if err != nil {
				return nil, err
			}
			return structpb.NewStruct(map[string]any{"authorized": authorized})
		},
	}
}

func contains(values []uint64, target uint64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *Service) Run(ctx context.Context) error {
	if err := common.NewObservabilityInjector(s.config).Register("workspace"); err != nil {
		return err
	}
	s.httpServer = &http.Server{Addr: ":" + s.config.Workspace.Http.Server.Port, Handler: common.NewOtelHttpHandler(s.routes(), "workspace_http"), ReadHeaderTimeout: 10 * time.Second}
	s.grpcServer = realtime.NewGRPCServer("nexuschat.workspace.v1.WorkspaceService", s.grpcMethods())
	go realtime.RelayOutbox(ctx, s.cassandra, s.events, "workspace")
	errorsCh := make(chan error, 2)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsCh <- err
		}
	}()
	go func() { errorsCh <- realtime.ServeGRPC(s.grpcServer, s.config.Workspace.Grpc.Server.Port) }()
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
		slog.Error("initialize workspace", "error", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if err := service.Run(ctx); err != nil {
		slog.Error("workspace stopped", "error", err)
		os.Exit(1)
	}
}
