package usecase

import (
	"context"
	"time"

	"coffee-reel/entity"
	"coffee-reel/repository"
)

type videoRepositoryMock struct {
	createWithIdempotencyFunc           func(context.Context, *entity.Video, *entity.IdempotencyRecord) (repository.VideoCreateResult, error)
	completeUploadFunc                  func(context.Context, uint64, uint64, time.Time) (*entity.Video, error)
	findPublicByIDFunc                  func(context.Context, uint64, *uint64) (*repository.PublicVideoItem, error)
	listPublicFunc                      func(context.Context, int, *repository.VideoCursor, *uint64) (repository.PublicVideoPage, error)
	findOwnedByIDFunc                   func(context.Context, uint64, uint64) (*repository.OwnedVideoDetail, error)
	listOwnedFunc                       func(context.Context, uint64, int, *repository.VideoCursor) (repository.OwnedVideoPage, error)
	setPrivateByOwnerFunc               func(context.Context, uint64, uint64, time.Time) (*entity.Video, error)
	republishByOwnerFunc                func(context.Context, uint64, uint64, time.Time) (*entity.Video, error)
	deleteByOwnerFunc                   func(context.Context, uint64, uint64, time.Time) error
	expireUploadsFunc                   func(context.Context, time.Time, int) (int, error)
	recordSourceValidationFunc          func(context.Context, uint64, entity.SourceVideoMeta, time.Time) error
	completeProcessingFunc              func(context.Context, repository.ProcessingCompletionInput) error
	failProcessingFunc                  func(context.Context, repository.ProcessingFailureInput) error
	isObjectReferencedFunc              func(context.Context, string) (bool, error)
	deleteExpiredIdempotencyRecordsFunc func(context.Context, time.Time, int) (int64, error)
}

func (m *videoRepositoryMock) CreateWithIdempotency(ctx context.Context, video *entity.Video, record *entity.IdempotencyRecord) (repository.VideoCreateResult, error) {
	if m.createWithIdempotencyFunc == nil {
		panic("unexpected VideoRepository.CreateWithIdempotency call")
	}
	return m.createWithIdempotencyFunc(ctx, video, record)
}
func (m *videoRepositoryMock) CompleteUpload(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error) {
	if m.completeUploadFunc == nil {
		panic("unexpected VideoRepository.CompleteUpload call")
	}
	return m.completeUploadFunc(ctx, userID, videoID, now)
}
func (m *videoRepositoryMock) FindPublicByID(ctx context.Context, videoID uint64, viewerUserID *uint64) (*repository.PublicVideoItem, error) {
	if m.findPublicByIDFunc == nil {
		panic("unexpected VideoRepository.FindPublicByID call")
	}
	return m.findPublicByIDFunc(ctx, videoID, viewerUserID)
}
func (m *videoRepositoryMock) ListPublic(ctx context.Context, limit int, cursor *repository.VideoCursor, viewerUserID *uint64) (repository.PublicVideoPage, error) {
	if m.listPublicFunc == nil {
		panic("unexpected VideoRepository.ListPublic call")
	}
	return m.listPublicFunc(ctx, limit, cursor, viewerUserID)
}
func (m *videoRepositoryMock) FindOwnedByID(ctx context.Context, userID, videoID uint64) (*repository.OwnedVideoDetail, error) {
	if m.findOwnedByIDFunc == nil {
		panic("unexpected VideoRepository.FindOwnedByID call")
	}
	return m.findOwnedByIDFunc(ctx, userID, videoID)
}
func (m *videoRepositoryMock) ListOwned(ctx context.Context, userID uint64, limit int, cursor *repository.VideoCursor) (repository.OwnedVideoPage, error) {
	if m.listOwnedFunc == nil {
		panic("unexpected VideoRepository.ListOwned call")
	}
	return m.listOwnedFunc(ctx, userID, limit, cursor)
}
func (m *videoRepositoryMock) SetPrivateByOwner(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error) {
	if m.setPrivateByOwnerFunc == nil {
		panic("unexpected VideoRepository.SetPrivateByOwner call")
	}
	return m.setPrivateByOwnerFunc(ctx, userID, videoID, now)
}
func (m *videoRepositoryMock) RepublishByOwner(ctx context.Context, userID, videoID uint64, now time.Time) (*entity.Video, error) {
	if m.republishByOwnerFunc == nil {
		panic("unexpected VideoRepository.RepublishByOwner call")
	}
	return m.republishByOwnerFunc(ctx, userID, videoID, now)
}
func (m *videoRepositoryMock) DeleteByOwner(ctx context.Context, userID, videoID uint64, now time.Time) error {
	if m.deleteByOwnerFunc == nil {
		panic("unexpected VideoRepository.DeleteByOwner call")
	}
	return m.deleteByOwnerFunc(ctx, userID, videoID, now)
}
func (m *videoRepositoryMock) ExpireUploads(ctx context.Context, now time.Time, limit int) (int, error) {
	if m.expireUploadsFunc == nil {
		panic("unexpected VideoRepository.ExpireUploads call")
	}
	return m.expireUploadsFunc(ctx, now, limit)
}
func (m *videoRepositoryMock) RecordSourceValidation(ctx context.Context, videoID uint64, meta entity.SourceVideoMeta, now time.Time) error {
	if m.recordSourceValidationFunc == nil {
		panic("unexpected VideoRepository.RecordSourceValidation call")
	}
	return m.recordSourceValidationFunc(ctx, videoID, meta, now)
}
func (m *videoRepositoryMock) CompleteProcessing(ctx context.Context, input repository.ProcessingCompletionInput) error {
	if m.completeProcessingFunc == nil {
		panic("unexpected VideoRepository.CompleteProcessing call")
	}
	return m.completeProcessingFunc(ctx, input)
}
func (m *videoRepositoryMock) FailProcessing(ctx context.Context, input repository.ProcessingFailureInput) error {
	if m.failProcessingFunc == nil {
		panic("unexpected VideoRepository.FailProcessing call")
	}
	return m.failProcessingFunc(ctx, input)
}
func (m *videoRepositoryMock) IsObjectReferenced(ctx context.Context, objectKey string) (bool, error) {
	if m.isObjectReferencedFunc == nil {
		panic("unexpected VideoRepository.IsObjectReferenced call")
	}
	return m.isObjectReferencedFunc(ctx, objectKey)
}
func (m *videoRepositoryMock) DeleteExpiredIdempotencyRecords(ctx context.Context, now time.Time, limit int) (int64, error) {
	if m.deleteExpiredIdempotencyRecordsFunc == nil {
		panic("unexpected VideoRepository.DeleteExpiredIdempotencyRecords call")
	}
	return m.deleteExpiredIdempotencyRecordsFunc(ctx, now, limit)
}

type videoProcessingJobRepositoryMock struct {
	claimNextFunc       func(context.Context, time.Time) (*repository.ProcessingClaim, error)
	scheduleRetryFunc   func(context.Context, uint64, time.Time, string, string, time.Time) error
	cancelFunc          func(context.Context, uint64, time.Time) error
	recoverTimedOutFunc func(context.Context, repository.ProcessingRecoveryInput) (int, error)
}

func (m *videoProcessingJobRepositoryMock) ClaimNext(ctx context.Context, now time.Time) (*repository.ProcessingClaim, error) {
	if m.claimNextFunc == nil {
		panic("unexpected VideoProcessingJobRepository.ClaimNext call")
	}
	return m.claimNextFunc(ctx, now)
}
func (m *videoProcessingJobRepositoryMock) ScheduleRetry(ctx context.Context, jobID uint64, availableAt time.Time, code, message string, now time.Time) error {
	if m.scheduleRetryFunc == nil {
		panic("unexpected VideoProcessingJobRepository.ScheduleRetry call")
	}
	return m.scheduleRetryFunc(ctx, jobID, availableAt, code, message, now)
}
func (m *videoProcessingJobRepositoryMock) Cancel(ctx context.Context, jobID uint64, now time.Time) error {
	if m.cancelFunc == nil {
		panic("unexpected VideoProcessingJobRepository.Cancel call")
	}
	return m.cancelFunc(ctx, jobID, now)
}
func (m *videoProcessingJobRepositoryMock) RecoverTimedOut(ctx context.Context, input repository.ProcessingRecoveryInput) (int, error) {
	if m.recoverTimedOutFunc == nil {
		panic("unexpected VideoProcessingJobRepository.RecoverTimedOut call")
	}
	return m.recoverTimedOutFunc(ctx, input)
}

type savedVideoRepositoryMock struct {
	saveFunc       func(context.Context, uint64, uint64, time.Time) error
	removeFunc     func(context.Context, uint64, uint64) error
	listByUserFunc func(context.Context, uint64, int, *repository.SavedVideoCursor) (repository.SavedVideoPage, error)
	existsFunc     func(context.Context, uint64, uint64) (bool, error)
}

func (m *savedVideoRepositoryMock) Save(ctx context.Context, userID, videoID uint64, now time.Time) error {
	if m.saveFunc == nil {
		panic("unexpected SavedVideoRepository.Save call")
	}
	return m.saveFunc(ctx, userID, videoID, now)
}
func (m *savedVideoRepositoryMock) Remove(ctx context.Context, userID, videoID uint64) error {
	if m.removeFunc == nil {
		panic("unexpected SavedVideoRepository.Remove call")
	}
	return m.removeFunc(ctx, userID, videoID)
}
func (m *savedVideoRepositoryMock) ListByUser(ctx context.Context, userID uint64, limit int, cursor *repository.SavedVideoCursor) (repository.SavedVideoPage, error) {
	if m.listByUserFunc == nil {
		panic("unexpected SavedVideoRepository.ListByUser call")
	}
	return m.listByUserFunc(ctx, userID, limit, cursor)
}
func (m *savedVideoRepositoryMock) Exists(ctx context.Context, userID, videoID uint64) (bool, error) {
	if m.existsFunc == nil {
		panic("unexpected SavedVideoRepository.Exists call")
	}
	return m.existsFunc(ctx, userID, videoID)
}

type storageCleanupJobRepositoryMock struct {
	createFunc          func(context.Context, *entity.StorageCleanupJob) error
	claimNextFunc       func(context.Context, time.Time) (*entity.StorageCleanupJob, error)
	scheduleRetryFunc   func(context.Context, uint64, time.Time, string, string, time.Time) error
	markSucceededFunc   func(context.Context, uint64, time.Time) error
	markFailedFunc      func(context.Context, uint64, string, string, time.Time) error
	recoverTimedOutFunc func(context.Context, repository.StorageCleanupRecoveryInput) (int, error)
}

func (m *storageCleanupJobRepositoryMock) Create(ctx context.Context, job *entity.StorageCleanupJob) error {
	if m.createFunc == nil {
		panic("unexpected StorageCleanupJobRepository.Create call")
	}
	return m.createFunc(ctx, job)
}
func (m *storageCleanupJobRepositoryMock) ClaimNext(ctx context.Context, now time.Time) (*entity.StorageCleanupJob, error) {
	if m.claimNextFunc == nil {
		panic("unexpected StorageCleanupJobRepository.ClaimNext call")
	}
	return m.claimNextFunc(ctx, now)
}
func (m *storageCleanupJobRepositoryMock) ScheduleRetry(ctx context.Context, jobID uint64, availableAt time.Time, code, message string, now time.Time) error {
	if m.scheduleRetryFunc == nil {
		panic("unexpected StorageCleanupJobRepository.ScheduleRetry call")
	}
	return m.scheduleRetryFunc(ctx, jobID, availableAt, code, message, now)
}
func (m *storageCleanupJobRepositoryMock) MarkSucceeded(ctx context.Context, jobID uint64, now time.Time) error {
	if m.markSucceededFunc == nil {
		panic("unexpected StorageCleanupJobRepository.MarkSucceeded call")
	}
	return m.markSucceededFunc(ctx, jobID, now)
}
func (m *storageCleanupJobRepositoryMock) MarkFailed(ctx context.Context, jobID uint64, code, message string, now time.Time) error {
	if m.markFailedFunc == nil {
		panic("unexpected StorageCleanupJobRepository.MarkFailed call")
	}
	return m.markFailedFunc(ctx, jobID, code, message, now)
}
func (m *storageCleanupJobRepositoryMock) RecoverTimedOut(ctx context.Context, input repository.StorageCleanupRecoveryInput) (int, error) {
	if m.recoverTimedOutFunc == nil {
		panic("unexpected StorageCleanupJobRepository.RecoverTimedOut call")
	}
	return m.recoverTimedOutFunc(ctx, input)
}

type objectStorageRepositoryMock struct {
	createUploadURLFunc    func(context.Context, string, string, time.Duration) (repository.UploadTarget, error)
	existsFunc             func(context.Context, string) (bool, error)
	statFunc               func(context.Context, string) (repository.StoredObjectInfo, error)
	downloadFunc           func(context.Context, string, string) error
	uploadProcessedFunc    func(context.Context, string, string) error
	uploadThumbnailFunc    func(context.Context, string, string) error
	createReadURLFunc      func(context.Context, string, time.Duration) (repository.ReadTarget, error)
	deleteFunc             func(context.Context, string) error
	listManagedObjectsFunc func(context.Context, *string, int32) (repository.ManagedObjectPage, error)
}

func (m *objectStorageRepositoryMock) CreateUploadURL(ctx context.Context, objectKey, contentType string, ttl time.Duration) (repository.UploadTarget, error) {
	if m.createUploadURLFunc == nil {
		panic("unexpected ObjectStorageRepository.CreateUploadURL call")
	}
	return m.createUploadURLFunc(ctx, objectKey, contentType, ttl)
}
func (m *objectStorageRepositoryMock) Exists(ctx context.Context, objectKey string) (bool, error) {
	if m.existsFunc == nil {
		panic("unexpected ObjectStorageRepository.Exists call")
	}
	return m.existsFunc(ctx, objectKey)
}
func (m *objectStorageRepositoryMock) Stat(ctx context.Context, objectKey string) (repository.StoredObjectInfo, error) {
	if m.statFunc == nil {
		panic("unexpected ObjectStorageRepository.Stat call")
	}
	return m.statFunc(ctx, objectKey)
}
func (m *objectStorageRepositoryMock) Download(ctx context.Context, objectKey, destinationPath string) error {
	if m.downloadFunc == nil {
		panic("unexpected ObjectStorageRepository.Download call")
	}
	return m.downloadFunc(ctx, objectKey, destinationPath)
}
func (m *objectStorageRepositoryMock) UploadProcessed(ctx context.Context, objectKey, sourcePath string) error {
	if m.uploadProcessedFunc == nil {
		panic("unexpected ObjectStorageRepository.UploadProcessed call")
	}
	return m.uploadProcessedFunc(ctx, objectKey, sourcePath)
}
func (m *objectStorageRepositoryMock) UploadThumbnail(ctx context.Context, objectKey, sourcePath string) error {
	if m.uploadThumbnailFunc == nil {
		panic("unexpected ObjectStorageRepository.UploadThumbnail call")
	}
	return m.uploadThumbnailFunc(ctx, objectKey, sourcePath)
}
func (m *objectStorageRepositoryMock) CreateReadURL(ctx context.Context, objectKey string, ttl time.Duration) (repository.ReadTarget, error) {
	if m.createReadURLFunc == nil {
		panic("unexpected ObjectStorageRepository.CreateReadURL call")
	}
	return m.createReadURLFunc(ctx, objectKey, ttl)
}
func (m *objectStorageRepositoryMock) Delete(ctx context.Context, objectKey string) error {
	if m.deleteFunc == nil {
		panic("unexpected ObjectStorageRepository.Delete call")
	}
	return m.deleteFunc(ctx, objectKey)
}
func (m *objectStorageRepositoryMock) ListManagedObjects(ctx context.Context, cursor *string, limit int32) (repository.ManagedObjectPage, error) {
	if m.listManagedObjectsFunc == nil {
		panic("unexpected ObjectStorageRepository.ListManagedObjects call")
	}
	return m.listManagedObjectsFunc(ctx, cursor, limit)
}

type mediaRepositoryMock struct {
	probeFunc             func(context.Context, string) (entity.SourceVideoMeta, error)
	transcodeFunc         func(context.Context, string, string, bool) error
	generateThumbnailFunc func(context.Context, string, string) error
	probeOutputFunc       func(context.Context, string) (entity.OutputVideoMeta, error)
}

func (m *mediaRepositoryMock) Probe(ctx context.Context, filePath string) (entity.SourceVideoMeta, error) {
	if m.probeFunc == nil {
		panic("unexpected MediaRepository.Probe call")
	}
	return m.probeFunc(ctx, filePath)
}
func (m *mediaRepositoryMock) Transcode(ctx context.Context, inputPath, outputPath string, hasAudio bool) error {
	if m.transcodeFunc == nil {
		panic("unexpected MediaRepository.Transcode call")
	}
	return m.transcodeFunc(ctx, inputPath, outputPath, hasAudio)
}
func (m *mediaRepositoryMock) GenerateThumbnail(ctx context.Context, inputPath, outputPath string) error {
	if m.generateThumbnailFunc == nil {
		panic("unexpected MediaRepository.GenerateThumbnail call")
	}
	return m.generateThumbnailFunc(ctx, inputPath, outputPath)
}
func (m *mediaRepositoryMock) ProbeOutput(ctx context.Context, filePath string) (entity.OutputVideoMeta, error) {
	if m.probeOutputFunc == nil {
		panic("unexpected MediaRepository.ProbeOutput call")
	}
	return m.probeOutputFunc(ctx, filePath)
}

func activeVideoUser(id uint64) *entity.User {
	return &entity.User{ID: id, Role: entity.RoleUser, Status: entity.StatusActive}
}

func suspendedVideoUser(id uint64) *entity.User {
	return &entity.User{ID: id, Role: entity.RoleUser, Status: entity.StatusSuspended}
}
