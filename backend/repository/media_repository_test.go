package repository

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"coffee-reel/entity"
)

func TestMediaErrorError(t *testing.T) {
	err := &MediaError{Message: "media failed"}
	if got := err.Error(); got != "media failed" {
		t.Fatalf("Error() = %q, want %q", got, "media failed")
	}
}

func TestNewMediaRepository(t *testing.T) {
	t.Run("設定値を正規化して生成する", func(t *testing.T) {
		repository, err := NewMediaRepository("  /usr/bin/ffprobe  ", "  /usr/bin/ffmpeg  ")
		if err != nil {
			t.Fatalf("NewMediaRepository() error = %v", err)
		}

		media, ok := repository.(*mediaRepository)
		if !ok {
			t.Fatalf("repository type = %T, want *mediaRepository", repository)
		}
		if media.ffprobePath != "/usr/bin/ffprobe" {
			t.Fatalf("ffprobePath = %q", media.ffprobePath)
		}
		if media.ffmpegPath != "/usr/bin/ffmpeg" {
			t.Fatalf("ffmpegPath = %q", media.ffmpegPath)
		}
	})

	tests := []struct {
		name        string
		ffprobePath string
		ffmpegPath  string
	}{
		{name: "ffprobeが空", ffprobePath: "", ffmpegPath: "/usr/bin/ffmpeg"},
		{name: "ffprobeが空白のみ", ffprobePath: "  ", ffmpegPath: "/usr/bin/ffmpeg"},
		{name: "ffmpegが空", ffprobePath: "/usr/bin/ffprobe", ffmpegPath: ""},
		{name: "ffmpegが空白のみ", ffprobePath: "/usr/bin/ffprobe", ffmpegPath: "  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository, err := NewMediaRepository(tt.ffprobePath, tt.ffmpegPath)
			if err == nil {
				t.Fatalf("NewMediaRepository() repository = %#v, want error", repository)
			}
		})
	}
}

func TestMediaRepositoryProbe(t *testing.T) {
	t.Run("空のパスを拒否する", func(t *testing.T) {
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})

		_, err := repository.Probe(context.Background(), "  ")
		if !errors.Is(err, entity.ErrInvalidInput) {
			t.Fatalf("Probe() error = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("存在しないファイルを破損として扱う", func(t *testing.T) {
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})

		_, err := repository.Probe(context.Background(), filepath.Join(t.TempDir(), "missing.mp4"))
		assertMediaError(t, err, entity.VideoFailureCorrupt, false, "video file could not be read")
	})

	t.Run("実体が未対応形式なら拡張子に関係なく拒否する", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fake.mp4")
		writeFile(t, path, []byte("this is not a video"))
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})

		_, err := repository.Probe(context.Background(), path)
		assertMediaError(t, err, entity.VideoFailureInvalidFormat, false, "video format is not supported")
	})

	t.Run("条件内のMP4からSourceVideoMetaを返す", func(t *testing.T) {
		path := writeMP4File(t)
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})

		meta, err := repository.Probe(context.Background(), path)
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if meta.MIMEType != "video/mp4" || meta.Container != "mp4" {
			t.Fatalf("format = %q/%q", meta.MIMEType, meta.Container)
		}
		if meta.Width != 720 || meta.Height != 1280 {
			t.Fatalf("resolution = %dx%d", meta.Width, meta.Height)
		}
		if math.Abs(meta.FrameRate-(30000.0/1001.0)) > 0.0001 {
			t.Fatalf("FrameRate = %v", meta.FrameRate)
		}
		if !meta.HasAudio || meta.AudioCodec != "aac" {
			t.Fatalf("audio = has:%v codec:%q", meta.HasAudio, meta.AudioCodec)
		}
	})

	t.Run("QuickTimeの回転情報を表示解像度へ反映する", func(t *testing.T) {
		path := writeQuickTimeFile(t)
		probe := validMOVProbeOutput()
		probe.Streams[0].Width = 1920
		probe.Streams[0].Height = 1080
		probe.Streams[0].Tags.Rotate = "90"
		repository := newTestMediaRepository(t, probe, fakeCommandOptions{})

		meta, err := repository.Probe(context.Background(), path)
		if err != nil {
			t.Fatalf("Probe() error = %v", err)
		}
		if meta.MIMEType != "video/quicktime" || meta.Container != "mov" {
			t.Fatalf("format = %q/%q", meta.MIMEType, meta.Container)
		}
		if meta.Width != 1080 || meta.Height != 1920 {
			t.Fatalf("rotated resolution = %dx%d", meta.Width, meta.Height)
		}
	})

	t.Run("FFprobeの期限超過を再試行可能として返す", func(t *testing.T) {
		path := writeMP4File(t)
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{sleep: time.Second})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := repository.Probe(ctx, path)
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, true, "media probe timed out")
	})

	t.Run("FFprobeの通常失敗を破損として返す", func(t *testing.T) {
		path := writeMP4File(t)
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{exitCode: 1})

		_, err := repository.Probe(context.Background(), path)
		assertMediaError(t, err, entity.VideoFailureCorrupt, false, "video could not be analyzed")
	})

	t.Run("FFprobeの不正JSONを破損として返す", func(t *testing.T) {
		path := writeMP4File(t)
		repository := newTestMediaRepositoryWithOutput(t, "not-json", fakeCommandOptions{})

		_, err := repository.Probe(context.Background(), path)
		assertMediaError(t, err, entity.VideoFailureCorrupt, false, "video could not be analyzed")
	})
}

func TestMediaRepositoryTranscode(t *testing.T) {
	tests := []struct {
		name       string
		inputPath  string
		outputPath string
	}{
		{name: "入力パスが空", inputPath: "", outputPath: "output.mp4"},
		{name: "入力パスが空白のみ", inputPath: "  ", outputPath: "output.mp4"},
		{name: "出力パスが空", inputPath: "input.mp4", outputPath: ""},
		{name: "出力パスが空白のみ", inputPath: "input.mp4", outputPath: "  "},
		{name: "入出力パスが同じ", inputPath: "same.mp4", outputPath: "same.mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})
			err := repository.Transcode(context.Background(), tt.inputPath, tt.outputPath, false)
			if !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("Transcode() error = %v, want ErrInvalidInput", err)
			}
		})
	}

	t.Run("音声ありの変換引数を固定する", func(t *testing.T) {
		argsPath := filepath.Join(t.TempDir(), "args.txt")
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{argsPath: argsPath})
		inputPath := filepath.Join(t.TempDir(), "input video.mp4")
		outputPath := filepath.Join(t.TempDir(), "output video.mp4")

		if err := repository.Transcode(context.Background(), inputPath, outputPath, true); err != nil {
			t.Fatalf("Transcode() error = %v", err)
		}

		want := []string{
			"-nostdin",
			"-hide_banner",
			"-loglevel", "error",
			"-y",
			"-i", inputPath,
			"-map", "0:v:0",
			"-vf", "scale=720:1280:flags=lanczos,fps=30",
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-threads:v", "1",
			"-crf", "23",
			"-maxrate", "3000k",
			"-bufsize", "6000k",
			"-pix_fmt", "yuv420p",
			"-map_metadata", "-1",
			"-map_chapters", "-1",
			"-movflags", "+faststart",
			"-map", "0:a:0?",
			"-c:a", "aac",
			"-b:a", "128k",
			"-f", "mp4",
			outputPath,
		}
		assertCommandArgs(t, argsPath, want)
	})

	t.Run("音声なしではAudio Trackを出力しない", func(t *testing.T) {
		argsPath := filepath.Join(t.TempDir(), "args.txt")
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{argsPath: argsPath})

		if err := repository.Transcode(context.Background(), "input.mp4", "output.mp4", false); err != nil {
			t.Fatalf("Transcode() error = %v", err)
		}

		args := readCommandArgs(t, argsPath)
		if !containsArgumentSequence(args, []string{"-an", "-f", "mp4", "output.mp4"}) {
			t.Fatalf("args = %#v, want -an followed by MP4 output", args)
		}
		if containsArgument(args, "-c:a") || containsArgument(args, "0:a:0?") {
			t.Fatalf("args = %#v, audio options must not be present", args)
		}
	})

	t.Run("FFmpeg失敗を再試行可能な処理失敗として返す", func(t *testing.T) {
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{exitCode: 1})

		err := repository.Transcode(context.Background(), "input.mp4", "output.mp4", true)
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, true, "video transcoding failed")
	})

	t.Run("Context終了時も再試行可能な処理失敗として返す", func(t *testing.T) {
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{sleep: time.Second})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		err := repository.Transcode(ctx, "input.mp4", "output.mp4", false)
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, true, "video transcoding failed")
	})

	t.Run("ファイルパスをShell命令として解釈しない", func(t *testing.T) {
		dir := t.TempDir()
		sentinel := filepath.Join(dir, "injected")
		inputPath := "input.mp4;touch " + sentinel
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})

		if err := repository.Transcode(context.Background(), inputPath, "output.mp4", false); err != nil {
			t.Fatalf("Transcode() error = %v", err)
		}
		if _, err := os.Stat(sentinel); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("shell command was interpreted, Stat() error = %v", err)
		}
	})
}

func TestMediaRepositoryGenerateThumbnail(t *testing.T) {
	tests := []struct {
		name       string
		inputPath  string
		outputPath string
	}{
		{name: "入力パスが空", inputPath: "", outputPath: "thumb.jpg"},
		{name: "出力パスが空", inputPath: "input.mp4", outputPath: ""},
		{name: "入出力パスが同じ", inputPath: "same.file", outputPath: "same.file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})
			err := repository.GenerateThumbnail(context.Background(), tt.inputPath, tt.outputPath)
			if !errors.Is(err, entity.ErrInvalidInput) {
				t.Fatalf("GenerateThumbnail() error = %v, want ErrInvalidInput", err)
			}
		})
	}

	t.Run("JPEGサムネイル生成引数を固定する", func(t *testing.T) {
		argsPath := filepath.Join(t.TempDir(), "args.txt")
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{argsPath: argsPath})
		inputPath := filepath.Join(t.TempDir(), "input video.mp4")
		outputPath := filepath.Join(t.TempDir(), "thumbnail image.jpg")

		if err := repository.GenerateThumbnail(context.Background(), inputPath, outputPath); err != nil {
			t.Fatalf("GenerateThumbnail() error = %v", err)
		}

		want := []string{
			"-nostdin",
			"-hide_banner",
			"-loglevel", "error",
			"-y",
			"-ss", "0",
			"-i", inputPath,
			"-frames:v", "1",
			"-vf", "scale=360:640:flags=lanczos",
			"-c:v", "mjpeg",
			"-f", "image2",
			outputPath,
		}
		assertCommandArgs(t, argsPath, want)
	})

	t.Run("FFmpeg失敗を再試行可能な処理失敗として返す", func(t *testing.T) {
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{exitCode: 1})

		err := repository.GenerateThumbnail(context.Background(), "input.mp4", "thumb.jpg")
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, true, "thumbnail generation failed")
	})
}

func TestMediaRepositoryProbeOutput(t *testing.T) {
	t.Run("空のパスを拒否する", func(t *testing.T) {
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})

		_, err := repository.ProbeOutput(context.Background(), " ")
		if !errors.Is(err, entity.ErrInvalidInput) {
			t.Fatalf("ProbeOutput() error = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("存在しない出力ファイルを処理失敗として返す", func(t *testing.T) {
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})

		_, err := repository.ProbeOutput(context.Background(), filepath.Join(t.TempDir(), "missing.mp4"))
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, false, "output video could not be read")
	})

	t.Run("MP4以外の出力を拒否する", func(t *testing.T) {
		path := writeQuickTimeFile(t)
		repository := newTestMediaRepository(t, validMOVProbeOutput(), fakeCommandOptions{})

		_, err := repository.ProbeOutput(context.Background(), path)
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, false, "output video format is invalid")
	})

	t.Run("音声ありの公開条件を満たす出力を返す", func(t *testing.T) {
		path := writeMP4File(t)
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{})

		meta, err := repository.ProbeOutput(context.Background(), path)
		if err != nil {
			t.Fatalf("ProbeOutput() error = %v", err)
		}
		if meta.Container != "mp4" || meta.Width != 720 || meta.Height != 1280 {
			t.Fatalf("meta = %#v", meta)
		}
		if meta.VideoCodec != "h264" || !meta.HasAudio || meta.AudioCodec != "aac" {
			t.Fatalf("codecs = video:%q hasAudio:%v audio:%q", meta.VideoCodec, meta.HasAudio, meta.AudioCodec)
		}
	})

	t.Run("無音の公開条件を満たす出力を返す", func(t *testing.T) {
		path := writeMP4File(t)
		probe := validMP4ProbeOutput()
		probe.Streams = probe.Streams[:1]
		repository := newTestMediaRepository(t, probe, fakeCommandOptions{})

		meta, err := repository.ProbeOutput(context.Background(), path)
		if err != nil {
			t.Fatalf("ProbeOutput() error = %v", err)
		}
		if meta.HasAudio || meta.AudioCodec != "" {
			t.Fatalf("audio = has:%v codec:%q", meta.HasAudio, meta.AudioCodec)
		}
	})

	t.Run("回転後に720x1280となる出力を許可する", func(t *testing.T) {
		path := writeMP4File(t)
		probe := validMP4ProbeOutput()
		probe.Streams[0].Width = 1280
		probe.Streams[0].Height = 720
		probe.Streams[0].SideDataList = []ffprobeSideData{{Rotation: -90}}
		repository := newTestMediaRepository(t, probe, fakeCommandOptions{})

		meta, err := repository.ProbeOutput(context.Background(), path)
		if err != nil {
			t.Fatalf("ProbeOutput() error = %v", err)
		}
		if meta.Width != 720 || meta.Height != 1280 {
			t.Fatalf("resolution = %dx%d", meta.Width, meta.Height)
		}
	})

	t.Run("FFprobeの期限超過を再試行可能として返す", func(t *testing.T) {
		path := writeMP4File(t)
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{sleep: time.Second})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := repository.ProbeOutput(ctx, path)
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, true, "output media probe timed out")
	})

	t.Run("Container情報の矛盾を拒否する", func(t *testing.T) {
		path := writeMP4File(t)
		probe := validMP4ProbeOutput()
		probe.Format.Tags.MajorBrand = "qt"
		repository := newTestMediaRepository(t, probe, fakeCommandOptions{})

		_, err := repository.ProbeOutput(context.Background(), path)
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, false, "output video container is invalid")
	})

	t.Run("Video Trackがない出力を拒否する", func(t *testing.T) {
		path := writeMP4File(t)
		probe := validMP4ProbeOutput()
		probe.Streams = probe.Streams[1:]
		repository := newTestMediaRepository(t, probe, fakeCommandOptions{})

		_, err := repository.ProbeOutput(context.Background(), path)
		assertMediaError(t, err, entity.VideoFailureVideoTrackMissing, false, "output video track is missing")
	})

	t.Run("不正なFrame Rateを拒否する", func(t *testing.T) {
		path := writeMP4File(t)
		probe := validMP4ProbeOutput()
		probe.Streams[0].AvgFrameRate = "0/0"
		repository := newTestMediaRepository(t, probe, fakeCommandOptions{})

		_, err := repository.ProbeOutput(context.Background(), path)
		assertMediaError(t, err, entity.VideoFailureProcessingFailed, false, "output frame rate is invalid")
	})

	invalidOutputs := []struct {
		name   string
		mutate func(*ffprobeOutput)
	}{
		{name: "横幅が異なる", mutate: func(probe *ffprobeOutput) { probe.Streams[0].Width = 721 }},
		{name: "高さが異なる", mutate: func(probe *ffprobeOutput) { probe.Streams[0].Height = 1279 }},
		{name: "30fpsを超える", mutate: func(probe *ffprobeOutput) { probe.Streams[0].AvgFrameRate = "31/1" }},
		{name: "Video CodecがH264ではない", mutate: func(probe *ffprobeOutput) { probe.Streams[0].CodecName = "hevc" }},
		{name: "Audio CodecがAACではない", mutate: func(probe *ffprobeOutput) { probe.Streams[1].CodecName = "mp3" }},
	}

	for _, tt := range invalidOutputs {
		t.Run(tt.name, func(t *testing.T) {
			path := writeMP4File(t)
			probe := validMP4ProbeOutput()
			tt.mutate(&probe)
			repository := newTestMediaRepository(t, probe, fakeCommandOptions{})

			_, err := repository.ProbeOutput(context.Background(), path)
			assertMediaError(t, err, entity.VideoFailureProcessingFailed, false, "output video does not meet publishing requirements")
		})
	}
}

func TestMediaRepositoryRunProbe(t *testing.T) {
	t.Run("固定引数で実行してJSONを解析する", func(t *testing.T) {
		argsPath := filepath.Join(t.TempDir(), "args.txt")
		wantOutput := validMP4ProbeOutput()
		repository := newTestMediaRepository(t, wantOutput, fakeCommandOptions{argsPath: argsPath})
		filePath := filepath.Join(t.TempDir(), "input video.mp4")

		got, err := repository.runProbe(context.Background(), filePath)
		if err != nil {
			t.Fatalf("runProbe() error = %v", err)
		}
		if !reflect.DeepEqual(got, wantOutput) {
			t.Fatalf("runProbe() = %#v, want %#v", got, wantOutput)
		}

		wantArgs := []string{
			"-v", "error",
			"-print_format", "json",
			"-show_format",
			"-show_streams",
			filePath,
		}
		assertCommandArgs(t, argsPath, wantArgs)
	})

	t.Run("不正JSONをDecode Errorとして返す", func(t *testing.T) {
		repository := newTestMediaRepositoryWithOutput(t, "{", fakeCommandOptions{})

		_, err := repository.runProbe(context.Background(), "input.mp4")
		if err == nil || !strings.Contains(err.Error(), "decode ffprobe output") {
			t.Fatalf("runProbe() error = %v", err)
		}
	})

	t.Run("Context終了を保持して返す", func(t *testing.T) {
		repository := newTestMediaRepository(t, validMP4ProbeOutput(), fakeCommandOptions{sleep: time.Second})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		_, err := repository.runProbe(ctx, "input.mp4")
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("runProbe() error = %v, want DeadlineExceeded", err)
		}
	})
}

func TestSourceMetaFromProbe(t *testing.T) {
	t.Run("仕様上限と同じ値を許可する", func(t *testing.T) {
		probe := validMP4ProbeOutput()
		probe.Format.Size = "50000000"
		probe.Format.Duration = "10.000"
		probe.Streams[0].Width = 1080
		probe.Streams[0].Height = 1920
		probe.Streams[0].AvgFrameRate = "60/1"

		meta, err := sourceMetaFromProbe(probe, "video/mp4")
		if err != nil {
			t.Fatalf("sourceMetaFromProbe() error = %v", err)
		}
		if meta.SizeBytes != 50_000_000 || meta.DurationMillis != 10_000 || meta.FrameRate != 60 {
			t.Fatalf("meta = %#v", meta)
		}
	})

	tests := []struct {
		name      string
		mimeType  string
		mutate    func(*ffprobeOutput)
		wantCode  entity.VideoFailureCode
		wantRetry bool
	}{
		{
			name:     "Containerを確認できない",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Format.FormatName = "matroska,webm"
			},
			wantCode: entity.VideoFailureInvalidFormat,
		},
		{
			name:     "Major Brandがない",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Format.Tags.MajorBrand = ""
			},
			wantCode: entity.VideoFailureInvalidFormat,
		},
		{
			name:     "Video Trackがない",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams = probe.Streams[1:]
			},
			wantCode: entity.VideoFailureVideoTrackMissing,
		},
		{
			name:     "解像度が0",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams[0].Width = 0
			},
			wantCode: entity.VideoFailureCorrupt,
		},
		{
			name:     "Frame Rateが不正",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams[0].AvgFrameRate = "0/0"
			},
			wantCode: entity.VideoFailureCorrupt,
		},
		{
			name:     "Durationが数値ではない",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Format.Duration = "unknown"
			},
			wantCode: entity.VideoFailureCorrupt,
		},
		{
			name:     "DurationがNaN",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Format.Duration = "NaN"
			},
			wantCode: entity.VideoFailureCorrupt,
		},
		{
			name:     "Durationが無限大",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Format.Duration = "+Inf"
			},
			wantCode: entity.VideoFailureCorrupt,
		},
		{
			name:     "Sizeが0",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Format.Size = "0"
			},
			wantCode: entity.VideoFailureCorrupt,
		},
		{
			name:     "50MBを超える",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Format.Size = "50000001"
			},
			wantCode: entity.VideoFailureSizeExceeded,
		},
		{
			name:     "10秒をわずかに超える",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Format.Duration = "10.0001"
			},
			wantCode: entity.VideoFailureDurationExceeded,
		},
		{
			name:     "最大横幅を超える",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams[0].Width = 1081
				probe.Streams[0].Height = 1920
			},
			wantCode: entity.VideoFailureResolutionExceeded,
		},
		{
			name:     "最大高さを超える",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams[0].Width = 1080
				probe.Streams[0].Height = 1921
			},
			wantCode: entity.VideoFailureResolutionExceeded,
		},
		{
			name:     "縦横比が9対16ではない",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams[0].Width = 720
				probe.Streams[0].Height = 1279
			},
			wantCode: entity.VideoFailureInvalidAspectRatio,
		},
		{
			name:     "60fpsを超える",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams[0].AvgFrameRate = "60001/1000"
			},
			wantCode: entity.VideoFailureFrameRateExceeded,
		},
		{
			name:     "Video Codecが空",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams[0].CodecName = " "
			},
			wantCode: entity.VideoFailureInvalidFormat,
		},
		{
			name:     "Audio TrackのCodecが空",
			mimeType: "video/mp4",
			mutate: func(probe *ffprobeOutput) {
				probe.Streams[1].CodecName = ""
			},
			wantCode: entity.VideoFailureInvalidFormat,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := validMP4ProbeOutput()
			tt.mutate(&probe)

			_, err := sourceMetaFromProbe(probe, tt.mimeType)
			assertMediaErrorCode(t, err, tt.wantCode, tt.wantRetry)
		})
	}
}

func TestNormalizeISOBaseMediaFormat(t *testing.T) {
	tests := []struct {
		name          string
		detectedMIME  string
		formatName    string
		majorBrand    string
		wantContainer string
		wantMIME      string
		wantOK        bool
	}{
		{
			name:          "MP4を正規化する",
			detectedMIME:  "video/mp4",
			formatName:    "mov,mp4,m4a,3gp,3g2,mj2",
			majorBrand:    " MP42 ",
			wantContainer: "mp4",
			wantMIME:      "video/mp4",
			wantOK:        true,
		},
		{
			name:          "QuickTimeを正規化する",
			detectedMIME:  "video/quicktime",
			formatName:    "mov,mp4,m4a,3gp,3g2,mj2",
			majorBrand:    "qt",
			wantContainer: "mov",
			wantMIME:      "video/quicktime",
			wantOK:        true,
		},
		{name: "非対応Containerを拒否する", detectedMIME: "video/mp4", formatName: "matroska", majorBrand: "mp42"},
		{name: "Major Brandなしを拒否する", detectedMIME: "video/mp4", formatName: "mov,mp4", majorBrand: ""},
		{name: "QuickTime MIMEとMP4 Brandの矛盾を拒否する", detectedMIME: "video/quicktime", formatName: "mov,mp4", majorBrand: "mp42"},
		{name: "MP4 MIMEとQuickTime Brandの矛盾を拒否する", detectedMIME: "video/mp4", formatName: "mov,mp4", majorBrand: "qt"},
		{name: "未対応MIMEを拒否する", detectedMIME: "application/octet-stream", formatName: "mov,mp4", majorBrand: "mp42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container, mimeType, ok := normalizeISOBaseMediaFormat(tt.detectedMIME, tt.formatName, tt.majorBrand)
			if container != tt.wantContainer || mimeType != tt.wantMIME || ok != tt.wantOK {
				t.Fatalf("normalizeISOBaseMediaFormat() = (%q, %q, %v), want (%q, %q, %v)", container, mimeType, ok, tt.wantContainer, tt.wantMIME, tt.wantOK)
			}
		})
	}
}

func TestContainsFormatName(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		target string
		want   bool
	}{
		{name: "複数形式からMP4を見つける", value: "mov,mp4,m4a,3gp", target: "mp4", want: true},
		{name: "大文字と空白を正規化する", value: " MOV, MP4 ", target: "mp4", want: true},
		{name: "部分一致を許可しない", value: "mp42,movx", target: "mp4", want: false},
		{name: "存在しない形式", value: "mov,m4a", target: "mp4", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsFormatName(tt.value, tt.target); got != tt.want {
				t.Fatalf("containsFormatName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectTracks(t *testing.T) {
	streams := []ffprobeStream{
		{CodecType: "subtitle", CodecName: "mov_text"},
		{CodecType: "audio", CodecName: "aac"},
		{CodecType: "video", CodecName: "h264"},
		{CodecType: "audio", CodecName: "opus"},
		{CodecType: "video", CodecName: "hevc"},
	}

	video, audio, ok := selectTracks(streams)
	if !ok {
		t.Fatal("selectTracks() ok = false")
	}
	if video == nil || video.CodecName != "h264" {
		t.Fatalf("video = %#v, want first video track", video)
	}
	if audio == nil || audio.CodecName != "aac" {
		t.Fatalf("audio = %#v, want first audio track", audio)
	}

	video, audio, ok = selectTracks([]ffprobeStream{{CodecType: "audio", CodecName: "aac"}})
	if ok || video != nil || audio == nil {
		t.Fatalf("audio-only result = video:%#v audio:%#v ok:%v", video, audio, ok)
	}
}

func TestRotationOf(t *testing.T) {
	tests := []struct {
		name   string
		stream *ffprobeStream
		want   int
	}{
		{name: "nilは0度", stream: nil, want: 0},
		{name: "Rotate Tagを優先する", stream: &ffprobeStream{Tags: ffprobeStreamTags{Rotate: "90"}, SideDataList: []ffprobeSideData{{Rotation: 270}}}, want: 90},
		{name: "Tagが不正ならSide Dataを使う", stream: &ffprobeStream{Tags: ffprobeStreamTags{Rotate: "invalid"}, SideDataList: []ffprobeSideData{{Rotation: -90}}}, want: -90},
		{name: "回転情報なしは0度", stream: &ffprobeStream{}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rotationOf(tt.stream); got != tt.want {
				t.Fatalf("rotationOf() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyRotation(t *testing.T) {
	tests := []struct {
		name                  string
		rotation              int
		wantWidth, wantHeight int
	}{
		{name: "0度", rotation: 0, wantWidth: 720, wantHeight: 1280},
		{name: "90度", rotation: 90, wantWidth: 1280, wantHeight: 720},
		{name: "180度", rotation: 180, wantWidth: 720, wantHeight: 1280},
		{name: "270度", rotation: 270, wantWidth: 1280, wantHeight: 720},
		{name: "360度は0度", rotation: 360, wantWidth: 720, wantHeight: 1280},
		{name: "450度は90度", rotation: 450, wantWidth: 1280, wantHeight: 720},
		{name: "マイナス90度は270度", rotation: -90, wantWidth: 1280, wantHeight: 720},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height := applyRotation(720, 1280, tt.rotation)
			if width != tt.wantWidth || height != tt.wantHeight {
				t.Fatalf("applyRotation() = %dx%d, want %dx%d", width, height, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}

func TestParseFrameRate(t *testing.T) {
	valid := []struct {
		name  string
		value string
		want  float64
	}{
		{name: "整数形式", value: "30", want: 30},
		{name: "分数形式", value: "30000/1001", want: 30000.0 / 1001.0},
		{name: "前後の空白", value: " 60/1 ", want: 60},
	}

	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFrameRate(tt.value)
			if err != nil {
				t.Fatalf("parseFrameRate() error = %v", err)
			}
			if math.Abs(got-tt.want) > 0.000001 {
				t.Fatalf("parseFrameRate() = %v, want %v", got, tt.want)
			}
		})
	}

	invalid := []string{"", "0", "-1", "NaN", "+Inf", "1/0", "abc", "1/abc", "1/2/3"}
	for _, value := range invalid {
		t.Run("不正値_"+strings.ReplaceAll(value, "/", "_"), func(t *testing.T) {
			if got, err := parseFrameRate(value); err == nil {
				t.Fatalf("parseFrameRate(%q) = %v, want error", value, got)
			}
		})
	}
}

func TestDetectMIME(t *testing.T) {
	t.Run("MP4の実体を判定する", func(t *testing.T) {
		path := writeMP4File(t)
		got, err := detectMIME(path)
		if err != nil {
			t.Fatalf("detectMIME() error = %v", err)
		}
		if got != "video/mp4" {
			t.Fatalf("detectMIME() = %q, want video/mp4", got)
		}
	})

	t.Run("QuickTimeの実体を判定する", func(t *testing.T) {
		path := writeQuickTimeFile(t)
		got, err := detectMIME(path)
		if err != nil {
			t.Fatalf("detectMIME() error = %v", err)
		}
		if got != "video/quicktime" {
			t.Fatalf("detectMIME() = %q, want video/quicktime", got)
		}
	})

	t.Run("拡張子ではなく内容を判定する", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fake.mp4")
		writeFile(t, path, []byte("plain text"))
		got, err := detectMIME(path)
		if err != nil {
			t.Fatalf("detectMIME() error = %v", err)
		}
		if got == "video/mp4" || got == "video/quicktime" {
			t.Fatalf("detectMIME() = %q, want non-video MIME", got)
		}
	})

	t.Run("空ファイルを拒否する", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.mp4")
		writeFile(t, path, nil)
		if got, err := detectMIME(path); err == nil {
			t.Fatalf("detectMIME() = %q, want error", got)
		}
	})

	t.Run("存在しないファイルを拒否する", func(t *testing.T) {
		if got, err := detectMIME(filepath.Join(t.TempDir(), "missing.mp4")); err == nil {
			t.Fatalf("detectMIME() = %q, want error", got)
		}
	})
}

func TestRunCommand(t *testing.T) {
	t.Run("引数を個別に渡して実行する", func(t *testing.T) {
		argsPath := filepath.Join(t.TempDir(), "args.txt")
		commandPath := writeFakeCommand(t, "", fakeCommandOptions{argsPath: argsPath})
		want := []string{"value with spaces", "semi;colon", "$(not-executed)"}

		if err := runCommand(context.Background(), commandPath, want...); err != nil {
			t.Fatalf("runCommand() error = %v", err)
		}
		assertCommandArgs(t, argsPath, want)
	})

	t.Run("Command失敗を返す", func(t *testing.T) {
		commandPath := writeFakeCommand(t, "", fakeCommandOptions{exitCode: 1})
		if err := runCommand(context.Background(), commandPath); err == nil {
			t.Fatal("runCommand() error = nil, want error")
		}
	})

	t.Run("Context終了を返す", func(t *testing.T) {
		commandPath := writeFakeCommand(t, "", fakeCommandOptions{sleep: time.Second})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		err := runCommand(ctx, commandPath)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("runCommand() error = %v, want DeadlineExceeded", err)
		}
	})
}

type fakeCommandOptions struct {
	argsPath string
	exitCode int
	sleep    time.Duration
}

func newTestMediaRepository(t *testing.T, output ffprobeOutput, options fakeCommandOptions) *mediaRepository {
	t.Helper()

	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return newTestMediaRepositoryWithOutput(t, string(data), options)
}

func newTestMediaRepositoryWithOutput(t *testing.T, output string, options fakeCommandOptions) *mediaRepository {
	t.Helper()

	commandPath := writeFakeCommand(t, output, options)
	repository, err := NewMediaRepository(commandPath, commandPath)
	if err != nil {
		t.Fatalf("NewMediaRepository() error = %v", err)
	}

	media, ok := repository.(*mediaRepository)
	if !ok {
		t.Fatalf("repository type = %T", repository)
	}
	return media
}

func writeFakeCommand(t *testing.T, stdout string, options fakeCommandOptions) string {
	t.Helper()

	dir := t.TempDir()
	commandPath := filepath.Join(dir, "fake-command")
	stdoutPath := filepath.Join(dir, "stdout.txt")
	writeFile(t, stdoutPath, []byte(stdout))

	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	if options.argsPath != "" {
		script.WriteString("printf '%s\\n' \"$@\" > ")
		script.WriteString(shellQuote(options.argsPath))
		script.WriteString("\n")
	}
	if options.sleep > 0 {
		script.WriteString("exec sleep ")
		script.WriteString(strconv.Itoa(int(math.Ceil(options.sleep.Seconds()))))
		script.WriteString("\n")
	}
	if stdout != "" {
		script.WriteString("cat ")
		script.WriteString(shellQuote(stdoutPath))
		script.WriteString("\n")
	}
	if options.exitCode != 0 {
		script.WriteString("echo fake command failed >&2\n")
	}
	script.WriteString("exit ")
	script.WriteString(strconv.Itoa(options.exitCode))
	script.WriteString("\n")

	if err := os.WriteFile(commandPath, []byte(script.String()), 0o700); err != nil {
		t.Fatalf("os.WriteFile(fake command) error = %v", err)
	}
	return commandPath
}

func validMP4ProbeOutput() ffprobeOutput {
	var output ffprobeOutput
	output.Format.FormatName = "mov,mp4,m4a,3gp,3g2,mj2"
	output.Format.Duration = "9.500"
	output.Format.Size = "1000000"
	output.Format.Tags.MajorBrand = "mp42"
	output.Streams = []ffprobeStream{
		{
			CodecType:    "video",
			CodecName:    "h264",
			Width:        720,
			Height:       1280,
			AvgFrameRate: "30000/1001",
		},
		{
			CodecType: "audio",
			CodecName: "aac",
		},
	}
	return output
}

func validMOVProbeOutput() ffprobeOutput {
	output := validMP4ProbeOutput()
	output.Format.Tags.MajorBrand = "qt"
	return output
}

func writeMP4File(t *testing.T) string {
	t.Helper()

	content := make([]byte, 24)
	binary.BigEndian.PutUint32(content[0:4], uint32(len(content)))
	copy(content[4:8], "ftyp")
	copy(content[8:12], "mp42")
	copy(content[16:20], "mp42")
	copy(content[20:24], "isom")

	path := filepath.Join(t.TempDir(), "video.mp4")
	writeFile(t, path, content)
	return path
}

func writeQuickTimeFile(t *testing.T) string {
	t.Helper()

	content := make([]byte, 20)
	binary.BigEndian.PutUint32(content[0:4], uint32(len(content)))
	copy(content[4:8], "ftyp")
	copy(content[8:12], "qt  ")
	copy(content[16:20], "qt  ")

	path := filepath.Join(t.TempDir(), "video.mov")
	writeFile(t, path, content)
	return path
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}

func assertMediaError(t *testing.T, err error, code entity.VideoFailureCode, retryable bool, message string) {
	t.Helper()

	var mediaError *MediaError
	if !errors.As(err, &mediaError) {
		t.Fatalf("error = %T %v, want *MediaError", err, err)
	}
	if mediaError.Code != code || mediaError.Retryable != retryable || mediaError.Message != message {
		t.Fatalf("MediaError = %#v, want code:%q retryable:%v message:%q", mediaError, code, retryable, message)
	}
}

func assertMediaErrorCode(t *testing.T, err error, code entity.VideoFailureCode, retryable bool) {
	t.Helper()

	var mediaError *MediaError
	if !errors.As(err, &mediaError) {
		t.Fatalf("error = %T %v, want *MediaError", err, err)
	}
	if mediaError.Code != code || mediaError.Retryable != retryable {
		t.Fatalf("MediaError = %#v, want code:%q retryable:%v", mediaError, code, retryable)
	}
}

func assertCommandArgs(t *testing.T, path string, want []string) {
	t.Helper()
	got := readCommandArgs(t, path)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("command args = %#v, want %#v", got, want)
	}
}

func readCommandArgs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", path, err)
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func containsArgument(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func containsArgumentSequence(args, sequence []string) bool {
	if len(sequence) == 0 || len(sequence) > len(args) {
		return false
	}
	for start := 0; start <= len(args)-len(sequence); start++ {
		if reflect.DeepEqual(args[start:start+len(sequence)], sequence) {
			return true
		}
	}
	return false
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
