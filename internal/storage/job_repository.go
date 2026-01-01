package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"zbor/internal/storage/sqlc"
)

// JobRepository はジョブのデータアクセス層
type JobRepository struct {
	db *DB
}

// NewJobRepository は新しいJobRepositoryを作成
func NewJobRepository(db *DB) *JobRepository {
	return &JobRepository{db: db}
}

// Create は新しいジョブを作成
func (r *JobRepository) Create(ctx context.Context, job *sqlc.ProcessingJob) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	job.CreatedAt = time.Now()
	if job.Status == nil {
		status := JobStatusQueued
		job.Status = &status
	}
	if job.Priority == nil {
		priority := int64(JobPriorityNormal)
		job.Priority = &priority
	}

	return r.db.Queries.CreateJob(ctx, sqlc.CreateJobParams{
		ID:          job.ID,
		SourceID:    job.SourceID,
		Type:        job.Type,
		Status:      job.Status,
		Priority:    job.Priority,
		Progress:    job.Progress,
		CurrentStep: job.CurrentStep,
		RetryCount:  job.RetryCount,
		Error:       job.Error,
		CreatedAt:   job.CreatedAt,
		StartedAt:   job.StartedAt,
		CompletedAt: job.CompletedAt,
	})
}

// GetByID はIDでジョブを取得
func (r *JobRepository) GetByID(ctx context.Context, id string) (*sqlc.ProcessingJob, error) {
	job, err := r.db.Queries.GetJobByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// GetNextQueued は次に処理すべきキュー済みジョブを取得（優先度順）
func (r *JobRepository) GetNextQueued(ctx context.Context) (*sqlc.ProcessingJob, error) {
	job, err := r.db.Queries.GetNextQueuedJob(ctx)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// Start はジョブを開始状態にする
func (r *JobRepository) Start(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.Queries.StartJob(ctx, sqlc.StartJobParams{
		StartedAt: &now,
		ID:        id,
	})
}

// UpdateProgress はジョブの進捗を更新
func (r *JobRepository) UpdateProgress(ctx context.Context, id string, progress int64) error {
	return r.db.Queries.UpdateJobProgress(ctx, sqlc.UpdateJobProgressParams{
		Progress: &progress,
		ID:       id,
	})
}

// UpdateProgressWithStep はジョブの進捗とステップを更新
func (r *JobRepository) UpdateProgressWithStep(ctx context.Context, id string, progress int64, step string) error {
	return r.db.Queries.UpdateJobProgressWithStep(ctx, sqlc.UpdateJobProgressWithStepParams{
		Progress:    &progress,
		CurrentStep: &step,
		ID:          id,
	})
}

// Complete はジョブを完了状態にする
func (r *JobRepository) Complete(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.Queries.CompleteJob(ctx, sqlc.CompleteJobParams{
		CompletedAt: &now,
		ID:          id,
	})
}

// Fail はジョブを失敗状態にする
func (r *JobRepository) Fail(ctx context.Context, id string, errorMsg string) error {
	now := time.Now()
	return r.db.Queries.FailJob(ctx, sqlc.FailJobParams{
		Error:       &errorMsg,
		CompletedAt: &now,
		ID:          id,
	})
}

// Retry はジョブを再試行キューに戻す
func (r *JobRepository) Retry(ctx context.Context, id string) error {
	return r.db.Queries.RetryJob(ctx, id)
}

// GetBySourceID はソースIDでジョブ一覧を取得
func (r *JobRepository) GetBySourceID(ctx context.Context, sourceID string) ([]sqlc.ProcessingJob, error) {
	return r.db.Queries.GetJobsBySourceID(ctx, &sourceID)
}

// ListByStatus はステータスでジョブ一覧を取得
func (r *JobRepository) ListByStatus(ctx context.Context, status string, limit int) ([]sqlc.ProcessingJob, error) {
	if limit == 0 {
		limit = 50
	}
	return r.db.Queries.ListJobsByStatus(ctx, sqlc.ListJobsByStatusParams{
		Status: &status,
		Limit:  int64(limit),
	})
}

// ListRecent は最近のジョブ一覧を取得
func (r *JobRepository) ListRecent(ctx context.Context, limit int) ([]sqlc.ProcessingJob, error) {
	if limit == 0 {
		limit = 50
	}
	return r.db.Queries.ListRecentJobs(ctx, int64(limit))
}

// Delete はジョブを削除
func (r *JobRepository) Delete(ctx context.Context, id string) error {
	return r.db.Queries.DeleteJob(ctx, id)
}

// CleanupCompleted は完了済みジョブを削除（指定日数より古いもの）
func (r *JobRepository) CleanupCompleted(ctx context.Context, olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	return r.db.Queries.CleanupCompletedJobs(ctx, &cutoff)
}

// CountByStatus はステータスごとのジョブ数を取得
func (r *JobRepository) CountByStatus(ctx context.Context) ([]sqlc.CountJobsByStatusRow, error) {
	return r.db.Queries.CountJobsByStatus(ctx)
}

// ジョブタイプ
const (
	JobTypeTranscribe = "transcribe" // Default (ReazonSpeech with overlap)

	// Model-specific transcription types
	JobTypeTranscribeReazonSpeech   = "transcribe:reazonspeech"
	JobTypeTranscribeSenseVoice     = "transcribe:sensevoice"
	JobTypeTranscribeSenseVoiceBeam = "transcribe:sensevoice:beam" // SenseVoice with beam search

	JobTypeFetch     = "fetch"
	JobTypeSummarize = "summarize"
	JobTypeDownload  = "download"
)

// ASR Model types
const (
	ASRModelReazonSpeech   = "reazonspeech"
	ASRModelSenseVoice     = "sensevoice"
	ASRModelSenseVoiceBeam = "sensevoice:beam" // SenseVoice with beam search
	ASRModelWhisper        = "whisper"         // Whisper (no timestamps)
	ASRModelWhisperAlign   = "whisper:align"   // Whisper with LCS-based timestamp alignment
)

// ジョブステータス
const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
)

// ジョブ優先度
const (
	JobPriorityImmediate = 0 // 即時処理
	JobPriorityNormal    = 5 // 通常処理
	JobPriorityBatch     = 9 // バッチ処理
)
