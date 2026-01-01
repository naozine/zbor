package models

import "time"

// ProcessingJob は非同期処理タスク
type ProcessingJob struct {
	ID          string     `json:"id"`
	SourceID    string     `json:"source_id"`
	Type        string     `json:"type"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	Progress    int        `json:"progress"`
	RetryCount  int        `json:"retry_count"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ジョブタイプ
const (
	JobTypeTranscribe = "transcribe"
	JobTypeFetch      = "fetch"
	JobTypeSummarize  = "summarize"
	JobTypeDownload   = "download"
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
