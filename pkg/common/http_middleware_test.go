package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestJWTForwardAuthUsesChannelHeader(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(JWTForwardAuth())
	router.GET("/", func(c *gin.Context) {
		channelID, ok := c.Request.Context().Value(ChannelKey).(uint64)
		if !ok {
			t.Fatalf("channel id missing from context")
		}
		if channelID != 42 {
			t.Fatalf("expected channel id 42, got %d", channelID)
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(ChannelIdHeader, "42")
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
}

func TestJWTForwardAuthFallsBackToAuthorizationToken(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	JwtSecret = "test-secret"
	JwtExpirationSecond = 3600

	token, err := NewJWT(99)
	if err != nil {
		t.Fatalf("create jwt: %v", err)
	}

	router := gin.New()
	router.Use(JWTForwardAuth())
	router.GET("/", func(c *gin.Context) {
		channelID, ok := c.Request.Context().Value(ChannelKey).(uint64)
		if !ok {
			t.Fatalf("channel id missing from context")
		}
		if channelID != 99 {
			t.Fatalf("expected channel id 99, got %d", channelID)
		}
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(JWTAuthHeader, "Bearer "+token)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}
}

func TestJWTForwardAuthRejectsMissingCredentials(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(JWTForwardAuth())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, res.Code)
	}
}
