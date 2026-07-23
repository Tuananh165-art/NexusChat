package realtime

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Tuananh165-art/NexusChat/pkg/common"
	"github.com/gin-gonic/gin"
	"github.com/gocql/gocql"
)

type Identity struct {
	UserID    uint64
	ChannelID uint64
}

func Authenticate(c *gin.Context, session *gocql.Session) (Identity, error) {
	userValue := c.Query("uid")
	if userValue == "" {
		userValue = c.GetHeader("X-User-Id")
	}
	userID, err := strconv.ParseUint(userValue, 10, 64)
	if err != nil || userID == 0 {
		return Identity{}, errors.New("invalid user id")
	}
	token := c.Query("access_token")
	if token == "" {
		token = strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
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

func RequireIdentity(session *gocql.Session) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, err := Authenticate(c, session)
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
