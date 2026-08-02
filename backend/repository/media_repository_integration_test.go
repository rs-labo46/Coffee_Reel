//go:build integration

package repository

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"coffee-reel/entity"
)

func TestMediaRepositoryIntegrationProcessesVideoWithAudio(t *testing.T) {
	ffprobePath, ffmpegPath := requireMediaTools(t)
	repository := newIntegrationMediaRepository(t, ffprobePath, ffmpegPath)
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input-with-audio.mp4")
	outputPath := filepath.Join(dir, "output-with-audio.mp4")
	thumbnailPath := filepath.Join(dir, "thumbnail.jpg")

	createIntegrationVideo(t, ffmpegPath, inputPath, true, "mp4")

	sourceMeta, err := repository.Probe(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if !sourceMeta.HasAudio || sourceMeta.AudioCodec == "" {
		t.Fatalf("source audio = has:%v codec:%q", sourceMeta.HasAudio, sourceMeta.AudioCodec)
	}

	if err := repository.Transcode(context.Background(), inputPath, outputPath, sourceMeta.HasAudio); err != nil {
		t.Fatalf("Transcode() error = %v", err)
	}
	if err := repository.GenerateThumbnail(context.Background(), outputPath, thumbnailPath); err != nil {
		t.Fatalf("GenerateThumbnail() error = %v", err)
	}

	outputMeta, err := repository.ProbeOutput(context.Background(), outputPath)
	if err != nil {
		t.Fatalf("ProbeOutput() error = %v", err)
	}
	if outputMeta.Container != "mp4" || outputMeta.Width != 720 || outputMeta.Height != 1280 || outputMeta.FrameRate > 30 || outputMeta.VideoCodec != "h264" || !outputMeta.HasAudio || outputMeta.AudioCodec != "aac" {
		t.Fatalf("outputMeta = %#v", outputMeta)
	}
	assertJPEGFile(t, thumbnailPath)
}

func TestMediaRepositoryIntegrationProcessesSilentVideo(t *testing.T) {
	ffprobePath, ffmpegPath := requireMediaTools(t)
	repository := newIntegrationMediaRepository(t, ffprobePath, ffmpegPath)
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input-silent.mp4")
	outputPath := filepath.Join(dir, "output-silent.mp4")

	createIntegrationVideo(t, ffmpegPath, inputPath, false, "mp4")

	sourceMeta, err := repository.Probe(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if sourceMeta.HasAudio || sourceMeta.AudioCodec != "" {
		t.Fatalf("source audio = has:%v codec:%q", sourceMeta.HasAudio, sourceMeta.AudioCodec)
	}

	if err := repository.Transcode(context.Background(), inputPath, outputPath, sourceMeta.HasAudio); err != nil {
		t.Fatalf("Transcode() error = %v", err)
	}

	outputMeta, err := repository.ProbeOutput(context.Background(), outputPath)
	if err != nil {
		t.Fatalf("ProbeOutput() error = %v", err)
	}
	if outputMeta.HasAudio || outputMeta.AudioCodec != "" {
		t.Fatalf("output audio = has:%v codec:%q", outputMeta.HasAudio, outputMeta.AudioCodec)
	}
}

func TestMediaRepositoryIntegrationAcceptsQuickTimeInput(t *testing.T) {
	ffprobePath, ffmpegPath := requireMediaTools(t)
	repository := newIntegrationMediaRepository(t, ffprobePath, ffmpegPath)
	inputPath := filepath.Join(t.TempDir(), "input.mov")

	createIntegrationVideo(t, ffmpegPath, inputPath, false, "mov")

	meta, err := repository.Probe(context.Background(), inputPath)
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if meta.MIMEType != "video/quicktime" || meta.Container != "mov" {
		t.Fatalf("format = %q/%q", meta.MIMEType, meta.Container)
	}
}

func TestMediaRepositoryIntegrationRejectsDisguisedFile(t *testing.T) {
	ffprobePath, ffmpegPath := requireMediaTools(t)
	repository := newIntegrationMediaRepository(t, ffprobePath, ffmpegPath)
	path := filepath.Join(t.TempDir(), "disguised.mp4")
	if err := os.WriteFile(path, []byte("not a video"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := repository.Probe(context.Background(), path)
	var mediaError *MediaError
	if !errors.As(err, &mediaError) || mediaError.Code != entity.VideoFailureInvalidFormat || mediaError.Retryable {
		t.Fatalf("Probe() error = %#v", err)
	}
}

func requireMediaTools(t *testing.T) (string, string) {
	t.Helper()

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	return ffprobePath, ffmpegPath
}

func newIntegrationMediaRepository(t *testing.T, ffprobePath, ffmpegPath string) *mediaRepository {
	t.Helper()

	repository, err := NewMediaRepository(ffprobePath, ffmpegPath)
	if err != nil {
		t.Fatalf("NewMediaRepository() error = %v", err)
	}
	media, ok := repository.(*mediaRepository)
	if !ok {
		t.Fatalf("repository type = %T", repository)
	}
	return media
}

func createIntegrationVideo(t *testing.T, ffmpegPath, outputPath string, hasAudio bool, container string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "lavfi",
		"-i", "color=c=black:s=720x1280:r=30:d=1",
	}
	if hasAudio {
		args = append(args,
			"-f", "lavfi",
			"-i", "sine=frequency=1000:sample_rate=44100:duration=1",
			"-map", "0:v:0",
			"-map", "1:a:0",
			"-c:a", "aac",
			"-b:a", "128k",
			"-shortest",
		)
	} else {
		args = append(args, "-an")
	}
	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		"-f", container,
		outputPath,
	)

	command := exec.CommandContext(ctx, ffmpegPath, args...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create integration video: %v: %s", err, output)
	}
}

func assertJPEGFile(t *testing.T, path string) {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open(%q) error = %v", path, err)
	}
	defer file.Close()

	buffer := make([]byte, 512)
	count, err := file.Read(buffer)
	if err != nil {
		t.Fatalf("file.Read() error = %v", err)
	}
	if got := http.DetectContentType(buffer[:count]); got != "image/jpeg" {
		t.Fatalf("thumbnail MIME = %q, want image/jpeg", got)
	}
}
