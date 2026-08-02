package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"coffee-reel/entity"
)

type IMediaRepository interface {
	Probe(ctx context.Context, filePath string) (entity.SourceVideoMeta, error)
	Transcode(ctx context.Context, inputPath, outputPath string, hasAudio bool) error
	GenerateThumbnail(ctx context.Context, inputPath, outputPath string) error
	ProbeOutput(ctx context.Context, filePath string) (entity.OutputVideoMeta, error)
}
type MediaError struct {
	Code      entity.VideoFailureCode
	Retryable bool
	Message   string
}

func (e *MediaError) Error() string {
	return e.Message
}

type mediaRepository struct {
	ffprobePath string
	ffmpegPath  string
}

func NewMediaRepository(ffprobePath, ffmpegPath string) (IMediaRepository, error) {
	ffprobePath = strings.TrimSpace(ffprobePath)
	ffmpegPath = strings.TrimSpace(ffmpegPath)
	if ffprobePath == "" || ffmpegPath == "" {
		return nil, fmt.Errorf("ffprobe and ffmpeg paths are required")
	}

	return &mediaRepository{
		ffprobePath: ffprobePath,
		ffmpegPath:  ffmpegPath,
	}, nil
}

type ffprobeOutput struct {
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
		Tags       struct {
			MajorBrand string `json:"major_brand"`
		} `json:"tags"`
	} `json:"format"`
	Streams []ffprobeStream `json:"streams"`
}

type ffprobeStream struct {
	CodecType    string            `json:"codec_type"`
	CodecName    string            `json:"codec_name"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	AvgFrameRate string            `json:"avg_frame_rate"`
	Tags         ffprobeStreamTags `json:"tags"`
	SideDataList []ffprobeSideData `json:"side_data_list"`
}

type ffprobeStreamTags struct {
	Rotate string `json:"rotate"`
}

type ffprobeSideData struct {
	Rotation int `json:"rotation"`
}

func (r *mediaRepository) Probe(ctx context.Context, filePath string) (entity.SourceVideoMeta, error) {
	if strings.TrimSpace(filePath) == "" {
		return entity.SourceVideoMeta{}, entity.ErrInvalidInput
	}

	detectedMIME, err := detectMIME(filePath)
	if err != nil {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureCorrupt,
			Retryable: false,
			Message:   "video file could not be read",
		}
	}
	if detectedMIME != "video/mp4" && detectedMIME != "video/quicktime" {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureInvalidFormat,
			Retryable: false,
			Message:   "video format is not supported",
		}
	}

	probe, err := r.runProbe(ctx, filePath)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return entity.SourceVideoMeta{}, &MediaError{
				Code:      entity.VideoFailureProcessingFailed,
				Retryable: true,
				Message:   "media probe timed out",
			}
		}
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureCorrupt,
			Retryable: false,
			Message:   "video could not be analyzed",
		}
	}

	return sourceMetaFromProbe(probe, detectedMIME)
}

func (r *mediaRepository) Transcode(ctx context.Context, inputPath, outputPath string, hasAudio bool) error {
	if strings.TrimSpace(inputPath) == "" || strings.TrimSpace(outputPath) == "" || inputPath == outputPath {
		return entity.ErrInvalidInput
	}

	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", inputPath,
		"-map", "0:v:0",
		"-vf", "scale=720:1280:flags=lanczos,fps=30",
		"-c:v", "libx264",
		"-preset", "medium",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
	}

	if hasAudio {
		args = append(args,
			"-map", "0:a:0?",
			"-c:a", "aac",
			"-b:a", "128k",
		)
	} else {
		args = append(args, "-an")
	}

	args = append(args, "-f", "mp4", outputPath)

	if err := runCommand(ctx, r.ffmpegPath, args...); err != nil {
		return &MediaError{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: true,
			Message:   "video transcoding failed",
		}
	}

	return nil
}

func (r *mediaRepository) GenerateThumbnail(ctx context.Context, inputPath, outputPath string) error {
	if strings.TrimSpace(inputPath) == "" || strings.TrimSpace(outputPath) == "" || inputPath == outputPath {
		return entity.ErrInvalidInput
	}

	args := []string{
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

	if err := runCommand(ctx, r.ffmpegPath, args...); err != nil {
		return &MediaError{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: true,
			Message:   "thumbnail generation failed",
		}
	}

	return nil
}

func (r *mediaRepository) ProbeOutput(ctx context.Context, filePath string) (entity.OutputVideoMeta, error) {
	if strings.TrimSpace(filePath) == "" {
		return entity.OutputVideoMeta{}, entity.ErrInvalidInput
	}

	detectedMIME, err := detectMIME(filePath)
	if err != nil {
		return entity.OutputVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: false,
			Message:   "output video could not be read",
		}
	}
	if detectedMIME != "video/mp4" {
		return entity.OutputVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: false,
			Message:   "output video format is invalid",
		}
	}

	probe, err := r.runProbe(ctx, filePath)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return entity.OutputVideoMeta{}, &MediaError{
				Code:      entity.VideoFailureProcessingFailed,
				Retryable: true,
				Message:   "output media probe timed out",
			}
		}
		return entity.OutputVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: false,
			Message:   "output video could not be analyzed",
		}
	}

	container, normalizedMIME, ok := normalizeISOBaseMediaFormat(
		detectedMIME,
		probe.Format.FormatName,
		probe.Format.Tags.MajorBrand,
	)
	if !ok || container != "mp4" || normalizedMIME != "video/mp4" {
		return entity.OutputVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: false,
			Message:   "output video container is invalid",
		}
	}

	video, audio, ok := selectTracks(probe.Streams)
	if !ok {
		return entity.OutputVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureVideoTrackMissing,
			Retryable: false,
			Message:   "output video track is missing",
		}
	}

	frameRate, err := parseFrameRate(video.AvgFrameRate)
	if err != nil {
		return entity.OutputVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: false,
			Message:   "output frame rate is invalid",
		}
	}

	width, height := applyRotation(
		video.Width,
		video.Height,
		rotationOf(video),
	)

	meta := entity.OutputVideoMeta{
		Container:  container,
		Width:      width,
		Height:     height,
		FrameRate:  frameRate,
		VideoCodec: strings.ToLower(strings.TrimSpace(video.CodecName)),
		HasAudio:   audio != nil,
	}

	if audio != nil {
		meta.AudioCodec = strings.ToLower(strings.TrimSpace(audio.CodecName))
	}

	if meta.Width != 720 ||
		meta.Height != 1280 ||
		meta.FrameRate > 30 ||
		meta.VideoCodec != "h264" ||
		(meta.HasAudio && meta.AudioCodec != "aac") ||
		(!meta.HasAudio && meta.AudioCodec != "") {
		return entity.OutputVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureProcessingFailed,
			Retryable: false,
			Message:   "output video does not meet publishing requirements",
		}
	}

	return meta, nil
}

func (r *mediaRepository) runProbe(ctx context.Context, filePath string) (ffprobeOutput, error) {
	command := exec.CommandContext(
		ctx,
		r.ffprobePath,
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		filePath,
	)

	var stdout bytes.Buffer
	command.Stdout = &stdout

	var stderr bytes.Buffer
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ffprobeOutput{}, ctx.Err()
		}
		return ffprobeOutput{}, fmt.Errorf("run ffprobe: %w", err)
	}

	var output ffprobeOutput
	if err := json.NewDecoder(&stdout).Decode(&output); err != nil {
		return ffprobeOutput{}, fmt.Errorf("decode ffprobe output: %w", err)
	}

	return output, nil
}

func sourceMetaFromProbe(probe ffprobeOutput, detectedMIME string) (entity.SourceVideoMeta, error) {
	container, mimeType, ok := normalizeISOBaseMediaFormat(
		detectedMIME,
		probe.Format.FormatName,
		probe.Format.Tags.MajorBrand,
	)
	if !ok {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureInvalidFormat,
			Retryable: false,
			Message:   "video container could not be verified",
		}
	}

	video, audio, ok := selectTracks(probe.Streams)
	if !ok {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureVideoTrackMissing,
			Retryable: false,
			Message:   "video track is missing",
		}
	}

	width, height := applyRotation(
		video.Width,
		video.Height,
		rotationOf(video),
	)
	if width < 1 || height < 1 {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureCorrupt,
			Retryable: false,
			Message:   "video resolution is invalid",
		}
	}

	frameRate, err := parseFrameRate(video.AvgFrameRate)
	if err != nil {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureCorrupt,
			Retryable: false,
			Message:   "video frame rate is invalid",
		}
	}

	durationSeconds, err := strconv.ParseFloat(
		probe.Format.Duration,
		64,
	)
	if err != nil ||
		durationSeconds <= 0 ||
		math.IsNaN(durationSeconds) ||
		math.IsInf(durationSeconds, 0) {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureCorrupt,
			Retryable: false,
			Message:   "video duration is invalid",
		}
	}

	sizeBytes, err := strconv.ParseInt(
		probe.Format.Size,
		10,
		64,
	)
	if err != nil || sizeBytes <= 0 {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureCorrupt,
			Retryable: false,
			Message:   "video size is invalid",
		}
	}

	meta := entity.SourceVideoMeta{
		MIMEType:       mimeType,
		Container:      container,
		SizeBytes:      sizeBytes,
		DurationMillis: int64(math.Ceil(durationSeconds * 1000)),
		Width:          width,
		Height:         height,
		FrameRate:      frameRate,
		VideoCodec:     strings.ToLower(strings.TrimSpace(video.CodecName)),
		HasAudio:       audio != nil,
	}

	if audio != nil {
		meta.AudioCodec = strings.ToLower(strings.TrimSpace(audio.CodecName))
	}

	if meta.SizeBytes > 30_000_000 {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureSizeExceeded,
			Retryable: false,
			Message:   "video size exceeds limit",
		}
	}

	if meta.DurationMillis > 10_000 {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureDurationExceeded,
			Retryable: false,
			Message:   "video duration exceeds limit",
		}
	}

	if meta.Width > 1080 || meta.Height > 1920 {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureResolutionExceeded,
			Retryable: false,
			Message:   "video resolution exceeds limit",
		}
	}

	if meta.Width*16 != meta.Height*9 {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureInvalidAspectRatio,
			Retryable: false,
			Message:   "video aspect ratio must be 9:16",
		}
	}

	if meta.FrameRate > 60 {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureFrameRateExceeded,
			Retryable: false,
			Message:   "video frame rate exceeds limit",
		}
	}

	if err := meta.Validate(); err != nil {
		return entity.SourceVideoMeta{}, &MediaError{
			Code:      entity.VideoFailureInvalidFormat,
			Retryable: false,
			Message:   "video format is invalid",
		}
	}

	return meta, nil
}

func normalizeISOBaseMediaFormat(detectedMIME string, formatName string, majorBrand string) (string, string, bool) {
	if !containsFormatName(formatName, "mov") &&
		!containsFormatName(formatName, "mp4") {
		return "", "", false
	}

	majorBrand = strings.ToLower(strings.TrimSpace(majorBrand))
	if majorBrand == "" {
		return "", "", false
	}

	switch detectedMIME {
	case "video/quicktime":
		if majorBrand != "qt" {
			return "", "", false
		}
		return "mov", "video/quicktime", true

	case "video/mp4":
		if majorBrand == "qt" {
			return "", "", false
		}
		return "mp4", "video/mp4", true
	}

	return "", "", false
}

func containsFormatName(value, target string) bool {
	for _, part := range strings.Split(strings.ToLower(value), ",") {
		if strings.TrimSpace(part) == target {
			return true
		}
	}

	return false
}

func selectTracks(streams []ffprobeStream) (*ffprobeStream, *ffprobeStream, bool) {
	var video *ffprobeStream
	var audio *ffprobeStream

	for i := range streams {
		stream := &streams[i]

		if stream.CodecType == "video" && video == nil {
			video = stream
		}

		if stream.CodecType == "audio" && audio == nil {
			audio = stream
		}
	}

	return video, audio, video != nil
}

func rotationOf(stream *ffprobeStream) int {
	if stream == nil {
		return 0
	}

	if stream.Tags.Rotate != "" {
		if value, err := strconv.Atoi(stream.Tags.Rotate); err == nil {
			return value
		}
	}

	for _, sideData := range stream.SideDataList {
		if sideData.Rotation != 0 {
			return sideData.Rotation
		}
	}

	return 0
}

func applyRotation(width int, height int, rotation int) (int, int) {
	rotation %= 360
	if rotation < 0 {
		rotation += 360
	}

	if rotation == 90 || rotation == 270 {
		return height, width
	}

	return width, height
}

func parseFrameRate(value string) (float64, error) {
	parts := strings.Split(
		strings.TrimSpace(value),
		"/",
	)

	var frameRate float64
	var err error

	switch len(parts) {
	case 1:
		frameRate, err = strconv.ParseFloat(parts[0], 64)

	case 2:
		var numerator float64
		numerator, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}

		var denominator float64
		denominator, err = strconv.ParseFloat(parts[1], 64)
		if err != nil || denominator == 0 {
			return 0, fmt.Errorf("invalid frame rate")
		}

		frameRate = numerator / denominator

	default:
		return 0, fmt.Errorf("invalid frame rate")
	}

	if err != nil ||
		frameRate <= 0 ||
		math.IsNaN(frameRate) ||
		math.IsInf(frameRate, 0) {
		return 0, fmt.Errorf("invalid frame rate")
	}

	return frameRate, nil
}

func detectMIME(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 512)

	count, err := file.Read(buffer)
	if err != nil && count == 0 {
		return "", err
	}
	if count == 0 {
		return "", fmt.Errorf("media file is empty")
	}

	content := buffer[:count]

	if len(content) >= 12 &&
		string(content[4:8]) == "ftyp" &&
		strings.EqualFold(
			strings.TrimSpace(string(content[8:12])),
			"qt",
		) {
		return "video/quicktime", nil
	}

	contentType := http.DetectContentType(content)

	return strings.TrimSpace(
		strings.SplitN(contentType, ";", 2)[0],
	), nil
}

func runCommand(ctx context.Context, executable string, args ...string) error {
	command := exec.CommandContext(ctx, executable, args...)

	var stderr bytes.Buffer
	command.Stderr = &stderr

	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}

	return nil
}
