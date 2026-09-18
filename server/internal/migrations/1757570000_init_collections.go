package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(InitCollectionsUp, initCollectionsDown, "1757570000_init_collections.go")
}

// InitCollectionsUp は events / works / work_secrets / reactions の
// コレクション・APIルール・インデックスを作成する(SPEC §6, §7.2)。
// テストからも直接呼べるよう公開している。
func InitCollectionsUp(app core.App) error {
	// ---------------------------------------------------------------
	// events (§6.1)
	// ---------------------------------------------------------------
	events := core.NewBaseCollection("events")
	events.Fields.Add(
		&core.TextField{Name: "name", Required: true, Min: 1, Max: 80},
		&core.TextField{Name: "slug", Required: true, Pattern: `^[a-z0-9-]{2,40}$`},
		&core.TextField{Name: "passphrase", Required: true, Min: 8},
		&core.NumberField{Name: "max_video_bytes", Required: true, OnlyInt: true},
		&core.BoolField{Name: "submissions_open"},
		&core.TextField{Name: "description"},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	events.AddIndex("idx_events_slug", true, "`slug`", "")
	// 合言葉を知っている人だけ閲覧可。書き込みは superuser のみ(nil)。
	events.ListRule = types.Pointer("passphrase = @request.headers.x_event_key")
	events.ViewRule = types.Pointer("passphrase = @request.headers.x_event_key")
	if err := app.Save(events); err != nil {
		return err
	}

	// ---------------------------------------------------------------
	// works (§6.2)
	// ---------------------------------------------------------------
	works := core.NewBaseCollection("works")
	works.Fields.Add(
		&core.RelationField{Name: "event", CollectionId: events.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
		&core.TextField{Name: "description", Required: true, Min: 1, Max: 10000},
		&core.FileField{
			Name:      "images",
			MaxSelect: 10,
			MaxSize:   10 << 20,
			MimeTypes: []string{"image/jpeg", "image/png", "image/webp", "image/gif"},
			Thumbs:    []string{"600x400"},
		},
		&core.FileField{Name: "video", MaxSelect: 1, MaxSize: 2147483648},
		&core.SelectField{Name: "video_status", Values: []string{"none", "uploading", "processing", "ready", "failed"}, MaxSelect: 1},
		&core.TextField{Name: "video_error"},
		&core.FileField{Name: "thumbnail", MaxSelect: 1},
		&core.URLField{Name: "video_url"},
		&core.URLField{Name: "demo_url"},
		&core.URLField{Name: "github_url"},
		&core.JSONField{Name: "tags"},
		&core.JSONField{Name: "links"},
		&core.NumberField{Name: "like_count", OnlyInt: true},
		&core.TextField{Name: "active_upload_id", Hidden: true},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	works.AddIndex("idx_works_event_created", false, "`event`, `created`", "")
	works.AddIndex("idx_works_event_like_count", false, "`event`, `like_count`", "")
	works.ListRule = types.Pointer("event.passphrase = @request.headers.x_event_key")
	works.ViewRule = types.Pointer("event.passphrase = @request.headers.x_event_key")
	if err := app.Save(works); err != nil {
		return err
	}

	// ---------------------------------------------------------------
	// work_secrets (§6.3) — 全ルールロック(管理者専用)
	// 作品ごとの秘密(編集キー・作品コード)だけを持つ。作者を特定する項目は置かない。
	// ---------------------------------------------------------------
	secrets := core.NewBaseCollection("work_secrets")
	secrets.Fields.Add(
		&core.RelationField{Name: "work", CollectionId: works.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
		&core.TextField{Name: "edit_key", Required: true},
		&core.TextField{Name: "work_code", Required: true},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	secrets.AddIndex("idx_work_secrets_work", true, "`work`", "")
	secrets.AddIndex("idx_work_secrets_code", true, "`work_code`", "")
	if err := app.Save(secrets); err != nil {
		return err
	}

	// ---------------------------------------------------------------
	// reactions (§6.4) — 全ルールロック(管理者専用)
	// ---------------------------------------------------------------
	reactions := core.NewBaseCollection("reactions")
	reactions.Fields.Add(
		&core.RelationField{Name: "work", CollectionId: works.Id, Required: true, MaxSelect: 1, CascadeDelete: true},
		&core.TextField{Name: "device_id", Required: true},
		&core.AutodateField{Name: "created", OnCreate: true},
	)
	reactions.AddIndex("idx_reactions_work_device", true, "`work`, `device_id`", "")
	return app.Save(reactions)
}

func initCollectionsDown(app core.App) error {
	for _, name := range []string{"reactions", "work_secrets", "works", "events"} {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			continue
		}
		if err := app.Delete(col); err != nil {
			return err
		}
	}
	return nil
}
