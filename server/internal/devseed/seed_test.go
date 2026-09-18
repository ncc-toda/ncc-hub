package devseed_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/ncc-toda/ncc-hub/server/internal/devseed"
	_ "github.com/ncc-toda/ncc-hub/server/internal/migrations"
)

func newApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	return app
}

func TestMaybeSeedDisabledIsNoop(t *testing.T) {
	app := newApp(t)
	if err := devseed.MaybeSeed(app, false, "127.0.0.1:8090"); err != nil {
		t.Fatal(err)
	}
	n, err := app.CountRecords("events", dbx.HashExp{"slug": devseed.EventSlug})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("disabled なのに events が %d 件", n)
	}
}

func TestMaybeSeedRejectsNonLoopback(t *testing.T) {
	app := newApp(t)
	if err := devseed.MaybeSeed(app, true, "0.0.0.0:8090"); err == nil {
		t.Fatal("非ループバックを許可してはいけない")
	}
	n, err := app.CountRecords("events", dbx.HashExp{"slug": devseed.EventSlug})
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("失敗したのに events が %d 件", n)
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	app := newApp(t)
	if err := devseed.MaybeSeed(app, true, "127.0.0.1:8090"); err != nil {
		t.Fatal(err)
	}
	if err := devseed.MaybeSeed(app, true, "127.0.0.1:8090"); err != nil {
		t.Fatal(err)
	}

	event, err := app.FindFirstRecordByFilter("events", "slug = {:slug}", dbx.Params{"slug": devseed.EventSlug})
	if err != nil {
		t.Fatal(err)
	}
	if event.GetString("passphrase") != devseed.Passphrase {
		t.Fatalf("passphrase = %q", event.GetString("passphrase"))
	}
	if !event.GetBool("submissions_open") {
		t.Fatal("submissions_open が false")
	}

	n, err := app.CountRecords("works", dbx.HashExp{"event": event.Id})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("works = %d, want 2", n)
	}

	works, err := app.FindRecordsByFilter("works", "event = {:id}", "like_count", 10, 0, dbx.Params{"id": event.Id})
	if err != nil {
		t.Fatal(err)
	}
	if works[0].GetInt("like_count") != 0 || works[1].GetInt("like_count") != 3 {
		t.Fatalf("like_count = %d, %d", works[0].GetInt("like_count"), works[1].GetInt("like_count"))
	}
	for _, w := range works {
		if w.GetString("video_url") == "" {
			t.Fatal("video_url が空")
		}
		sec, err := app.FindFirstRecordByFilter("work_secrets", "work = {:id}", dbx.Params{"id": w.Id})
		if err != nil {
			t.Fatal(err)
		}
		if sec.GetString("work_code") == "" || sec.GetString("edit_key") == "" {
			t.Fatal("work_secrets が空")
		}
	}
}

func TestSeedDoesNotOverwriteExistingSuperuser(t *testing.T) {
	app := newApp(t)
	before, err := app.CountRecords(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	if before == 0 {
		t.Fatal("テスト用 superuser が無い")
	}
	if err := devseed.Seed(app); err != nil {
		t.Fatal(err)
	}
	after, err := app.CountRecords(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("superuser が増えた: before=%d after=%d", before, after)
	}
	if _, err := app.FindAuthRecordByEmail(core.CollectionNameSuperusers, devseed.SuperuserEmail); err == nil {
		t.Fatal("既存 superuser があるのに dev@example.com を作ってはいけない")
	}
}

func TestSuperuserCredentialsAreValid(t *testing.T) {
	app := newApp(t)
	col, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(devseed.SuperuserEmail)
	r.SetPassword(devseed.SuperuserPassword)
	r.SetVerified(true)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
}
