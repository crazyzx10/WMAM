package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestBearerTokenPrefersHeaderAndFallsBackToCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: authCookieName, Value: "cookie-token"})
	ctx.Request = request

	if got := bearerToken(ctx); got != "cookie-token" {
		t.Fatalf("expected cookie token fallback, got %q", got)
	}

	request.Header.Set("Authorization", "Bearer header-token")
	if got := bearerToken(ctx); got != "header-token" {
		t.Fatalf("expected bearer token to win, got %q", got)
	}
}

func TestSetAuthCookieUsesHttpOnlySameSiteLax(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	setAuthCookie(ctx, "token-value", 30*24*time.Hour, true)

	header := recorder.Header().Get("Set-Cookie")
	for _, want := range []string{authCookieName + "=token-value", "Max-Age=2592000", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(header, want) {
			t.Fatalf("expected Set-Cookie header to contain %q, got %q", want, header)
		}
	}
}
