package media

import (
	"reflect"
	"testing"
)

// SPEC §10.2 の ffmpeg 引数 + §11 の -map_metadata -1 が正確に組み立てられること。
func TestBuildTranscodeArgs(t *testing.T) {
	got := BuildTranscodeArgs("/in/src.mov", "/out/out.mp4", 2)
	want := []string{
		"-y", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-i", "/in/src.mov",
		"-vf", "scale='if(gt(a,1),min(1920,iw),-2)':'if(gt(a,1),-2,min(1920,ih))'",
		"-map_metadata", "-1",
		"-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k", "-ac", "2",
		"-movflags", "+faststart",
		"-threads", "2",
		"/out/out.mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildTranscodeArgs mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildThumbArgs(t *testing.T) {
	got := BuildThumbArgs("/in/src.mp4", "/out/thumb.jpg", "1")
	want := []string{
		"-y", "-nostdin", "-hide_banner", "-loglevel", "error",
		"-ss", "1", "-i", "/in/src.mp4",
		"-frames:v", "1", "-vf", "scale=640:-2", "-q:v", "4",
		"/out/thumb.jpg",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildThumbArgs mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildProbeArgs(t *testing.T) {
	got := BuildProbeArgs("/in/src.mp4")
	want := []string{
		"-v", "error",
		"-show_entries", "format=duration:stream=codec_type",
		"-of", "json",
		"/in/src.mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildProbeArgs mismatch:\n got: %v\nwant: %v", got, want)
	}
}

func TestEnvDefaults(t *testing.T) {
	if FFmpegBin() != "ffmpeg" || FFprobeBin() != "ffprobe" || ExiftoolBin() != "exiftool" {
		t.Fatal("既定のバイナリ名が想定と異なります")
	}
	if Threads() != 2 {
		t.Fatalf("Threads() = %d, want 2", Threads())
	}
	if TranscodeTimeout().Minutes() != 90 {
		t.Fatalf("TranscodeTimeout() = %v, want 90m", TranscodeTimeout())
	}
	if MaxVideoSeconds() != 1800 {
		t.Fatalf("MaxVideoSeconds() = %v, want 1800", MaxVideoSeconds())
	}
	t.Setenv("WORKS_FFMPEG_BIN", "/custom/ffmpeg")
	t.Setenv("WORKS_FFMPEG_THREADS", "4")
	if FFmpegBin() != "/custom/ffmpeg" || Threads() != 4 {
		t.Fatal("環境変数による上書きが効いていません")
	}
}
