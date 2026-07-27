package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type integrationClient struct {
	http *http.Client
	base string
	id   string
}

func TestTwoUserChatLifecycle(t *testing.T) {
	if os.Getenv("NEXUSCHAT_INTEGRATION") != "1" {
		t.Skip("set NEXUSCHAT_INTEGRATION=1 to run against the Docker stack")
	}
	base := strings.TrimRight(os.Getenv("NEXUSCHAT_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost"
	}
	a := newIntegrationClient(t, base)
	b := newIntegrationClient(t, base)
	a.signup(t, "nxa"+fmt.Sprint(time.Now().UnixNano()), "User A")
	b.signup(t, "nxb"+fmt.Sprint(time.Now().UnixNano()), "User B")

	// Group invite is independent of friendship and must work before either
	// user is blocked.
	room := a.createRoom(t)
	b.joinRoom(t, room["invite_code"].(string))

	// Pending outgoing/incoming: direct chat is forbidden before acceptance.
	a.sendFriendRequest(t, b.id)
	a.assertRelationship(t, b.id, "pending_outgoing")
	b.assertRelationship(t, a.id, "pending_incoming")
	a.assertDirectStatus(t, b.id, http.StatusForbidden)

	// Declined is observable, and a later request can be sent again.
	b.decline(t, a.id)
	a.assertRelationship(t, b.id, "declined")
	a.sendFriendRequest(t, b.id)
	b.accept(t, a.id)
	a.assertRelationship(t, b.id, "accepted")
	b.assertRelationship(t, a.id, "accepted")
	direct := a.createDirect(t, b.id)
	if direct["access_token"] == "" {
		t.Fatal("direct channel did not return an access token")
	}
	concurrent := a.createDirectConcurrently(t, b.id)
	if concurrent[0]["channel_id"] != concurrent[1]["channel_id"] {
		t.Fatalf("concurrent direct creation returned different channels: %#v", concurrent)
	}
	directToken := direct["access_token"].(string)
	ticket := a.issueChatTicket(t, directToken)
	chatConn := a.openChatTicket(t, ticket["ticket"].(string))
	chatConn.Close()
	if replayConn, _, err := a.dialChatTicket(ticket["ticket"].(string)); err == nil {
		_ = replayConn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, readErr := replayConn.ReadMessage()
		_ = replayConn.Close()
		if readErr == nil {
			t.Fatal("one-time chat ticket was accepted twice")
		}
	}

	// Random matching remains available for strangers/features unrelated to
	// friendship. The test only verifies both sockets receive a match result.
	matchA := a.openMatch(t)
	matchB := b.openMatch(t)
	defer matchA.Close()
	defer matchB.Close()
	readMatch(t, matchA)
	readMatch(t, matchB)

	// A block revokes permission for both new opens and the existing direct
	// channel's HTTP authorization path.
	b.block(t, a.id, direct["access_token"].(string))
	a.assertDirectStatus(t, b.id, http.StatusForbidden)
	b.assertDirectTokenForbidden(t, direct["channel_id"].(string), direct["access_token"].(string))
}

func newIntegrationClient(t *testing.T, base string) *integrationClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &integrationClient{http: &http.Client{Jar: jar, Timeout: 15 * time.Second}, base: base}
}

func (c *integrationClient) signup(t *testing.T, username, name string) {
	t.Helper()
	body := map[string]string{"username": username, "password": "NexusChat123!", "display_name": name}
	var result struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	c.request(t, http.MethodPost, "/api/user/signup", body, nil, http.StatusCreated, &result)
	c.id = result.User.ID
	if c.id == "" {
		t.Fatal("signup returned no user id")
	}
}

func (c *integrationClient) sendFriendRequest(t *testing.T, id string) {
	c.request(t, http.MethodPost, "/api/user/friends/requests", map[string]string{"user_id": id}, nil, http.StatusCreated, nil)
}
func (c *integrationClient) accept(t *testing.T, from string) {
	c.request(t, http.MethodPost, "/api/user/friends/requests/"+from+"/accept", nil, nil, http.StatusOK, nil)
}
func (c *integrationClient) decline(t *testing.T, from string) {
	c.request(t, http.MethodPost, "/api/user/friends/requests/"+from+"/decline", nil, nil, http.StatusOK, nil)
}
func (c *integrationClient) assertRelationship(t *testing.T, peer, expected string) {
	var result struct {
		Status string `json:"status"`
	}
	c.request(t, http.MethodGet, "/api/user/friends/status/"+peer, nil, nil, http.StatusOK, &result)
	if result.Status != expected {
		t.Fatalf("relationship %s -> %s, want %s", c.id, result.Status, expected)
	}
}
func (c *integrationClient) assertDirectStatus(t *testing.T, peer string, expected int) {
	c.request(t, http.MethodPost, "/api/chat/direct", map[string]string{"user_id": peer}, nil, expected, nil)
}
func (c *integrationClient) createDirect(t *testing.T, peer string) map[string]any {
	var result map[string]any
	c.request(t, http.MethodPost, "/api/chat/direct", map[string]string{"user_id": peer}, nil, http.StatusOK, &result)
	return result
}

func (c *integrationClient) createDirectConcurrently(t *testing.T, peer string) []map[string]any {
	t.Helper()
	type result struct {
		value map[string]any
		err   error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			body, err := json.Marshal(map[string]string{"user_id": peer})
			if err != nil {
				results <- result{err: err}
				return
			}
			req, err := http.NewRequest(http.MethodPost, c.base+"/api/chat/direct", bytes.NewReader(body))
			if err != nil {
				results <- result{err: err}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := c.http.Do(req)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer res.Body.Close()
			data, readErr := io.ReadAll(res.Body)
			if readErr != nil {
				results <- result{err: readErr}
				return
			}
			if res.StatusCode != http.StatusOK {
				results <- result{err: fmt.Errorf("concurrent direct creation returned status %d: %s", res.StatusCode, data)}
				return
			}
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil {
				results <- result{err: err}
				return
			}
			results <- result{value: value}
		}()
	}
	values := make([]map[string]any, 0, 2)
	for i := 0; i < 2; i++ {
		item := <-results
		if item.err != nil {
			t.Fatal(item.err)
		}
		values = append(values, item.value)
	}
	return values
}
func (c *integrationClient) createRoom(t *testing.T) map[string]any {
	var result map[string]any
	c.request(t, http.MethodPost, "/api/chat/rooms", map[string]any{"name": "integration-room"}, nil, http.StatusCreated, &result)
	return result
}
func (c *integrationClient) joinRoom(t *testing.T, code string) {
	c.request(t, http.MethodPost, "/api/chat/rooms/join", map[string]string{"invite_code": code}, nil, http.StatusOK, nil)
}
func (c *integrationClient) block(t *testing.T, peer, token string) {
	c.request(t, http.MethodPost, "/api/safety/blocks", map[string]string{"user_id": peer}, map[string]string{"Authorization": "Bearer " + token}, http.StatusNoContent, nil)
}
func (c *integrationClient) assertDirectTokenForbidden(t *testing.T, channel, token string) {
	c.request(t, http.MethodGet, "/api/chat/channel/messages", nil, map[string]string{"Authorization": "Bearer " + token}, http.StatusForbidden, nil)
}
func (c *integrationClient) issueChatTicket(t *testing.T, channelToken string) map[string]any {
	t.Helper()
	var result map[string]any
	c.request(t, http.MethodPost, "/api/chat/channel/ws-ticket", nil, map[string]string{"Authorization": "Bearer " + channelToken}, http.StatusOK, &result)
	return result
}

func (c *integrationClient) dialChatTicket(ticket string) (*websocket.Conn, *http.Response, error) {
	wsURL := strings.Replace(c.base, "http://", "ws://", 1) + "/api/chat?ticket=" + url.QueryEscape(ticket)
	req := http.Header{}
	baseURL, _ := url.Parse(c.base)
	cookies := c.http.Jar.Cookies(baseURL)
	for _, cookie := range cookies {
		req.Add("Cookie", cookie.Name+"="+cookie.Value)
	}
	return websocket.DefaultDialer.Dial(wsURL, req)
}

func (c *integrationClient) openChatTicket(t *testing.T, ticket string) *websocket.Conn {
	t.Helper()
	conn, _, err := c.dialChatTicket(ticket)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func (c *integrationClient) openMatch(t *testing.T) *websocket.Conn {
	t.Helper()
	url := strings.Replace(c.base, "http://", "ws://", 1) + "/api/match"
	req := http.Header{}
	cookies := c.http.Jar.Cookies(mustURL(t, c.base))
	for _, cookie := range cookies {
		req.Add("Cookie", cookie.Name+"="+cookie.Value)
	}
	conn, _, err := websocket.DefaultDialer.Dial(url, req)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}
func readMatch(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	_, body, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if json.Unmarshal(body, &result) != nil || result["channel_id"] == nil {
		t.Fatalf("invalid match result: %s", body)
	}
}

func (c *integrationClient) request(t *testing.T, method, path string, body any, headers map[string]string, expected int, out any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != expected {
		t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, expected, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatal(err)
		}
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestRuntimeReadStateAndAuthorization(t *testing.T) {
	if os.Getenv("NEXUSCHAT_INTEGRATION") != "1" {
		t.Skip("set NEXUSCHAT_INTEGRATION=1 to run against the Docker stack")
	}
	base := strings.TrimRight(os.Getenv("NEXUSCHAT_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost"
	}
	a := newIntegrationClient(t, base)
	b := newIntegrationClient(t, base)
	c := newIntegrationClient(t, base)
	suffix := fmt.Sprint(time.Now().UnixNano())
	a.signup(t, "nxa"+suffix, "Read State A")
	b.signup(t, "nxb"+suffix, "Read State B")
	c.signup(t, "nxc"+suffix, "Safety Viewer C")

	readRoom := a.createRoomWithMembers(t, "read-state-room", []string{b.id})
	aRead := a.openRoom(t, readRoom["channel_id"].(string))
	bRead := b.openRoom(t, readRoom["channel_id"].(string))
	readConn := b.openChatTicket(t, b.issueChatTicket(t, bRead)["ticket"].(string))
	defer readConn.Close()
	first := sendTextAndReadID(t, readConn, b.id, "first durable message")
	second := sendTextAndReadID(t, readConn, b.id, "second durable message")
	if first == second {
		t.Fatalf("runtime messages returned the same id: %q", first)
	}
	a.markRead(t, aRead, second)
	a.markRead(t, aRead, first)
	state := a.readState(t, aRead)
	if state != second {
		t.Fatalf("read state regressed after out-of-order updates: got %q, want %q", state, second)
	}

	workspaceA := a.createRoom(t)
	workspaceB := a.createRoom(t)
	workspaceAToken := a.openRoom(t, workspaceA["channel_id"].(string))
	workspaceBToken := a.openRoom(t, workspaceB["channel_id"].(string))
	item := a.createWorkspaceItem(t, workspaceAToken)
	a.request(t, http.MethodGet, "/api/workspace/boards/"+workspaceA["channel_id"].(string), nil, map[string]string{"Authorization": "Bearer " + workspaceBToken}, http.StatusForbidden, nil)
	a.request(t, http.MethodGet, "/api/workspace/items/"+item["id"].(string), nil, map[string]string{"Authorization": "Bearer " + workspaceBToken}, http.StatusNotFound, nil)

	safetyRoom := a.createRoomWithMembers(t, "safety-access-room", []string{c.id})
	aSafety := a.openRoom(t, safetyRoom["channel_id"].(string))
	cSafety := c.openRoom(t, safetyRoom["channel_id"].(string))
	var report map[string]any
	a.request(t, http.MethodPost, "/api/safety/reports", map[string]string{"target_user_id": b.id, "message_id": "0", "reason": "runtime authorization check", "details": "integration test"}, map[string]string{"Authorization": "Bearer " + aSafety}, http.StatusCreated, &report)
	c.request(t, http.MethodGet, "/api/safety/reports/"+report["id"].(string), nil, map[string]string{"Authorization": "Bearer " + cSafety}, http.StatusForbidden, nil)
}

func (c *integrationClient) createRoomWithMembers(t *testing.T, name string, members []string) map[string]any {
	t.Helper()
	var result map[string]any
	c.request(t, http.MethodPost, "/api/chat/rooms", map[string]any{"name": name, "member_ids": members}, nil, http.StatusCreated, &result)
	return result
}

func (c *integrationClient) openRoom(t *testing.T, channelID string) string {
	t.Helper()
	var result struct {
		AccessToken string `json:"access_token"`
	}
	c.request(t, http.MethodPost, "/api/chat/rooms/"+channelID+"/open", nil, nil, http.StatusOK, &result)
	if result.AccessToken == "" {
		t.Fatal("open room returned no access token")
	}
	return result.AccessToken
}

func (c *integrationClient) markRead(t *testing.T, token, messageID string) {
	t.Helper()
	c.request(t, http.MethodPost, "/api/chat/channel/read-state", map[string]string{"message_id": messageID}, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK, nil)
}

func (c *integrationClient) readState(t *testing.T, token string) string {
	t.Helper()
	var result struct {
		LastReadMessageID string `json:"last_read_message_id"`
	}
	c.request(t, http.MethodGet, "/api/chat/channel/read-state", nil, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK, &result)
	return result.LastReadMessageID
}

func (c *integrationClient) createWorkspaceItem(t *testing.T, token string) map[string]any {
	t.Helper()
	var result map[string]any
	c.request(t, http.MethodPost, "/api/workspace/items", map[string]string{"kind": "note", "title": "Channel A item", "content": "authorization test"}, map[string]string{"Authorization": "Bearer " + token}, http.StatusCreated, &result)
	return result
}

func sendTextAndReadID(t *testing.T, conn *websocket.Conn, userID, payload string) string {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{"event": 0, "user_id": userID, "payload": payload}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		_, body, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		var message struct {
			Event     int    `json:"event"`
			MessageID string `json:"message_id"`
		}
		if json.Unmarshal(body, &message) == nil && message.Event == 0 && message.MessageID != "" {
			return message.MessageID
		}
	}
}
