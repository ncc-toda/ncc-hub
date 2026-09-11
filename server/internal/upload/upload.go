// Package upload は動画の分割アップロード(SPEC §8.5)を提供する。
package upload

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/security"

	"github.com/ncc-toda/ncc-hub/server/internal/auth"
)

// ChunkSize は固定チャンクサイズ 20MB(SPEC §8.5)。
const ChunkSize int64 = 20971520

var allowedExts = []string{".mp4", ".mov", ".m4v", ".webm", ".mkv", ".avi"}

// Meta は pb_data/uploads/<upload_id>/meta.json の内容(SPEC §8.5.1)。
type Meta struct {
	WorkID      string    `json:"work_id"`
	EventID     string    `json:"event_id"`
	FilenameExt string    `json:"filename_ext"`
	Size        int64     `json:"size"`
	ChunkSize   int64     `json:"chunk_size"`
	TotalChunks int       `json:"total_chunks"`
	Created     time.Time `json:"created"`
}

// diskFree はテストで差し替え可能にしている。
var diskFree = func(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// UploadsDir は分割アップロードの一時領域のルート。
func UploadsDir(app core.App) string {
	return filepath.Join(app.DataDir(), "uploads")
}

func dirFor(app core.App, uploadID string) string {
	return filepath.Join(UploadsDir(app), uploadID)
}

func metaPath(dir string) string { return filepath.Join(dir, "meta.json") }

func loadMeta(dir string) (*Meta, error) {
	b, err := os.ReadFile(metaPath(dir))
	if err != nil {
		return nil, err
	}
	m := &Meta{}
	if err := json.Unmarshal(b, m); err != nil {
		return nil, err
	}
	return m, nil
}

func chunkName(index int) string { return fmt.Sprintf("%04d.part", index) }

func expectedChunkSize(m *Meta, index int) int64 {
	if index == m.TotalChunks-1 {
		return m.Size - m.ChunkSize*int64(m.TotalChunks-1)
	}
	return m.ChunkSize
}

func receivedIndexes(dir string, m *Meta) []int {
	indexes := []int{}
	for i := 0; i < m.TotalChunks; i++ {
		if info, err := os.Stat(filepath.Join(dir, chunkName(i))); err == nil && info.Size() == expectedChunkSize(m, i) {
			indexes = append(indexes, i)
		}
	}
	sort.Ints(indexes)
	return indexes
}

// Register はルートと掃除 cron を登録する。
func Register(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.POST("/api/x/works/{id}/video/init", initUpload)
		// チャンク本体は 20MB + 余裕のボディ上限を明示する(SPEC §8.5.2)。
		se.Router.PUT("/api/x/works/{id}/video/chunk/{upload_id}/{index}", putChunk).
			Bind(apis.BodyLimit(22 << 20))
		se.Router.GET("/api/x/works/{id}/video/chunk/{upload_id}", status)
		se.Router.POST("/api/x/works/{id}/video/complete/{upload_id}", complete)
		se.Router.DELETE("/api/x/works/{id}/video/chunk/{upload_id}", abort)
		return se.Next()
	})

	// 15分ごとに古い一時ディレクトリを掃除する(SPEC §8.5.6)。
	app.Cron().MustAdd("cleanup_uploads", "*/15 * * * *", func() {
		CleanupOnce(app)
	})
}

// resolveUpload は upload_id が作品の進行中アップロードと一致することを検証する。
func resolveUpload(e *core.RequestEvent, work *core.Record) (*Meta, string, error) {
	notFound := auth.NewError(http.StatusNotFound, "not_found", "アップロードが見つかりません", nil)

	uploadID := e.Request.PathValue("upload_id")
	if uploadID == "" || uploadID != work.GetString("active_upload_id") {
		return nil, "", notFound
	}
	dir := dirFor(e.App, uploadID)
	meta, err := loadMeta(dir)
	if err != nil || meta.WorkID != work.Id {
		return nil, "", notFound
	}
	return meta, dir, nil
}

// ---------------------------------------------------------------
// POST /api/x/works/{id}/video/init (SPEC §8.5.1)
// ---------------------------------------------------------------

func initUpload(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	if err := auth.RequireOpen(ev); err != nil {
		return err
	}
	work, _, err := auth.RequireEditableWork(e, ev, e.Request.PathValue("id"))
	if err != nil {
		return err
	}

	req := struct {
		Filename  string `json:"filename"`
		Size      int64  `json:"size"`
		ChunkSize int64  `json:"chunk_size"`
	}{}
	if err := e.BindBody(&req); err != nil {
		return auth.NewError(http.StatusBadRequest, "bad_request", "リクエストの形式が不正です", nil)
	}

	if req.ChunkSize != ChunkSize {
		return auth.NewError(http.StatusBadRequest, "bad_request",
			fmt.Sprintf("chunk_size は %d 固定です", ChunkSize), nil)
	}
	if req.Size <= 0 {
		return auth.NewError(http.StatusBadRequest, "bad_request", "サイズが不正です", nil)
	}

	maxBytes := int64(ev.GetFloat("max_video_bytes"))
	if maxBytes <= 0 {
		maxBytes = 2147483648
	}
	if req.Size > maxBytes {
		return auth.NewError(http.StatusRequestEntityTooLarge, "too_large",
			fmt.Sprintf("動画は %dMB 以内にしてください", maxBytes>>20), nil)
	}

	ext := strings.ToLower(filepath.Ext(req.Filename))
	allowed := false
	for _, a := range allowedExts {
		if ext == a {
			allowed = true
			break
		}
	}
	if !allowed {
		return auth.NewError(http.StatusUnprocessableEntity, "validation",
			"対応していない動画形式です(mp4 / mov / m4v / webm / mkv / avi)",
			map[string]any{"fields": map[string]any{"filename": "対応していない動画形式です"}})
	}

	// 空きディスク検査: size * 2.5 + 1GB (SPEC §8.5.1 手順4)
	if err := os.MkdirAll(UploadsDir(e.App), 0o755); err != nil {
		return err
	}
	free, err := diskFree(e.App.DataDir())
	if err == nil {
		required := int64(float64(req.Size)*2.5) + (1 << 30)
		if free < required {
			return auth.NewError(http.StatusInsufficientStorage, "insufficient_storage",
				"サーバーの空き容量が不足しています。先生に連絡してください", nil)
		}
	}

	// 既存の進行中アップロードはやり直し扱いで破棄(SPEC §8.5.1 手順5)。
	if old := work.GetString("active_upload_id"); old != "" {
		_ = os.RemoveAll(dirFor(e.App, old))
	}

	uploadID := security.RandomString(24)
	dir := dirFor(e.App, uploadID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	totalChunks := int((req.Size + ChunkSize - 1) / ChunkSize)
	meta := &Meta{
		WorkID:      work.Id,
		EventID:     ev.Id,
		FilenameExt: ext,
		Size:        req.Size,
		ChunkSize:   ChunkSize,
		TotalChunks: totalChunks,
		Created:     time.Now().UTC(),
	}
	b, _ := json.Marshal(meta)
	if err := os.WriteFile(metaPath(dir), b, 0o644); err != nil {
		return err
	}

	work.Set("active_upload_id", uploadID)
	work.Set("video_status", "uploading")
	if err := e.App.Save(work); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}

	return e.JSON(http.StatusOK, map[string]any{
		"upload_id":    uploadID,
		"chunk_size":   ChunkSize,
		"total_chunks": totalChunks,
	})
}

// ---------------------------------------------------------------
// PUT /api/x/works/{id}/video/chunk/{upload_id}/{index} (SPEC §8.5.2)
// ---------------------------------------------------------------

func putChunk(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	if err := auth.RequireOpen(ev); err != nil {
		return err
	}
	work, _, err := auth.RequireEditableWork(e, ev, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	meta, dir, err := resolveUpload(e, work)
	if err != nil {
		return err
	}

	index, err := strconv.Atoi(e.Request.PathValue("index"))
	if err != nil || index < 0 || index >= meta.TotalChunks {
		return auth.NewError(http.StatusBadRequest, "bad_request", "チャンク番号が不正です", nil)
	}
	expected := expectedChunkSize(meta, index)

	tmpPath := filepath.Join(dir, chunkName(index)+".tmp")
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(f, io.LimitReader(e.Request.Body, expected+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		return auth.NewError(http.StatusBadRequest, "bad_request", "チャンクを受信できませんでした", nil)
	}
	if written != expected {
		_ = os.Remove(tmpPath)
		return auth.NewError(http.StatusBadRequest, "bad_request", "チャンクのサイズが一致しません", nil)
	}
	// rename で原子的に確定する。同じ index の再送は上書き(冪等)。
	if err := os.Rename(tmpPath, filepath.Join(dir, chunkName(index))); err != nil {
		return err
	}

	return e.JSON(http.StatusOK, map[string]any{
		"index":    index,
		"received": len(receivedIndexes(dir, meta)),
	})
}

// ---------------------------------------------------------------
// GET /api/x/works/{id}/video/chunk/{upload_id} (SPEC §8.5.3)
// ---------------------------------------------------------------

func status(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	// 状態取得は受付終了後も再開判断に使えるよう requireOpen は課さない。
	work, _, err := auth.RequireEditableWork(e, ev, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	meta, dir, err := resolveUpload(e, work)
	if err != nil {
		return err
	}
	return e.JSON(http.StatusOK, map[string]any{
		"received_indexes": receivedIndexes(dir, meta),
		"total_chunks":     meta.TotalChunks,
		"size":             meta.Size,
	})
}

// ---------------------------------------------------------------
// POST /api/x/works/{id}/video/complete/{upload_id} (SPEC §8.5.4)
// ---------------------------------------------------------------

func complete(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	if err := auth.RequireOpen(ev); err != nil {
		return err
	}
	work, _, err := auth.RequireEditableWork(e, ev, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	meta, dir, err := resolveUpload(e, work)
	if err != nil {
		return err
	}

	// 全チャンクの存在とサイズを確認(不足は 409 + missing)。
	missing := []int{}
	received := map[int]bool{}
	for _, i := range receivedIndexes(dir, meta) {
		received[i] = true
	}
	for i := 0; i < meta.TotalChunks; i++ {
		if !received[i] {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		return auth.NewError(http.StatusConflict, "upload_state_conflict",
			"未受信のチャンクがあります", map[string]any{"missing": missing})
	}

	// 順番に結合する。
	assembledPath := filepath.Join(dir, "assembled"+meta.FilenameExt)
	out, err := os.Create(assembledPath)
	if err != nil {
		return err
	}
	var total int64
	for i := 0; i < meta.TotalChunks; i++ {
		in, err := os.Open(filepath.Join(dir, chunkName(i)))
		if err != nil {
			_ = out.Close()
			_ = os.Remove(assembledPath)
			return auth.NewError(http.StatusConflict, "upload_state_conflict",
				"チャンクを読み込めませんでした", map[string]any{"missing": []int{i}})
		}
		n, copyErr := io.Copy(out, in)
		_ = in.Close()
		if copyErr != nil {
			_ = out.Close()
			_ = os.Remove(assembledPath)
			return copyErr
		}
		total += n
	}
	if err := out.Close(); err != nil {
		return err
	}
	if total != meta.Size {
		_ = os.Remove(assembledPath)
		return auth.NewError(http.StatusConflict, "upload_state_conflict",
			"ファイルサイズが一致しません", nil)
	}

	file, err := filesystem.NewFileFromPath(assembledPath)
	if err != nil {
		return err
	}
	file.Name = "video_" + security.RandomString(16) + meta.FilenameExt
	file.OriginalName = file.Name // 元ファイル名は破棄(SPEC §11)

	work.Set("video", file)
	work.Set("video_status", "processing")
	work.Set("active_upload_id", "")
	if err := e.App.Save(work); err != nil {
		return err
	}

	_ = os.RemoveAll(dir)

	return e.JSON(http.StatusOK, map[string]any{"work": work})
}

// ---------------------------------------------------------------
// DELETE /api/x/works/{id}/video/chunk/{upload_id} (SPEC §8.5.5)
// ---------------------------------------------------------------

func abort(e *core.RequestEvent) error {
	ev, err := auth.RequireEvent(e)
	if err != nil {
		return err
	}
	if err := auth.RequireOpen(ev); err != nil {
		return err
	}
	work, _, err := auth.RequireEditableWork(e, ev, e.Request.PathValue("id"))
	if err != nil {
		return err
	}
	_, dir, err := resolveUpload(e, work)
	if err != nil {
		return err
	}

	_ = os.RemoveAll(dir)
	work.Set("active_upload_id", "")
	if work.GetString("video") != "" {
		work.Set("video_status", "ready")
	} else {
		work.Set("video_status", "none")
	}
	if err := e.App.Save(work); err != nil {
		return err
	}
	return e.NoContent(http.StatusNoContent)
}

// ---------------------------------------------------------------
// 掃除 cron (SPEC §8.5.6)
// ---------------------------------------------------------------

func uploadTTL() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("WORKS_UPLOAD_TTL")); err == nil && d > 0 {
		return d
	}
	return 24 * time.Hour
}

// CleanupOnce は期限切れ・壊れたアップロードディレクトリを削除し、
// 該当作品の状態を戻す。cron から15分ごとに呼ばれる。
func CleanupOnce(app core.App) {
	root := UploadsDir(app)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	ttl := uploadTTL()
	now := time.Now()

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "_transcode" {
			continue
		}
		dir := filepath.Join(root, entry.Name())

		meta, err := loadMeta(dir)
		if err != nil {
			// meta.json の無い壊れたディレクトリ。作成直後の競合を避けるため
			// 更新から1時間以上経ったものだけ削除する。
			if info, statErr := entry.Info(); statErr == nil && now.Sub(info.ModTime()) > time.Hour {
				_ = os.RemoveAll(dir)
			}
			continue
		}
		if now.Sub(meta.Created) <= ttl {
			continue
		}

		_ = os.RemoveAll(dir)

		work, err := app.FindRecordById("works", meta.WorkID)
		if err != nil || work.GetString("active_upload_id") != entry.Name() {
			continue
		}
		work.Set("active_upload_id", "")
		if work.GetString("video") != "" {
			work.Set("video_status", "ready")
		} else {
			work.Set("video_status", "none")
		}
		if err := app.Save(work); err != nil {
			app.Logger().Warn("cleanup_uploads: 作品状態の復元に失敗しました", "work", meta.WorkID, "error", err)
		}
	}
}
