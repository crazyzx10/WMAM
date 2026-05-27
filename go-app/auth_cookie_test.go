package main

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"
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

func TestRedactRuntimeErrorMasksSecretsAndQueryValues(t *testing.T) {
	err := errors.New(`Get "https://example.test/path?access_token=token-value&secret=raw-secret&password=db-pass": raw-secret db-pass`)
	got := redactRuntimeError(err, "raw-secret", "db-pass")

	for _, leaked := range []string{"token-value", "raw-secret", "db-pass"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("expected sensitive value %q to be redacted, got %q", leaked, got)
		}
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("expected redacted marker, got %q", got)
	}
}

func TestJobAuditDescriptionIncludesFailureDetail(t *testing.T) {
	got := jobAuditDescription("初始化 MySQL 表失败", "Error 3822 (HY000): Duplicate check constraint name 'single_row'")
	want := "初始化 MySQL 表失败：Error 3822 (HY000): Duplicate check constraint name 'single_row'"
	if got != want {
		t.Fatalf("jobAuditDescription() = %q, want %q", got, want)
	}
}

func TestJobAuditDescriptionAvoidsDuplicatingSameMessage(t *testing.T) {
	got := jobAuditDescription("任务锁不可用", "任务锁不可用")
	if got != "任务锁不可用" {
		t.Fatalf("jobAuditDescription() = %q", got)
	}
}

func TestGetLatestDataDateUsesChineseDateColumn(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer database.Close()

	if _, err := database.Exec(`CREATE TABLE publisher_adpos_general (小程序ID TEXT, 日期 TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO publisher_adpos_general (小程序ID, 日期) VALUES (?, ?), (?, ?)`, "wx-demo", "2026-05-20", "wx-demo", "2026-05-21"); err != nil {
		t.Fatalf("insert data: %v", err)
	}

	got := getLatestDataDate(database, "publisher_adpos_general", "日期", "wx-demo", "2025-07-01")
	if got != "2026-05-22" {
		t.Fatalf("expected next date after latest data, got %q", got)
	}
}
