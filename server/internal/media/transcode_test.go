package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	_ "github.com/ncc-toda/ncc-hub/server/internal/migrations"
)

// 実際の ffmpeg / ffprobe を使った変換の一気通貫テスト。
// バイナリが無い環境ではスキップする。音声なし動画が成功することも兼ねて確認する。
func TestTranscodeEndToEnd(t *testing.T) {
	if _, err := exec.LookPath(FFmpegBin()); err != nil {
		t.Skip("ffmpeg が見つからないためスキップ")
	}
	if _, err := exec.LookPath(FFprobeBin()); err != nil {
		t.Skip("ffprobe が見つからないためスキップ")
	}

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	// イベントと作品を用意
	evCol, err := app.FindCollectionByNameOrId("events")
	if err != nil {
		t.Fatal(err)
	}
	ev := core.NewRecord(evCol)
	ev.Set("name", "テスト")
	ev.Set("slug", "media-test")
	ev.Set("passphrase", "aikotoba-123")
	ev.Set("max_video_bytes", 2147483648)
	ev.Set("submissions_open", true)
	if err := app.Save(ev); err != nil {
		t.Fatal(err)
	}

	workCol, err := app.FindCollectionByNameOrId("works")
	if err != nil {
		t.Fatal(err)
	}
	work := core.NewRecord(workCol)
	work.Set("event", ev.Id)
	work.Set("description", "動画つき作品の説明")
	work.Set("video_status", "none")

	// 音声なしの1秒テスト動画を生成
	src := filepath.Join(t.TempDir(), "src.mp4")
	gen := exec.Command(FFmpegBin(), "-y", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=10",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("テスト動画の生成に失敗: %v %s", err, out)
	}

	f, err := filesystem.NewFileFromPath(src)
	if err != nil {
		t.Fatal(err)
	}
	work.Set("video", f)
	work.Set("video_status", "processing")
	if err := app.Save(work); err != nil {
		t.Fatal(err)
	}
	oldName := work.GetString("video")

	RunTranscodeOnce(app)

	rec, err := app.FindRecordById("works", work.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.GetString("video_status"); got != "ready" {
		t.Fatalf("video_status = %q (video_error=%q), want ready", got, rec.GetString("video_error"))
	}
	video := rec.GetString("video")
	if !regexp.MustCompile(`^video_[a-zA-Z0-9]{16}\.mp4$`).MatchString(video) {
		t.Fatalf("変換後の動画ファイル名が不正: %q", video)
	}
	thumb := rec.GetString("thumbnail")
	if !regexp.MustCompile(`^thumb_[a-zA-Z0-9]{16}\.jpg$`).MatchString(thumb) {
		t.Fatalf("サムネイルのファイル名が不正: %q", thumb)
	}
	storage := filepath.Join(app.DataDir(), "storage", rec.BaseFilesPath())
	for _, name := range []string{video, thumb} {
		if _, err := os.Stat(filepath.Join(storage, name)); err != nil {
			t.Fatalf("生成ファイルが存在しません: %s (%v)", name, err)
		}
	}
	// 置き換え前の元動画は PocketBase により削除される(SPEC §10.1)
	if _, err := os.Stat(filepath.Join(storage, oldName)); !os.IsNotExist(err) {
		t.Fatalf("置き換え前の元動画が残っています: %s", oldName)
	}
	// 一時ディレクトリは削除済み
	if _, err := os.Stat(filepath.Join(app.DataDir(), "uploads", "_transcode", rec.Id)); !os.IsNotExist(err) {
		t.Fatal("_transcode の一時ディレクトリが残っています")
	}
}
