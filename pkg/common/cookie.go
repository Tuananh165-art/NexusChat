package common

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	OAuthStateCookieName string = "oauthstate"
	SessionIdCookieName  string = "sid"
)

func GenerateStateOauthCookie(c *gin.Context, maxAge int, path, domain string, secure bool, httpOnly bool, sameSite string) (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("generate oauth state cookie error: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(b)
	setCookie(c, OAuthStateCookieName, state, maxAge, path, domain, secure, httpOnly, sameSite)
	return state, nil
}

func SetAuthCookie(c *gin.Context, sessionID string, maxAge int, path, domain string, secure bool, httpOnly bool, sameSite string) {
	setCookie(c, SessionIdCookieName, sessionID, maxAge, path, domain, secure, httpOnly, sameSite)
}

func ClearCookie(c *gin.Context, name, path, domain string, secure bool, httpOnly bool, sameSite string) {
	setCookie(c, name, "", -1, path, domain, secure, httpOnly, sameSite)
}

func setCookie(c *gin.Context, name, value string, maxAge int, path, domain string, secure, httpOnly bool, sameSite string) {
	c.SetSameSite(parseSameSite(sameSite))
	c.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
}

func parseSameSite(value string) http.SameSite {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func GetCookie(c *gin.Context, name string) (string, error) {
	cookie, err := c.Request.Cookie(name)
	if err != nil {
		return "", fmt.Errorf("get oauth state cookie error: %w", err)
	}
	unescapedCookie, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return "", fmt.Errorf("unescape oauth state cookie error: %w", err)
	}
	return unescapedCookie, nil
}
