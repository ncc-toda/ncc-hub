// 静的フロント配信（SPA fallback と Cache-Control）。
package webstatic

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

const (
	htmlCache  = "no-store"
	assetCache = "public, max-age=31536000, immutable"
)

// CacheControl はパスに応じた Cache-Control 値を返す。
func CacheControl(path string) string {
	if strings.HasPrefix(path, "/assets/") {
		return assetCache
	}
	return htmlCache
}

// Middleware は HTML をキャッシュさせず、hashed asset だけ長期キャッシュする。
// Nix 成果物の mtime は 1970-01-01 なので、HTML では条件付きリクエストを捨てて 304 を防ぐ。
func Middleware() *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Func: func(e *core.RequestEvent) error {
			path := e.Request.URL.Path
			e.Response.Header().Set("Cache-Control", CacheControl(path))
			if !strings.HasPrefix(path, "/assets/") {
				e.Request.Header.Del("If-Modified-Since")
				e.Request.Header.Del("If-None-Match")
			}
			return e.Next()
		},
	}
}

// Mount は pb_public を配信する。/{path...} より具体的な /assets を先に登録する。
func Mount(se *core.ServeEvent, publicDir string) {
	if _, err := os.Stat(publicDir); err != nil {
		return
	}

	gzip := apis.Gzip()
	cache := Middleware()
	assetsDir := filepath.Join(publicDir, "assets")
	if _, err := os.Stat(assetsDir); err == nil && !se.Router.HasRoute(http.MethodGet, "/assets/{path...}") {
		se.Router.GET("/assets/{path...}", apis.Static(os.DirFS(assetsDir), false)).
			Bind(cache).
			Bind(gzip)
	}
	if !se.Router.HasRoute(http.MethodGet, "/{path...}") {
		se.Router.GET("/{path...}", apis.Static(os.DirFS(publicDir), true)).
			Bind(cache).
			Bind(gzip)
	}
}
