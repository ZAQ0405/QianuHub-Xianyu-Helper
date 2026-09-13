package server

import (
	"github.com/go-chi/chi/v5"

	"xianyu-go/internal/auth"
)

// mountVersionedItemBatchRoutes 挂载商品同步和类目推荐的 `/api/v1` 入口；批量铺货已移除。
func (s *Server) mountVersionedItemBatchRoutes(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(s.Auth.Middleware)
		r.Use(auth.RequireAuth)
		r.Post("/api/v1/items/get-all-from-account", s.syncItemsFromAccount)
		r.Post("/api/v1/items/get-by-page", s.syncItemsPageFromAccount)
		r.Post("/api/v1/items/publish-categories/recommend", s.recommendItemPublishCategory)
	})
}
