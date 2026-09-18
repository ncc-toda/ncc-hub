// Package works は作品の作成・更新・削除のカスタムルートを提供する(SPEC §8.2〜§8.4)。
package works

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gabriel-vasile/mimetype"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/pocketbase/pocketbase/tools/security"

	"github.com/ncc-toda/ncc-hub/server/internal/auth"
	"github.com/ncc-toda/ncc-hub/server/internal/media"
)

// Register はカスタムルートを登録する。
func Register(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.POST("/api/x/works", create)
		se.Router.PATCH("/api/x/works/{id}", update)
		se.Router.DELETE("/api/x/works/{id}", remove)
		return se.Next()
	})
}

// ---------------------------------------------------------------
// バリデーション (SPEC §6.2, §6.3, §8.1)
// ---------------------------------------------------------------

func validationError(fields map[string]any) *router.ApiError {
	return &router.ApiError{
		Status:  http.StatusUnprocessableEntity,
		Message: "入力内容を確認してください",
		Data:    map[string]any{"code": "validation", "fields": fields},
	}
}

// mapSaveError は app.Save のバリデーションエラーを SPEC §8.1 の 422 形式へ変換する。
func mapSaveError(err error) error {
	apiErr := router.NewApiError(http.StatusUnprocessableEntity, "", err)
	if len(apiErr.Data) > 0 {
		fields := make(map[string]any, len(apiErr.Data))
		for k, v := range apiErr.Data {
			fields[k] = v
		}
		return validationError(fields)
	}
	return auth.NewError(http.StatusBadRequest, "bad_request", "リクエストを処理できませんでした", nil)
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// checkDescription は説明・アピールを検証する。作成時は必須、更新時も空にはできない(SPEC §8.2, §8.3)。
func checkDescription(v string, fields map[string]any) {
	if n := runeLen(v); n < 1 || n > 10000 {
		fields["description"] = "説明、アピールは1〜10,000文字で入力してください"
	}
}

func checkURLField(name, v string, fields map[string]any) {
	if v != "" && !isHTTPURL(v) {
		fields[name] = "http:// または https:// のURLを入力してください"
	}
}

func parseTags(raw string, fields map[string]any) []string {
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		fields["tags"] = "タグの形式が不正です"
		return nil
	}
	if len(tags) > 10 {
		fields["tags"] = "タグは10個までです"
		return nil
	}
	for _, t := range tags {
		if t == "" || runeLen(t) > 20 {
			fields["tags"] = "タグは1〜20文字で入力してください"
			return nil
		}
	}
	return tags
}

type workLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func parseLinks(raw string, fields map[string]any) []workLink {
	var links []workLink
	if err := json.Unmarshal([]byte(raw), &links); err != nil {
		fields["links"] = "リンクの形式が不正です"
		return nil
	}
	if len(links) > 5 {
		fields["links"] = "リンクは5件までです"
		return nil
	}
	for i := range links {
		links[i].Title = strings.TrimSpace(links[i].Title)
		links[i].URL = strings.TrimSpace(links[i].URL)
		if links[i].Title == "" || links[i].URL == "" {
			fields["links"] = "リンクはタイトルとURLの両方を入力してください"
			return nil
		}
		if n := runeLen(links[i].Title); n > 30 {
			fields["links"] = "リンクのタイトルは1〜30文字で入力してください"
			return nil
		}
		if !isHTTPURL(links[i].URL) {
			fields["links"] = "リンクは http:// または https:// のURLを入力してください"
			return nil
		}
	}
	return links
}

// ---------------------------------------------------------------
// 作品コード (SPEC §8.2)
// ---------------------------------------------------------------

// workCodeAlphabet は作品コードに使う文字。紙のアンケートへ書き写す前提なので
// 紛らわしい 0/O・1/I/L と、語感の事故を避けるため U を除いてある。
const workCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTVWXYZ"

// newWorkCode は XXXX-XXXX 形式の作品コードを1つ作る。
func newWorkCode() string {
	s := security.RandomStringWithAlphabet(8, workCodeAlphabet)
	return s[:4] + "-" + s[4:]
}

// generateWorkCode は未使用の作品コードを返す。衝突したら作り直す(最大5回)。
// work_secrets.work_code の unique index が最終的な保証で、ここは事前の絞り込み。
func generateWorkCode(app core.App) (string, error) {
	for range 5 {
		code := newWorkCode()
		n, err := app.CountRecords("work_secrets", dbx.HashExp{"work_code": code})
		if err != nil {
			return "", err
		}
		if n == 0 {
			return code, nil
		}
	}
	return "", errors.New("work_code: 未使用の作品コードを生成できませんでした")
}

// checkContent は「中身のある投稿」であることを検証する(SPEC §8.2 の中身条件)。
// 作成時点では動画ファイルがまだ無いので、続けてアップロードする意思表示として
// video_pending を受け付ける。自己申告なので偽れるが、作れるのは空の投稿だけ。
func checkContent(ok bool, fields map[string]any) {
	if !ok {
		fields["content"] = "画像か動画のどちらかを必ず入れてください"
	}
}

// ---------------------------------------------------------------
// 画像処理 (SPEC §8.2 手順3, §11)
// ---------------------------------------------------------------

var extByMime = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// findImages は multipart の "images" (または "images[]") のファイルを取り出す。
func findImages(e *core.RequestEvent) ([]*filesystem.File, error) {
	files, err := e.FindUploadedFiles("images")
	if err == http.ErrMissingFile {
		files, err = e.FindUploadedFiles("images[]")
	}
	if err == http.ErrMissingFile {
		return nil, nil
	}
	return files, err
}

// randomizeImageNames は MIME 判定に基づき img_<16文字乱数>.<ext> へ付け替える。
// 元のファイル名・拡張子は一切信用せず破棄する(SPEC §11)。
func randomizeImageNames(files []*filesystem.File) error {
	for _, f := range files {
		r, err := f.Reader.Open()
		if err != nil {
			return err
		}
		mt, err := mimetype.DetectReader(r)
		_ = r.Close()
		if err != nil {
			return err
		}
		ext, ok := extByMime[mt.String()]
		if !ok {
			return fmt.Errorf("unsupported image type: %s", mt.String())
		}
		f.Name = "img_" + security.RandomString(16) + ext
		f.OriginalName = f.Name // ストレージのメタデータにも元名を残さない
	}
	return nil
}

func parseMultipart(e *core.RequestEvent) error {
	if err := e.Request.ParseMultipartForm(router.DefaultMaxMemory); err != nil {
		return auth.NewError(http.StatusBadRequest, "bad_request", "multipart/form-data で送信してください", nil)
	}
	return nil
}

// ---------------------------------------------------------------
// POST /api/x/works (SPEC §8.2)
// ---------------------------------------------------------------

func create(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	if err := auth.RequireOpen(ev); err != nil {
		return err
	}
	if err := parseMultipart(e); err != nil {
		return err
	}
	form := e.Request.MultipartForm

	value := func(k string) string {
		if vs := form.Value[k]; len(vs) > 0 {
			return vs[0]
		}
		return ""
	}

	fields := map[string]any{}

	description := value("description")
	checkDescription(description, fields)
	videoURL := value("video_url")
	checkURLField("video_url", videoURL, fields)
	demoURL := value("demo_url")
	checkURLField("demo_url", demoURL, fields)
	githubURL := value("github_url")
	checkURLField("github_url", githubURL, fields)

	var tags []string
	if raw := value("tags"); raw != "" {
		tags = parseTags(raw, fields)
	}
	var links []workLink
	if raw := value("links"); raw != "" {
		links = parseLinks(raw, fields)
	}

	images, err := findImages(e)
	if err != nil {
		return auth.NewError(http.StatusBadRequest, "bad_request", "画像を読み込めませんでした", nil)
	}
	if len(images) > 10 {
		fields["images"] = "画像は10枚までです"
	} else if err := randomizeImageNames(images); err != nil {
		fields["images"] = "対応していない画像形式です(JPEG/PNG/WebP/GIF のみ)"
	}

	checkContent(len(images) > 0 || videoURL != "" || value("video_pending") == "1", fields)

	if len(fields) > 0 {
		return validationError(fields)
	}

	worksCol, err := e.App.FindCollectionByNameOrId("works")
	if err != nil {
		return err
	}
	secretsCol, err := e.App.FindCollectionByNameOrId("work_secrets")
	if err != nil {
		return err
	}

	record := core.NewRecord(worksCol)
	editKey := security.RandomString(32)
	workCode, err := generateWorkCode(e.App)
	if err != nil {
		return err
	}

	// 作品と work_secrets は1トランザクションで作成する(SPEC §8.2 手順6)。
	txErr := e.App.RunInTransaction(func(tx core.App) error {
		record.Set("event", ev.Id)
		record.Set("description", description)
		record.Set("video_url", videoURL)
		record.Set("demo_url", demoURL)
		record.Set("github_url", githubURL)
		if tags != nil {
			record.Set("tags", tags)
		}
		if links != nil {
			record.Set("links", links)
		}
		record.Set("video_status", "none")
		record.Set("like_count", 0)
		if len(images) > 0 {
			record.Set("images", images)
		}
		if err := tx.Save(record); err != nil {
			return err
		}

		secret := core.NewRecord(secretsCol)
		secret.Set("work", record.Id)
		secret.Set("edit_key", editKey)
		secret.Set("work_code", workCode)
		return tx.Save(secret)
	})
	if txErr != nil {
		return mapSaveError(txErr)
	}

	// 保存後に exiftool で EXIF を除去(失敗は警告ログのみ。SPEC §10.4)。
	media.StripImageMetadata(e.App, record)

	// 編集キーと作品コードを返すのはこのレスポンスだけ(SPEC §8.2 手順7)。
	return e.JSON(http.StatusCreated, map[string]any{
		"work":      record,
		"edit_key":  editKey,
		"work_code": workCode,
	})
}

// ---------------------------------------------------------------
// PATCH /api/x/works/{id} (SPEC §8.3)
// ---------------------------------------------------------------

func update(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	if err := auth.RequireOpen(ev); err != nil {
		return err
	}
	record, _, err := auth.RequireEditableWork(e, ev, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	if err := parseMultipart(e); err != nil {
		return err
	}
	form := e.Request.MultipartForm

	// video フィールド本体はこのルートでは受け付けない(SPEC §8.3)。
	if len(form.File["video"]) > 0 || len(form.Value["video"]) > 0 {
		return auth.NewError(http.StatusBadRequest, "bad_request",
			"動画は分割アップロード(/video/init)をお使いください", nil)
	}

	present := func(k string) (string, bool) {
		vs, ok := form.Value[k]
		if !ok || len(vs) == 0 {
			return "", false
		}
		return vs[0], true
	}

	fields := map[string]any{}

	description, hasDescription := present("description")
	if hasDescription {
		checkDescription(description, fields)
	}
	videoURL, hasVideoURL := present("video_url")
	if hasVideoURL {
		checkURLField("video_url", videoURL, fields)
	}
	demoURL, hasDemoURL := present("demo_url")
	if hasDemoURL {
		checkURLField("demo_url", demoURL, fields)
	}
	githubURL, hasGithubURL := present("github_url")
	if hasGithubURL {
		checkURLField("github_url", githubURL, fields)
	}
	var tags []string
	rawTags, hasTags := present("tags")
	if hasTags && rawTags != "" {
		tags = parseTags(rawTags, fields)
	}
	var links []workLink
	rawLinks, hasLinks := present("links")
	if hasLinks && rawLinks != "" {
		links = parseLinks(rawLinks, fields)
	}

	// 画像の削除指定
	existing := record.GetStringSlice("images")
	var removeNames []string
	if raw, ok := present("images_remove"); ok && raw != "" {
		var reqNames []string
		if err := json.Unmarshal([]byte(raw), &reqNames); err != nil {
			fields["images_remove"] = "削除する画像の指定が不正です"
		} else {
			for _, n := range reqNames {
				for _, ex := range existing {
					if n == ex {
						removeNames = append(removeNames, n)
						break
					}
				}
			}
		}
	}

	// 画像の追加
	newImages, err := findImages(e)
	if err != nil {
		return auth.NewError(http.StatusBadRequest, "bad_request", "画像を読み込めませんでした", nil)
	}
	if len(newImages) > 0 {
		if err := randomizeImageNames(newImages); err != nil {
			fields["images"] = "対応していない画像形式です(JPEG/PNG/WebP/GIF のみ)"
		}
	}
	imageCount := len(existing) - len(removeNames) + len(newImages)
	if imageCount > 10 {
		fields["images"] = "画像は合計10枚までです"
	}

	// 更新後の状態で中身条件を判定する(SPEC §8.3)。
	// 作成時と違い実際のレコードを見られるので、ここは厳密に検証できる。
	videoRemove := false
	if v, ok := present("video_remove"); ok && v == "1" {
		videoRemove = true
	}
	status := record.GetString("video_status")
	hasVideo := !videoRemove && (status == "ready" || status == "uploading" || status == "processing")
	finalVideoURL := record.GetString("video_url")
	if hasVideoURL {
		finalVideoURL = videoURL
	}
	videoPending := false
	if v, ok := present("video_pending"); ok && v == "1" {
		videoPending = true
	}
	checkContent(imageCount > 0 || hasVideo || finalVideoURL != "" || videoPending, fields)

	if len(fields) > 0 {
		return validationError(fields)
	}

	if hasDescription {
		record.Set("description", description)
	}
	if hasVideoURL {
		record.Set("video_url", videoURL)
	}
	if hasDemoURL {
		record.Set("demo_url", demoURL)
	}
	if hasGithubURL {
		record.Set("github_url", githubURL)
	}
	if hasTags {
		record.Set("tags", tags)
	}
	if hasLinks {
		record.Set("links", links)
	}
	if len(removeNames) > 0 {
		record.Set("images-", removeNames)
	}
	if len(newImages) > 0 {
		record.Set("images+", newImages)
	}
	if videoRemove {
		record.Set("video", "")
		record.Set("thumbnail", "")
		record.Set("video_status", "none")
		record.Set("video_error", "")
	}

	// 作品コード・編集キーは更新できないので work_secrets には触れない(SPEC §8.3)。
	if err := e.App.Save(record); err != nil {
		return mapSaveError(err)
	}

	media.StripImageMetadata(e.App, record)

	return e.JSON(http.StatusOK, map[string]any{"work": record})
}

// ---------------------------------------------------------------
// DELETE /api/x/works/{id} (SPEC §8.4)
// ---------------------------------------------------------------

func remove(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	if err := auth.RequireOpen(ev); err != nil {
		return err
	}
	record, _, err := auth.RequireEditableWork(e, ev, e.Request.PathValue("id"))
	if err != nil {
		return err
	}

	// 進行中の分割アップロードの一時ディレクトリを削除(SPEC §8.4)。
	if uploadID := record.GetString("active_upload_id"); uploadID != "" {
		_ = os.RemoveAll(filepath.Join(e.App.DataDir(), "uploads", uploadID))
	}

	// works の削除で secrets・reactions・ファイルも連鎖削除される(SPEC §6.5)。
	// 監査ログは auth.RegisterHooks の OnRecordAfterDeleteSuccess で記録する。
	if err := e.App.Delete(record); err != nil {
		return auth.NewError(http.StatusBadRequest, "bad_request", "削除できませんでした", nil)
	}
	return e.NoContent(http.StatusNoContent)
}
