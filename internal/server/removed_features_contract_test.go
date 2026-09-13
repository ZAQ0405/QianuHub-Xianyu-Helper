package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// assertRemovedServerEndpoint 验证个人版不会注册已移除功能的 HTTP 入口。
func assertRemovedServerEndpoint(t *testing.T, method, path string) {
	t.Helper()
	srv, _, cleanup := newTestServer(t)
	defer cleanup()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	srv.Router().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("已移除接口仍可访问: %s %s status=%d", method, path, rec.Code)
	}
}
