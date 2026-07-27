package realtime

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

type Client struct {
	UserID    uint64
	ChannelID uint64
	Device    string
	Conn      *websocket.Conn
	Send      chan []byte
}

type Hub struct {
	mu      sync.RWMutex
	clients map[uint64]map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[uint64]map[*Client]struct{})}
}

func (h *Hub) Add(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.UserID] == nil {
		h.clients[client.UserID] = make(map[*Client]struct{})
	}
	h.clients[client.UserID][client] = struct{}{}
}

func (h *Hub) Remove(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients := h.clients[client.UserID]; clients != nil {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.clients, client.UserID)
		}
	}
}

func (h *Hub) Send(userID uint64, value any) bool {
	body, err := json.Marshal(value)
	if err != nil {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	delivered := false
	for client := range h.clients[userID] {
		select {
		case client.Send <- body:
			delivered = true
		default:
		}
	}
	return delivered
}

func (h *Hub) BroadcastToChannel(channelID uint64, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, clients := range h.clients {
		for client := range clients {
			if client.ChannelID != channelID {
				continue
			}
			select {
			case client.Send <- body:
			default:
			}
		}
	}
}

func (h *Hub) Broadcast(value any) {
	body, err := json.Marshal(value)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, clients := range h.clients {
		for client := range clients {
			select {
			case client.Send <- body:
			default:
			}
		}
	}
}

func (h *Hub) Online(userID uint64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, clients := range h.clients {
		for client := range clients {
			close(client.Send)
			_ = client.Conn.Close()
		}
	}
	h.clients = make(map[uint64]map[*Client]struct{})
}
