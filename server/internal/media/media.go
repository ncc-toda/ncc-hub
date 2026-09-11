// Package media は ffmpeg / ffprobe / exiftool の呼び出しと
// 動画変換 cron を提供する(SPEC §10)。
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"github.com/pocketbase/pocketbase/tools/security"
)

// ---------------------------------------------------------------
// 環境変数 (SPEC §10.5)
// ---------------------------------------------------------------

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func FFmpegBin() string   { return envOr("WORKS_FFMPEG_BIN", "ffmpeg") }
func FFprobeBin() string  { return envOr("WORKS_FFPROBE_BIN", "ffprobe") }
func ExiftoolBin() string { return envOr("WORKS_EXIFTOOL_BIN", "exiftool") }

func Threads() int {
	if n, err := strconv.Atoi(os.Getenv("WORKS_FFMPEG_THREADS")); err == nil && n > 0 {
		return n
	}
	return 2
}

func TranscodeTimeout() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("WORKS_TRANSCODE_TIMEOUT")); err == nil && d > 0 {
		return d
	}
	return 90 * time.Minute
}

func MaxVideoSeconds() float64 {
	if n, err := strconv.ParseFloat(os.Getenv("WORKS_MAX_VIDEO_SECONDS"), 64); err == nil && n > 0 {
		return n
	}
	return 1800
}

// ---------------------------------------------------------------
// コマンド組み立て (SPEC §10.2, §10.3, §11)
// ---------------------------------------------------------------

// BuildTranscodeArgs は SPEC §10.2 の ffmpeg 引数に §11 の -map_metadata -1 を加えたもの。
func BuildTranscodeArgs(in, out string, threads int) []string {
	return []string{
		"-y", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-i", in,
		"-vf", "scale='if(gt(a,1),min(1920,iw),-2)':'if(gt(a,1),-2,min(1920,ih))'",
		"-map_metadata", "-1",
		"-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		"-movflags", "+faststart",
		"-threads", strconv.Itoa(threads),
		out,
	}
}

// BuildThumbArgs はサムネイル抽出の ffmpeg 引数(SPEC §10.2)。
func BuildThumbArgs(in, out, seek string) []string {
	return []string{
		"-y", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", seek, "-i", in,
		"-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "4",
		out,
	}
}

// BuildProbeArgs は事前検査の ffprobe 引数(SPEC §10.3)。
func BuildProbeArgs(in string) []string {
	return []string{
		"-v", "error",
		"-show_entries", "format=duration:stream=codec_type",
		"-of", "json",
		in,
	}
}

// ProbeResult は ffprobe の解析結果。
type ProbeResult struct {
	HasVideo bool
	Duration float64
}

func probe(ctx context.Context, in string) (*ProbeResult, error) {
	out, err := exec.CommandContext(ctx, FFprobeBin(), BuildProbeArgs(in)...).Output()
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}
	res := &ProbeResult{}
	for _, s := range parsed.Streams {
		if s.CodecType == "video" {
			res.HasVideo = true
		}
	}
	res.Duration, _ = strconv.ParseFloat(parsed.Format.Duration, 64)
	return res, nil
}

// ---------------------------------------------------------------
// 画像の EXIF 除去 (SPEC §10.4)
// ---------------------------------------------------------------

// StripImageMetadata は保存済み画像に対して exiftool -all= を同期実行する。
// 失敗しても警告ログのみでエラーは返さない(SPEC §10.4)。
func StripImageMetadata(app core.App, record *core.Record) {
	names := record.GetStringSlice("images")
	if len(names) == 0 {
		return
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, filepath.Join(app.DataDir(), "storage", record.BaseFilesPath(), name))
	}
	args := append([]string{"-all=", "-overwrite_original", "-q"}, paths...)
	if out, err := exec.Command(ExiftoolBin(), args...).CombinedOutput(); err != nil {
		app.Logger().Warn("exiftool による EXIF 除去に失敗しました",
			"error", err, "output", string(out), "work", record.Id)
	}
}

// ---------------------------------------------------------------
// 動画変換 cron (SPEC §10.1)
// ---------------------------------------------------------------

var transcodeMu sync.Mutex

// Register は毎分実行の transcode cron を登録する。
func Register(app core.App) {
	app.Cron().MustAdd("transcode", "* * * * *", func() {
		RunTranscodeOnce(app)
	})
}

// RunTranscodeOnce は video_status=processing の最も古い作品を1件変換する。
// 同時実行は1(TryLock が取れなければスキップ)。
func RunTranscodeOnce(app core.App) {
	if !transcodeMu.TryLock() {
		return
	}
	defer transcodeMu.Unlock()

	records, err := app.FindRecordsByFilter("works", "video_status = 'processing'", "updated", 1, 0)
	if err != nil || len(records) == 0 {
		return
	}
	transcodeRecord(app, records[0])
}

const failedUserMessage = "動画を変換できませんでした。別の形式で書き出すか、YouTube の限定公開URLをお使いください"

func transcodeRecord(app core.App, record *core.Record) {
	fail := func(reason string, logArgs ...any) {
		record.Set("video_status", "failed")
		record.Set("video_error", reason)
		if err := app.Save(record); err != nil {
			app.Logger().Error("変換失敗状態の保存に失敗しました", "work", record.Id, "error", err)
		}
		args := append([]any{"work", record.Id, "reason", reason}, logArgs...)
		app.Logger().Error("transcode failed", args...)
	}

	videoName := record.GetString("video")
	if videoName == "" {
		fail(failedUserMessage, "error", "video file missing on record")
		return
	}
	in := filepath.Join(app.DataDir(), "storage", record.BaseFilesPath(), videoName)
	if _, err := os.Stat(in); err != nil {
		fail(failedUserMessage, "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), TranscodeTimeout())
	defer cancel()

	// 事前検査 (SPEC §10.3)
	pr, err := probe(ctx, in)
	if err != nil {
		fail(failedUserMessage, "error", err)
		return
	}
	if !pr.HasVideo {
		fail("動画ファイルとして読み込めませんでした。映像の入ったファイルをアップロードしてください")
		return
	}
	maxSec := MaxVideoSeconds()
	if pr.Duration > maxSec {
		fail(fmt.Sprintf("動画が長すぎます。%d分以内にしてください", int(maxSec)/60))
		return
	}

	outDir := filepath.Join(app.DataDir(), "uploads", "_transcode", record.Id)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fail(failedUserMessage, "error", err)
		return
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	outPath := filepath.Join(outDir, "out.mp4")
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, FFmpegBin(), BuildTranscodeArgs(in, outPath, Threads())...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		fail(failedUserMessage, "error", err, "stderr", tail(stderr.Bytes(), 2048))
		return
	}

	// サムネイル: -ss 1 → 失敗したら -ss 0 で再試行 (SPEC §10.2)
	thumbPath := filepath.Join(outDir, "thumb.jpg")
	thumbOK := extractThumb(ctx, in, thumbPath, "1") || extractThumb(ctx, in, thumbPath, "0")
	if !thumbOK {
		app.Logger().Warn("サムネイル生成に失敗しました", "work", record.Id)
	}

	videoFile, err := filesystem.NewFileFromPath(outPath)
	if err != nil {
		fail(failedUserMessage, "error", err)
		return
	}
	videoFile.Name = "video_" + security.RandomString(16) + ".mp4"
	videoFile.OriginalName = videoFile.Name
	record.Set("video", videoFile)

	if thumbOK {
		thumbFile, err := filesystem.NewFileFromPath(thumbPath)
		if err == nil {
			thumbFile.Name = "thumb_" + security.RandomString(16) + ".jpg"
			thumbFile.OriginalName = thumbFile.Name
			record.Set("thumbnail", thumbFile)
		}
	}

	record.Set("video_status", "ready")
	record.Set("video_error", "")
	if err := app.Save(record); err != nil {
		fail(failedUserMessage, "error", err)
		return
	}
	app.Logger().Info("transcode finished", "work", record.Id)
}

func extractThumb(ctx context.Context, in, out, seek string) bool {
	if err := exec.CommandContext(ctx, FFmpegBin(), BuildThumbArgs(in, out, seek)...).Run(); err != nil {
		return false
	}
	info, err := os.Stat(out)
	return err == nil && info.Size() > 0
}

func tail(b []byte, n int) string {
	if len(b) > n {
		b = b[len(b)-n:]
	}
	return string(b)
}
