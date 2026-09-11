package works_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/ncc-toda/ncc-hub/server/internal/auth"
	"github.com/ncc-toda/ncc-hub/server/internal/likes"
	_ "github.com/ncc-toda/ncc-hub/server/internal/migrations"
	"github.com/ncc-toda/ncc-hub/server/internal/upload"
	"github.com/ncc-toda/ncc-hub/server/internal/works"
)

const (
	passphrase = "himitsu-no-aikotoba"
	eventID    = "evt000000000001"
	workID     = "wrk000000000001"
	editKey    = "editkey0123456789abcdefghijklmno" // 32文字
	uploadID   = "up0000000000000000000001"         // 24文字
	deviceID   = "12345678-1234-4123-8123-123456789012"
)

// 1x1 の透過 PNG。
const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func pngBytes(t testing.TB) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(pngBase64)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// newApp は マイグレーション適用済みの TestApp を作り、全ルート・フックを登録する。
// マイグレーションは blank import された internal/migrations が core.AppMigrations に
// 登録するため、tests.NewTestApp 内の RunAllMigrations で自動適用される。
func newApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	auth.RegisterHooks(app)
	works.Register(app)
	upload.Register(app)
	likes.Register(app)
	return app
}

func seedEvent(t testing.TB, app core.App, open bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("id", eventID)
	r.Set("name", "テストイベント")
	r.Set("slug", "test2026")
	r.Set("passphrase", passphrase)
	r.Set("max_video_bytes", 2147483648)
	r.Set("submissions_open", open)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func seedWork(t testing.TB, app core.App) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("works")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("id", workID)
	r.Set("event", eventID)
	r.Set("title", "既存の作品")
	r.Set("video_status", "none")
	r.Set("like_count", 0)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}

	secCol, err := app.FindCollectionByNameOrId("work_secrets")
	if err != nil {
		t.Fatal(err)
	}
	s := core.NewRecord(secCol)
	s.Set("work", workID)
	s.Set("edit_key", editKey)
	s.Set("author_name", "山田太郎")
	if err := app.Save(s); err != nil {
		t.Fatal(err)
	}
	return r
}

func seedUpload(t testing.TB, app core.App, work *core.Record, meta upload.Meta) string {
	t.Helper()
	dir := filepath.Join(app.DataDir(), "uploads", uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	work.Set("active_upload_id", uploadID)
	work.Set("video_status", "uploading")
	if err := app.Save(work); err != nil {
		t.Fatal(err)
	}
	return dir
}

func seedReaction(t testing.TB, app core.App) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("reactions")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("work", workID)
	r.Set("device_id", deviceID)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
}

func multipartBody(t testing.TB, values map[string]string, fileField, fileName string, fileContent []byte) (io.Reader, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	for k, v := range values {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if fileField != "" {
		fw, err := w.CreateFormFile(fileField, fileName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(fileContent); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf, w.FormDataContentType()
}

// ---------------------------------------------------------------
// 標準 Records API の合言葉ゲート (SPEC §7.2, §15)
// ---------------------------------------------------------------

func TestWorksListRequiresEventKey(t *testing.T) {
	factory := func(t testing.TB) *tests.TestApp {
		app := newApp(t)
		seedEvent(t, app, true)
		seedWork(t, app)
		return app
	}

	(&tests.ApiScenario{
		Name:            "合言葉なしで works 一覧は 401",
		Method:          http.MethodGet,
		URL:             "/api/collections/works/records",
		ExpectedStatus:  401,
		ExpectedContent: []string{`"event_key_required"`},
		TestAppFactory:  factory,
	}).Test(t)

	(&tests.ApiScenario{
		Name:            "誤った合言葉で works 一覧は 401",
		Method:          http.MethodGet,
		URL:             "/api/collections/works/records",
		Headers:         map[string]string{"X-Event-Key": "wrong-key-123"},
		ExpectedStatus:  401,
		ExpectedContent: []string{`"event_key_invalid"`},
		TestAppFactory:  factory,
	}).Test(t)

	(&tests.ApiScenario{
		Name:            "正しい合言葉で works 一覧は 200",
		Method:          http.MethodGet,
		URL:             "/api/collections/works/records",
		Headers:         map[string]string{"X-Event-Key": passphrase},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"既存の作品"`, fmt.Sprintf("%q", workID)},
		NotExpectedContent: []string{
			"author_name", "edit_key", "active_upload_id", "山田",
		},
		TestAppFactory: factory,
	}).Test(t)
}

func TestWorkSecretsLockedEvenWithValidKey(t *testing.T) {
	factory := func(t testing.TB) *tests.TestApp {
		app := newApp(t)
		seedEvent(t, app, true)
		seedWork(t, app)
		return app
	}

	for _, col := range []string{"work_secrets", "reactions"} {
		(&tests.ApiScenario{
			Name:            col + " は正しい合言葉でも 403",
			Method:          http.MethodGet,
			URL:             "/api/collections/" + col + "/records",
			Headers:         map[string]string{"X-Event-Key": passphrase},
			ExpectedStatus:  403,
			ExpectedContent: []string{`"status":403`},
			TestAppFactory:  factory,
		}).Test(t)
	}
}

// ---------------------------------------------------------------
// POST /api/x/works (SPEC §8.2)
// ---------------------------------------------------------------

func TestCreateWork(t *testing.T) {
	factory := func(t testing.TB) *tests.TestApp {
		app := newApp(t)
		seedEvent(t, app, true)
		return app
	}

	body, contentType := multipartBody(t, map[string]string{
		"title":       "すごい作品",
		"description": "アピール文です",
		"tags":        `["くじ引き","Svelte"]`,
		"demo_url":    "https://example.com/demo",
		"author_name": "山田太郎",
	}, "images", "山田太郎_くじ.png", pngBytes(t))

	(&tests.ApiScenario{
		Name:   "作成成功で 201 + edit_key、作者情報は含まれない",
		Method: http.MethodPost,
		URL:    "/api/x/works",
		Body:   body,
		Headers: map[string]string{
			"X-Event-Key":  passphrase,
			"Content-Type": contentType,
		},
		ExpectedStatus:     201,
		ExpectedContent:    []string{`"edit_key":"`, `"title":"すごい作品"`, `"video_status":"none"`},
		NotExpectedContent: []string{"author_name", "山田太郎", "active_upload_id"},
		TestAppFactory:     factory,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			work, err := app.FindFirstRecordByFilter("works", "title = 'すごい作品'")
			if err != nil {
				t.Fatalf("作成された作品が見つかりません: %v", err)
			}
			images := work.GetStringSlice("images")
			if len(images) != 1 {
				t.Fatalf("images の数が不正: %v", images)
			}
			if !regexp.MustCompile(`^img_[a-zA-Z0-9]{16}\.png$`).MatchString(images[0]) {
				t.Fatalf("保存ファイル名が乱数化されていません: %s", images[0])
			}
			stored := filepath.Join(app.DataDir(), "storage", work.BaseFilesPath(), images[0])
			if _, err := os.Stat(stored); err != nil {
				t.Fatalf("保存ファイルが存在しません: %v", err)
			}
			secret, err := app.FindFirstRecordByFilter("work_secrets", "work = {:w}", dbx.Params{"w": work.Id})
			if err != nil {
				t.Fatalf("work_secrets が作成されていません: %v", err)
			}
			if secret.GetString("author_name") != "山田太郎" {
				t.Fatal("author_name が保存されていません")
			}
			if len(secret.GetString("edit_key")) != 32 {
				t.Fatal("edit_key が32文字ではありません")
			}
		},
	}).Test(t)

	invalidBody, invalidType := multipartBody(t, map[string]string{
		"title":       "",
		"author_name": "",
		"demo_url":    "ftp://example.com",
	}, "", "", nil)

	(&tests.ApiScenario{
		Name:   "バリデーション失敗は 422 + fields",
		Method: http.MethodPost,
		URL:    "/api/x/works",
		Body:   invalidBody,
		Headers: map[string]string{
			"X-Event-Key":  passphrase,
			"Content-Type": invalidType,
		},
		ExpectedStatus:  422,
		ExpectedContent: []string{`"code":"validation"`, `"title"`, `"author_name"`, `"demo_url"`},
		TestAppFactory:  factory,
	}).Test(t)

	closedFactory := func(t testing.TB) *tests.TestApp {
		app := newApp(t)
		seedEvent(t, app, false)
		return app
	}
	closedBody, closedType := multipartBody(t, map[string]string{
		"title":       "作品",
		"author_name": "山田",
	}, "", "", nil)

	(&tests.ApiScenario{
		Name:   "受付終了中の作成は 423",
		Method: http.MethodPost,
		URL:    "/api/x/works",
		Body:   closedBody,
		Headers: map[string]string{
			"X-Event-Key":  passphrase,
			"Content-Type": closedType,
		},
		ExpectedStatus:  423,
		ExpectedContent: []string{`"submissions_closed"`},
		TestAppFactory:  closedFactory,
	}).Test(t)
}

// ---------------------------------------------------------------
// PATCH /api/x/works/{id} (SPEC §8.3)
// ---------------------------------------------------------------

func TestPatchWork(t *testing.T) {
	factory := func(t testing.TB) *tests.TestApp {
		app := newApp(t)
		seedEvent(t, app, true)
		seedWork(t, app)
		return app
	}

	wrongKeyBody, wrongKeyType := multipartBody(t, map[string]string{"title": "改ざん"}, "", "", nil)
	(&tests.ApiScenario{
		Name:   "誤った編集キーで PATCH は 403",
		Method: http.MethodPatch,
		URL:    "/api/x/works/" + workID,
		Body:   wrongKeyBody,
		Headers: map[string]string{
			"X-Event-Key":  passphrase,
			"X-Edit-Key":   "wrong-edit-key",
			"Content-Type": wrongKeyType,
		},
		ExpectedStatus:  403,
		ExpectedContent: []string{`"edit_key_invalid"`},
		TestAppFactory:  factory,
	}).Test(t)

	okBody, okType := multipartBody(t, map[string]string{
		"title":       "新タイトル",
		"author_name": "田中花子",
	}, "", "", nil)
	(&tests.ApiScenario{
		Name:   "正しい編集キーで PATCH は 200",
		Method: http.MethodPatch,
		URL:    "/api/x/works/" + workID,
		Body:   okBody,
		Headers: map[string]string{
			"X-Event-Key":  passphrase,
			"X-Edit-Key":   editKey,
			"Content-Type": okType,
		},
		ExpectedStatus:     200,
		ExpectedContent:    []string{`"title":"新タイトル"`},
		NotExpectedContent: []string{"edit_key", "author_name", "田中花子"},
		TestAppFactory:     factory,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			secret, err := app.FindFirstRecordByFilter("work_secrets", "work = {:w}", dbx.Params{"w": workID})
			if err != nil {
				t.Fatal(err)
			}
			if secret.GetString("author_name") != "田中花子" {
				t.Fatal("work_secrets の author_name が更新されていません")
			}
		},
	}).Test(t)

	videoBody, videoType := multipartBody(t, map[string]string{}, "video", "movie.mp4", []byte("fake"))
	(&tests.ApiScenario{
		Name:   "video フィールドの直接送信は 400",
		Method: http.MethodPatch,
		URL:    "/api/x/works/" + workID,
		Body:   videoBody,
		Headers: map[string]string{
			"X-Event-Key":  passphrase,
			"X-Edit-Key":   editKey,
			"Content-Type": videoType,
		},
		ExpectedStatus:  400,
		ExpectedContent: []string{`"bad_request"`},
		TestAppFactory:  factory,
	}).Test(t)
}

// ---------------------------------------------------------------
// DELETE /api/x/works/{id} (SPEC §8.4)
// ---------------------------------------------------------------

func TestDeleteWork(t *testing.T) {
	(&tests.ApiScenario{
		Name:   "誤った編集キーで DELETE は 403",
		Method: http.MethodDelete,
		URL:    "/api/x/works/" + workID,
		Headers: map[string]string{
			"X-Event-Key": passphrase,
			"X-Edit-Key":  "wrong",
		},
		ExpectedStatus:  403,
		ExpectedContent: []string{`"edit_key_invalid"`},
		TestAppFactory: func(t testing.TB) *tests.TestApp {
			app := newApp(t)
			seedEvent(t, app, true)
			seedWork(t, app)
			return app
		},
	}).Test(t)

	var uploadDir string
	(&tests.ApiScenario{
		Name:   "正しい編集キーで DELETE は 204、連鎖削除される",
		Method: http.MethodDelete,
		URL:    "/api/x/works/" + workID,
		Headers: map[string]string{
			"X-Event-Key": passphrase,
			"X-Edit-Key":  editKey,
		},
		ExpectedStatus: 204,
		TestAppFactory: func(t testing.TB) *tests.TestApp {
			app := newApp(t)
			seedEvent(t, app, true)
			w := seedWork(t, app)
			seedReaction(t, app)
			uploadDir = seedUpload(t, app, w, upload.Meta{
				WorkID: workID, EventID: eventID, FilenameExt: ".mp4",
				Size: 5, ChunkSize: upload.ChunkSize, TotalChunks: 1,
				Created: time.Now().UTC(),
			})
			return app
		},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			if _, err := app.FindRecordById("works", workID); err == nil {
				t.Fatal("works が削除されていません")
			}
			if _, err := app.FindFirstRecordByFilter("work_secrets", "work = {:w}", dbx.Params{"w": workID}); err == nil {
				t.Fatal("work_secrets が連鎖削除されていません")
			}
			if n, _ := app.CountRecords("reactions", dbx.HashExp{"work": workID}); n != 0 {
				t.Fatal("reactions が連鎖削除されていません")
			}
			if _, err := os.Stat(uploadDir); !os.IsNotExist(err) {
				t.Fatal("進行中アップロードの一時ディレクトリが削除されていません")
			}
		},
	}).Test(t)
}

// ---------------------------------------------------------------
// 分割アップロード (SPEC §8.5)
// ---------------------------------------------------------------

func uploadFactory(open bool) func(t testing.TB) *tests.TestApp {
	return func(t testing.TB) *tests.TestApp {
		app := newApp(t)
		seedEvent(t, app, open)
		seedWork(t, app)
		return app
	}
}

func initReq(body string) (io.Reader, map[string]string) {
	return strings.NewReader(body), map[string]string{
		"X-Event-Key":  passphrase,
		"X-Edit-Key":   editKey,
		"Content-Type": "application/json",
	}
}

func TestUploadInitValidations(t *testing.T) {
	body, headers := initReq(`{"filename":"a.mov","size":734003200,"chunk_size":1048576}`)
	(&tests.ApiScenario{
		Name:            "chunk_size が既定値以外は 400",
		Method:          http.MethodPost,
		URL:             "/api/x/works/" + workID + "/video/init",
		Body:            body,
		Headers:         headers,
		ExpectedStatus:  400,
		ExpectedContent: []string{`"bad_request"`},
		TestAppFactory:  uploadFactory(true),
	}).Test(t)

	body, headers = initReq(`{"filename":"a.mov","size":3221225472,"chunk_size":20971520}`)
	(&tests.ApiScenario{
		Name:            "max_video_bytes 超過は 413",
		Method:          http.MethodPost,
		URL:             "/api/x/works/" + workID + "/video/init",
		Body:            body,
		Headers:         headers,
		ExpectedStatus:  413,
		ExpectedContent: []string{`"too_large"`},
		TestAppFactory:  uploadFactory(true),
	}).Test(t)

	body, headers = initReq(`{"filename":"a.txt","size":1000,"chunk_size":20971520}`)
	(&tests.ApiScenario{
		Name:            "対応していない拡張子は 422",
		Method:          http.MethodPost,
		URL:             "/api/x/works/" + workID + "/video/init",
		Body:            body,
		Headers:         headers,
		ExpectedStatus:  422,
		ExpectedContent: []string{`"validation"`},
		TestAppFactory:  uploadFactory(true),
	}).Test(t)

	body, headers = initReq(`{"filename":"a.mov","size":0,"chunk_size":20971520}`)
	(&tests.ApiScenario{
		Name:            "size 0 は 400",
		Method:          http.MethodPost,
		URL:             "/api/x/works/" + workID + "/video/init",
		Body:            body,
		Headers:         headers,
		ExpectedStatus:  400,
		ExpectedContent: []string{`"bad_request"`},
		TestAppFactory:  uploadFactory(true),
	}).Test(t)

	body, headers = initReq(`{"filename":"IMG_1234.MOV","size":41943041,"chunk_size":20971520}`)
	(&tests.ApiScenario{
		Name:            "正常な init は 200",
		Method:          http.MethodPost,
		URL:             "/api/x/works/" + workID + "/video/init",
		Body:            body,
		Headers:         headers,
		ExpectedStatus:  200,
		ExpectedContent: []string{`"upload_id":"`, `"total_chunks":3`, `"chunk_size":20971520`},
		TestAppFactory:  uploadFactory(true),
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			work, err := app.FindRecordById("works", workID)
			if err != nil {
				t.Fatal(err)
			}
			if work.GetString("video_status") != "uploading" {
				t.Fatal("video_status が uploading になっていません")
			}
			id := work.GetString("active_upload_id")
			if len(id) != 24 {
				t.Fatalf("active_upload_id が不正: %q", id)
			}
			if _, err := os.Stat(filepath.Join(app.DataDir(), "uploads", id, "meta.json")); err != nil {
				t.Fatal("meta.json が作成されていません")
			}
		},
	}).Test(t)

	body, headers = initReq(`{"filename":"a.mov","size":1000,"chunk_size":20971520}`)
	(&tests.ApiScenario{
		Name:            "受付終了中の init は 423",
		Method:          http.MethodPost,
		URL:             "/api/x/works/" + workID + "/video/init",
		Body:            body,
		Headers:         headers,
		ExpectedStatus:  423,
		ExpectedContent: []string{`"submissions_closed"`},
		TestAppFactory:  uploadFactory(false),
	}).Test(t)
}

func chunkedUploadFactory(seedChunk bool) func(t testing.TB) *tests.TestApp {
	return func(t testing.TB) *tests.TestApp {
		app := newApp(t)
		seedEvent(t, app, true)
		w := seedWork(t, app)
		dir := seedUpload(t, app, w, upload.Meta{
			WorkID: workID, EventID: eventID, FilenameExt: ".mp4",
			Size: 5, ChunkSize: upload.ChunkSize, TotalChunks: 1,
			Created: time.Now().UTC(),
		})
		if seedChunk {
			if err := os.WriteFile(filepath.Join(dir, "0000.part"), []byte("hello"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return app
	}
}

func TestUploadChunk(t *testing.T) {
	chunkHeaders := map[string]string{
		"X-Event-Key":  passphrase,
		"X-Edit-Key":   editKey,
		"Content-Type": "application/octet-stream",
	}

	(&tests.ApiScenario{
		Name:            "チャンクサイズ不一致は 400",
		Method:          http.MethodPut,
		URL:             "/api/x/works/" + workID + "/video/chunk/" + uploadID + "/0",
		Body:            strings.NewReader("too long body"),
		Headers:         chunkHeaders,
		ExpectedStatus:  400,
		ExpectedContent: []string{`"bad_request"`},
		TestAppFactory:  chunkedUploadFactory(false),
	}).Test(t)

	(&tests.ApiScenario{
		Name:            "チャンク番号が範囲外は 400",
		Method:          http.MethodPut,
		URL:             "/api/x/works/" + workID + "/video/chunk/" + uploadID + "/5",
		Body:            strings.NewReader("hello"),
		Headers:         chunkHeaders,
		ExpectedStatus:  400,
		ExpectedContent: []string{`"bad_request"`},
		TestAppFactory:  chunkedUploadFactory(false),
	}).Test(t)

	(&tests.ApiScenario{
		Name:            "正しいチャンクは 200",
		Method:          http.MethodPut,
		URL:             "/api/x/works/" + workID + "/video/chunk/" + uploadID + "/0",
		Body:            strings.NewReader("hello"),
		Headers:         chunkHeaders,
		ExpectedStatus:  200,
		ExpectedContent: []string{`"index":0`, `"received":1`},
		TestAppFactory:  chunkedUploadFactory(false),
	}).Test(t)

	(&tests.ApiScenario{
		Name:            "存在しない upload_id は 404",
		Method:          http.MethodPut,
		URL:             "/api/x/works/" + workID + "/video/chunk/xx0000000000000000000000/0",
		Body:            strings.NewReader("hello"),
		Headers:         chunkHeaders,
		ExpectedStatus:  404,
		ExpectedContent: []string{`"not_found"`},
		TestAppFactory:  chunkedUploadFactory(false),
	}).Test(t)
}

func TestUploadStatusCompleteAbort(t *testing.T) {
	authHeaders := map[string]string{
		"X-Event-Key": passphrase,
		"X-Edit-Key":  editKey,
	}

	// 状態取得は受付終了中でも 200(再開判断に使う)
	(&tests.ApiScenario{
		Name:            "状態取得",
		Method:          http.MethodGet,
		URL:             "/api/x/works/" + workID + "/video/chunk/" + uploadID,
		Headers:         authHeaders,
		ExpectedStatus:  200,
		ExpectedContent: []string{`"received_indexes":[0]`, `"total_chunks":1`, `"size":5`},
		TestAppFactory:  chunkedUploadFactory(true),
	}).Test(t)

	(&tests.ApiScenario{
		Name:            "チャンク不足の complete は 409 + missing",
		Method:          http.MethodPost,
		URL:             "/api/x/works/" + workID + "/video/complete/" + uploadID,
		Headers:         authHeaders,
		ExpectedStatus:  409,
		ExpectedContent: []string{`"upload_state_conflict"`, `"missing":[0]`},
		TestAppFactory:  chunkedUploadFactory(false),
	}).Test(t)

	(&tests.ApiScenario{
		Name:            "complete 成功で video_status=processing",
		Method:          http.MethodPost,
		URL:             "/api/x/works/" + workID + "/video/complete/" + uploadID,
		Headers:         authHeaders,
		ExpectedStatus:  200,
		ExpectedContent: []string{`"video_status":"processing"`},
		TestAppFactory:  chunkedUploadFactory(true),
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			work, err := app.FindRecordById("works", workID)
			if err != nil {
				t.Fatal(err)
			}
			video := work.GetString("video")
			if !regexp.MustCompile(`^video_[a-zA-Z0-9]{16}\.mp4$`).MatchString(video) {
				t.Fatalf("動画ファイル名が不正: %q", video)
			}
			if work.GetString("active_upload_id") != "" {
				t.Fatal("active_upload_id がクリアされていません")
			}
			if _, err := os.Stat(filepath.Join(app.DataDir(), "uploads", uploadID)); !os.IsNotExist(err) {
				t.Fatal("一時ディレクトリが削除されていません")
			}
		},
	}).Test(t)

	(&tests.ApiScenario{
		Name:           "中止は 204 で状態が戻る",
		Method:         http.MethodDelete,
		URL:            "/api/x/works/" + workID + "/video/chunk/" + uploadID,
		Headers:        authHeaders,
		ExpectedStatus: 204,
		TestAppFactory: chunkedUploadFactory(true),
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			work, err := app.FindRecordById("works", workID)
			if err != nil {
				t.Fatal(err)
			}
			if work.GetString("video_status") != "none" {
				t.Fatal("video_status が none に戻っていません")
			}
			if work.GetString("active_upload_id") != "" {
				t.Fatal("active_upload_id がクリアされていません")
			}
		},
	}).Test(t)
}

func TestCleanupUploads(t *testing.T) {
	app := newApp(t)
	defer app.Cleanup()
	seedEvent(t, app, true)
	w := seedWork(t, app)
	dir := seedUpload(t, app, w, upload.Meta{
		WorkID: workID, EventID: eventID, FilenameExt: ".mp4",
		Size: 5, ChunkSize: upload.ChunkSize, TotalChunks: 1,
		Created: time.Now().UTC().Add(-25 * time.Hour), // TTL(24h) 超過
	})

	upload.CleanupOnce(app)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("期限切れディレクトリが削除されていません")
	}
	work, err := app.FindRecordById("works", workID)
	if err != nil {
		t.Fatal(err)
	}
	if work.GetString("video_status") != "none" {
		t.Fatal("video_status が戻っていません")
	}
	if work.GetString("active_upload_id") != "" {
		t.Fatal("active_upload_id がクリアされていません")
	}
}

// ---------------------------------------------------------------
// いいね (SPEC §8.6)
// ---------------------------------------------------------------

func likeFactory(open, withReaction bool) func(t testing.TB) *tests.TestApp {
	return func(t testing.TB) *tests.TestApp {
		app := newApp(t)
		seedEvent(t, app, open)
		seedWork(t, app)
		if withReaction {
			seedReaction(t, app)
			w, err := app.FindRecordById("works", workID)
			if err != nil {
				t.Fatal(err)
			}
			w.Set("like_count", 1)
			if err := app.Save(w); err != nil {
				t.Fatal(err)
			}
		}
		return app
	}
}

func TestLikeToggle(t *testing.T) {
	(&tests.ApiScenario{
		Name:   "1回目のいいねで liked=true, like_count=1",
		Method: http.MethodPost,
		URL:    "/api/x/works/" + workID + "/like",
		Headers: map[string]string{
			"X-Event-Key": passphrase,
			"X-Device-Id": deviceID,
		},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"liked":true`, `"like_count":1`},
		TestAppFactory:  likeFactory(true, false),
	}).Test(t)

	(&tests.ApiScenario{
		Name:   "2回目のいいねで liked=false, like_count=0",
		Method: http.MethodPost,
		URL:    "/api/x/works/" + workID + "/like",
		Headers: map[string]string{
			"X-Event-Key": passphrase,
			"X-Device-Id": deviceID,
		},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"liked":false`, `"like_count":0`},
		TestAppFactory:  likeFactory(true, true),
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			work, err := app.FindRecordById("works", workID)
			if err != nil {
				t.Fatal(err)
			}
			if work.GetInt("like_count") != 0 {
				t.Fatal("like_count が 0 に戻っていません")
			}
		},
	}).Test(t)

	(&tests.ApiScenario{
		Name:   "X-Device-Id が UUID v4 形式でなければ 400",
		Method: http.MethodPost,
		URL:    "/api/x/works/" + workID + "/like",
		Headers: map[string]string{
			"X-Event-Key": passphrase,
			"X-Device-Id": "not-a-uuid",
		},
		ExpectedStatus:  400,
		ExpectedContent: []string{`"bad_request"`},
		TestAppFactory:  likeFactory(true, false),
	}).Test(t)

	(&tests.ApiScenario{
		Name:   "受付終了中のいいねは 423",
		Method: http.MethodPost,
		URL:    "/api/x/works/" + workID + "/like",
		Headers: map[string]string{
			"X-Event-Key": passphrase,
			"X-Device-Id": deviceID,
		},
		ExpectedStatus:  423,
		ExpectedContent: []string{`"submissions_closed"`},
		TestAppFactory:  likeFactory(false, false),
	}).Test(t)
}

func TestLikesList(t *testing.T) {
	(&tests.ApiScenario{
		Name:   "いいね済み作品IDの一覧",
		Method: http.MethodGet,
		URL:    "/api/x/likes?event=" + eventID,
		Headers: map[string]string{
			"X-Event-Key": passphrase,
			"X-Device-Id": deviceID,
		},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"work_ids":["` + workID + `"]`},
		TestAppFactory:  likeFactory(true, true),
	}).Test(t)

	(&tests.ApiScenario{
		Name:   "いいねが無ければ空配列",
		Method: http.MethodGet,
		URL:    "/api/x/likes",
		Headers: map[string]string{
			"X-Event-Key": passphrase,
			"X-Device-Id": deviceID,
		},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"work_ids":[]`},
		TestAppFactory:  likeFactory(true, false),
	}).Test(t)
}
