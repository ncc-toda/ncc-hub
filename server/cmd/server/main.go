package main

import (
	"log"
	"net/http"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/hook"

	"github.com/ncc-toda/ncc-hub/server/internal/auth"
	"github.com/ncc-toda/ncc-hub/server/internal/likes"
	"github.com/ncc-toda/ncc-hub/server/internal/media"
	_ "github.com/ncc-toda/ncc-hub/server/internal/migrations"
	"github.com/ncc-toda/ncc-hub/server/internal/upload"
	"github.com/ncc-toda/ncc-hub/server/internal/works"
)

func main() {
	app := pocketbase.New()

	var publicDir string
	var automigrate bool
	app.RootCmd.PersistentFlags().StringVar(&publicDir, "publicDir", "./pb_public",
		"静的フロントエンド(pb_public)のディレクトリ")
	app.RootCmd.PersistentFlags().BoolVar(&automigrate, "automigrate", false,
		"コレクション変更時にマイグレーションファイルを自動生成する(開発用)")
	_ = app.RootCmd.ParseFlags(os.Args[1:])

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Dir:         "internal/migrations",
		Automigrate: automigrate,
	})

	// 標準 Records API のフック(合言葉ゲート・監査ログ)とカスタムルート・cron。
	auth.RegisterHooks(app)
	works.Register(app)
	upload.Register(app)
	media.Register(app)
	likes.Register(app)

	// 静的フロント配信(SPA fallback あり)。カスタムルートより後に評価されるよう優先度を下げる。
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Priority: 999,
		Func: func(se *core.ServeEvent) error {
			if _, err := os.Stat(publicDir); err == nil {
				if !se.Router.HasRoute(http.MethodGet, "/{path...}") {
					se.Router.GET("/{path...}", apis.Static(os.DirFS(publicDir), true)).
						Bind(apis.Gzip())
				}
			}
			return se.Next()
		},
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
