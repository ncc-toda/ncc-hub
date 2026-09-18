// Package devseed は開発用フレーバーのデータを投入する(SPEC §5.5)。
package devseed

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/security"
)

const (
	// EventSlug は開発用イベントの slug。フロントの DEV_EVENT_SLUG と同じ。
	EventSlug = "dev"
	// Passphrase は開発用イベントの合言葉。フロントの DEV_PASSPHRASE と同じ。
	Passphrase = "dev-aikotoba"
	// SuperuserEmail は開発用管理者。0 件のときだけ作成する。
	SuperuserEmail = "dev@example.com"
	// SuperuserPassword は 20 文字以上(SPEC §5.5)。
	SuperuserPassword = "ncc-hub-dev-admin-pass"

	eventName        = "開発用イベント"
	maxVideoBytes    = 2147483648
	workCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"
)

var sampleWorks = []sampleWork{
	{
		description: "開発用のサンプル作品です。新着順の確認に使います。",
		videoURL:    "https://example.com/dev-work-new",
		likeCount:   0,
	},
	{
		description: "いいね順の確認用サンプルです。カードと詳細の表示を見られます。",
		videoURL:    "https://example.com/dev-work-likes",
		likeCount:   3,
	},
}

type sampleWork struct {
	description string
	videoURL    string
	likeCount   int
}

// Register は --dev かつループバックのときだけシードする。
func Register(app core.App) {
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Priority: -100,
		Func: func(se *core.ServeEvent) error {
			if err := MaybeSeed(se.App, se.App.IsDev(), se.Server.Addr); err != nil {
				return err
			}
			return se.Next()
		},
	})
}

// MaybeSeed は enabled のときだけ投入する。ループバック以外はエラー。
func MaybeSeed(app core.App, enabled bool, httpAddr string) error {
	if !enabled {
		return nil
	}
	if err := RequireLoopback(httpAddr); err != nil {
		return err
	}
	return Seed(app)
}

// Seed は不足分だけ投入する。既存レコードは上書きしない。
func Seed(app core.App) error {
	if err := ensureSuperuser(app); err != nil {
		return err
	}
	event, err := ensureEvent(app)
	if err != nil {
		return err
	}
	if err := ensureWorks(app, event); err != nil {
		return err
	}
	logSeed(app, event)
	return nil
}

func ensureSuperuser(app core.App) error {
	n, err := app.CountRecords(core.CollectionNameSuperusers, dbx.Not(dbx.HashExp{
		"email": core.DefaultInstallerEmail,
	}))
	if err != nil {
		return fmt.Errorf("devseed superuser の件数: %w", err)
	}
	if n > 0 {
		return nil
	}

	col, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return fmt.Errorf("devseed superuser コレクション: %w", err)
	}
	r := core.NewRecord(col)
	r.SetEmail(SuperuserEmail)
	r.SetPassword(SuperuserPassword)
	r.SetVerified(true)
	if err := app.Save(r); err != nil {
		return fmt.Errorf("devseed superuser の作成: %w", err)
	}
	return nil
}

func ensureEvent(app core.App) (*core.Record, error) {
	event, err := app.FindFirstRecordByFilter("events", "slug = {:slug}", dbx.Params{"slug": EventSlug})
	if err == nil {
		return event, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("devseed イベントの検索: %w", err)
	}

	col, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		return nil, fmt.Errorf("devseed events コレクション: %w", err)
	}
	r := core.NewRecord(col)
	r.Set("name", eventName)
	r.Set("slug", EventSlug)
	r.Set("passphrase", Passphrase)
	r.Set("max_video_bytes", maxVideoBytes)
	r.Set("submissions_open", true)
	r.Set("description", "just dev が投入する開発用イベントです。")
	if err := app.Save(r); err != nil {
		return nil, fmt.Errorf("devseed イベントの作成: %w", err)
	}
	return r, nil
}

func ensureWorks(app core.App, event *core.Record) error {
	n, err := app.CountRecords("works", dbx.HashExp{"event": event.Id})
	if err != nil {
		return fmt.Errorf("devseed 作品の件数: %w", err)
	}
	if n > 0 {
		return nil
	}

	worksCol, err := app.FindCollectionByNameOrId("works")
	if err != nil {
		return fmt.Errorf("devseed works コレクション: %w", err)
	}
	secretsCol, err := app.FindCollectionByNameOrId("work_secrets")
	if err != nil {
		return fmt.Errorf("devseed work_secrets コレクション: %w", err)
	}

	for _, sample := range sampleWorks {
		if err := createSampleWork(app, event, worksCol, secretsCol, sample); err != nil {
			return err
		}
	}
	return nil
}

func createSampleWork(
	app core.App,
	event *core.Record,
	worksCol, secretsCol *core.Collection,
	sample sampleWork,
) error {
	code, err := unusedWorkCode(app)
	if err != nil {
		return err
	}
	editKey := security.RandomString(32)

	work := core.NewRecord(worksCol)
	work.Set("event", event.Id)
	work.Set("description", sample.description)
	work.Set("video_url", sample.videoURL)
	work.Set("video_status", "none")
	work.Set("like_count", sample.likeCount)

	return app.RunInTransaction(func(tx core.App) error {
		if err := tx.Save(work); err != nil {
			return fmt.Errorf("devseed 作品の作成: %w", err)
		}
		sec := core.NewRecord(secretsCol)
		sec.Set("work", work.Id)
		sec.Set("edit_key", editKey)
		sec.Set("work_code", code)
		if err := tx.Save(sec); err != nil {
			return fmt.Errorf("devseed work_secrets の作成: %w", err)
		}
		return nil
	})
}

func unusedWorkCode(app core.App) (string, error) {
	for range 5 {
		s := security.RandomStringWithAlphabet(8, workCodeAlphabet)
		code := s[:4] + "-" + s[4:]
		n, err := app.CountRecords("work_secrets", dbx.HashExp{"work_code": code})
		if err != nil {
			return "", fmt.Errorf("devseed work_code の確認: %w", err)
		}
		if n == 0 {
			return code, nil
		}
	}
	return "", errors.New("devseed: 未使用の作品コードを生成できませんでした")
}

func logSeed(app core.App, event *core.Record) {
	app.Logger().Info("devseed: 開発用イベント",
		"slug", EventSlug,
		"passphrase", Passphrase,
		"admin", SuperuserEmail,
	)

	works, err := app.FindRecordsByFilter("works", "event = {:id}", "created", 20, 0, dbx.Params{"id": event.Id})
	if err != nil {
		app.Logger().Warn("devseed: 作品一覧を取得できませんでした", "error", err)
		return
	}
	for _, w := range works {
		sec, err := app.FindFirstRecordByFilter("work_secrets", "work = {:id}", dbx.Params{"id": w.Id})
		if err != nil {
			continue
		}
		app.Logger().Info("devseed: サンプル作品",
			"work_id", w.Id,
			"work_code", sec.GetString("work_code"),
			"edit_key", sec.GetString("edit_key"),
		)
	}
}
