package realtime

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
	"github.com/redis/go-redis/v9"
)

type Identity struct {
	UserID    uint64
	ChannelID uint64
}

// SessionValidator resolves the authenticated user from the HttpOnly session
// cookie. It deliberately does not accept a user id supplied by the client.
type SessionValidator func(context.Context, string) (uint64, error)

func RedisSessionValidator(client redis.UniversalClient) SessionValidator {
	return func(ctx context.Context, sid string) (uint64, error) {
		if strings.TrimSpace(sid) == "" {
			return 0, errors.New("empty session")
		}
		value, err := client.Get(ctx, common.Join("rc:session", ":", sid)).Uint64()
		if err != nil || value == 0 {
			return 0, errors.New("invalid session")
		}
		return value, nil
	}
}

func authenticatedSessionUser(c *gin.Context, validators []SessionValidator) (uint64, error) {
	if len(validators) == 0 || validators[0] == nil {
		return 0, errors.New("session validator is not configured")
	}
	sid, err := common.GetCookie(c, common.SessionIdCookieName)
	if err != nil {
		return 0, errors.New("missing session cookie")
	}
	return validators[0](c.Request.Context(), sid)
}

func Authenticate(c *gin.Context, session *gocql.Session, validators ...SessionValidator) (Identity, error) {
	userID, err := authenticatedSessionUser(c, validators)
	if err != nil {
		return Identity{}, err
	}
	// Keep legacy parameters only as consistency checks; they can never select
	// the authenticated principal.
	if raw := strings.TrimSpace(c.Query("uid")); raw != "" {
		requested, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil || requested != userID {
			return Identity{}, errors.New("user id does not match session")
		}
	}
	if raw := strings.TrimSpace(c.GetHeader("X-User-Id")); raw != "" {
		requested, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil || requested != userID {
			return Identity{}, errors.New("user id does not match session")
		}
	}
	token := c.Query("access_token")
	if token == "" {
		token = strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	}
	if token == "" {
		for _, protocol := range strings.Split(c.GetHeader("Sec-WebSocket-Protocol"), ",") {
			protocol = strings.TrimSpace(protocol)
			if strings.HasPrefix(protocol, "nexuschat-channel.") {
				token = strings.TrimPrefix(protocol, "nexuschat-channel.")
				break
			}
		}
	}
	auth, err := common.Auth(&common.AuthPayload{AccessToken: token})
	if err != nil || auth.Expired {
		return Identity{}, errors.New("invalid channel token")
	}
	var member uint64
	err = session.Query(
		"SELECT user_id FROM channels WHERE id = ? AND user_id = ? LIMIT 1",
		auth.ChannelID, userID,
	).WithContext(c.Request.Context()).Scan(&member)
	if err != nil || member != userID {
		return Identity{}, errors.New("user is not a channel member")
	}
	return Identity{UserID: userID, ChannelID: auth.ChannelID}, nil
}

func RequireIdentity(session *gocql.Session, validators ...SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := Authenticate(c, session, validators...)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
			return
		}
		c.Set("identity", identity)
		c.Next()
	}
}

func IdentityFrom(c *gin.Context) Identity {
	value, _ := c.Get("identity")
	identity, _ := value.(Identity)
	return identity
}

// RequireUserID authenticates APIs that do not have a channel token yet.
// The user id is always resolved from the HttpOnly session cookie.
func RequireUserID(validators ...SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, err := authenticatedSessionUser(c, validators)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
			return
		}
		if raw := strings.TrimSpace(c.GetHeader("X-User-Id")); raw != "" {
			requested, parseErr := strconv.ParseUint(raw, 10, 64)
			if parseErr != nil || requested != userID {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
				return
			}
		}
		c.Set("identity", Identity{UserID: userID})
		c.Next()
	}
}
