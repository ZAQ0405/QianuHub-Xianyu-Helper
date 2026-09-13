package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// itemBatchRouteCase 描述一个版本化商品同步或批量发布路由的兼容测试样例。
type itemBatchRouteCase struct {
	// name 是测试样例名称。
	name string
	// method 是请求使用的 HTTP 方法。
	method string
	// versionedPath 是版本化入口路径。
	versionedPath string
	// legacyPath 是旧兼容入口路径。
	legacyPath string
	// body 是两条入口共用的请求体。
	body string
	// wantStatus 是两条入口应返回的状态码。
	wantStatus int
}

// requestItemBatchRoute 发送带认证会话的商品批量路由请求并返回状态码。
func requestItemBatchRoute(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, method, path, body string) int {
	t.Helper()
	// request 是当前商品批量兼容入口请求。
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.AddCookie(sessionCookie)
	// recorder 是捕获商品批量响应的记录器。
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if strings.HasPrefix(path, "/api/v1/") && recorder.Code >= http.StatusOK && recorder.Code < http.StatusMultipleChoices {
		assertOpenAPIRecordedSuccessResponse(t, request, recorder)
	}
	return recorder.Code
}

// TestVersionedItemBatchRoutes 验证商品同步入口保留，批量铺货入口已从路由树移除。
func TestVersionedItemBatchRoutes(t *testing.T) {
	// srv 是用于验证商品同步与批量发布路由的 HTTP 测试服务。
	srv, _, cleanup := newTestServer(t)
	defer cleanup()
	// handler 是当前测试使用的完整路由树。
	handler := srv.Router()
	// sessionCookie 是管理员登录后得到的认证会话。
	sessionCookie := loginHelper(t, handler)
	// cases 是覆盖版本化与旧路径状态码兼容性的测试样例集合。
	cases := []itemBatchRouteCase{
		{name: "sync-all", method: http.MethodPost, versionedPath: "/api/v1/items/get-all-from-account", legacyPath: "/items/get-all-from-account", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "sync-page", method: http.MethodPost, versionedPath: "/api/v1/items/get-by-page", legacyPath: "/items/get-by-page", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "recommend-category", method: http.MethodPost, versionedPath: "/api/v1/items/publish-categories/recommend", legacyPath: "/items/publish-categories/recommend", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "preview-batch-removed", method: http.MethodPost, versionedPath: "/api/v1/items/publish-batches/preview", wantStatus: http.StatusMethodNotAllowed},
		{name: "start-batch-removed", method: http.MethodPost, versionedPath: "/api/v1/items/publish-batches", body: `{}`, wantStatus: http.StatusNotFound},
		{name: "list-batches-removed", method: http.MethodGet, versionedPath: "/api/v1/items/publish-batches", wantStatus: http.StatusNotFound},
		{name: "get-batch-removed", method: http.MethodGet, versionedPath: "/api/v1/items/publish-batches/missing", wantStatus: http.StatusNotFound},
		{name: "download-result-removed", method: http.MethodGet, versionedPath: "/api/v1/items/publish-batches/missing/result.csv", wantStatus: http.StatusNotFound},
		{name: "delete-batch-removed", method: http.MethodDelete, versionedPath: "/api/v1/items/publish-batches/missing", wantStatus: http.StatusNotFound},
		{name: "cancel-batch-removed", method: http.MethodPost, versionedPath: "/api/v1/items/publish-batches/missing/cancel", wantStatus: http.StatusMethodNotAllowed},
		{name: "retry-batch-removed", method: http.MethodPost, versionedPath: "/api/v1/items/publish-batches/missing/retry-failed", wantStatus: http.StatusMethodNotAllowed},
	}
	for _, routeCase := range cases { // routeCase 是当前正在执行的商品批量路由样例。
		// versionedStatus 是版本化入口实际返回的状态码。
		versionedStatus := requestItemBatchRoute(t, handler, sessionCookie, routeCase.method, routeCase.versionedPath, routeCase.body)
		// legacyStatus 仅在仍保留旧入口的样例中读取；已删除功能不再验证旧路径。
		legacyStatus := versionedStatus
		if routeCase.legacyPath != "" {
			legacyStatus = requestItemBatchRoute(t, handler, sessionCookie, routeCase.method, routeCase.legacyPath, routeCase.body)
		}
		if versionedStatus != routeCase.wantStatus || (routeCase.legacyPath != "" && legacyStatus != routeCase.wantStatus) {
			t.Errorf("%s status versioned=%d legacy=%d want=%d", routeCase.name, versionedStatus, legacyStatus, routeCase.wantStatus)
		}
	}
}
