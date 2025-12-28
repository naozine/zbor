-- name: CreateJob :exec
INSERT INTO processing_jobs (
    id, source_id, type, status, priority, progress,
    retry_count, error, created_at, started_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetJobByID :one
SELECT id, source_id, type, status, priority, progress,
    retry_count, error, created_at, started_at, completed_at
FROM processing_jobs WHERE id = ?;

-- name: GetNextQueuedJob :one
SELECT id, source_id, type, status, priority, progress,
    retry_count, error, created_at, started_at, completed_at
FROM processing_jobs
WHERE status = 'queued'
ORDER BY priority ASC, created_at ASC
LIMIT 1;

-- name: StartJob :exec
UPDATE processing_jobs
SET status = 'running', started_at = ?
WHERE id = ?;

-- name: UpdateJobProgress :exec
UPDATE processing_jobs SET progress = ? WHERE id = ?;

-- name: CompleteJob :exec
UPDATE processing_jobs
SET status = 'completed', progress = 100, completed_at = ?
WHERE id = ?;

-- name: FailJob :exec
UPDATE processing_jobs
SET status = 'failed', error = ?, completed_at = ?
WHERE id = ?;

-- name: RetryJob :exec
UPDATE processing_jobs
SET status = 'queued', retry_count = retry_count + 1, error = NULL
WHERE id = ?;

-- name: GetJobsBySourceID :many
SELECT id, source_id, type, status, priority, progress,
    retry_count, error, created_at, started_at, completed_at
FROM processing_jobs
WHERE source_id = ?
ORDER BY created_at DESC;

-- name: ListJobsByStatus :many
SELECT id, source_id, type, status, priority, progress,
    retry_count, error, created_at, started_at, completed_at
FROM processing_jobs
WHERE status = ?
ORDER BY priority ASC, created_at ASC
LIMIT ?;

-- name: ListRecentJobs :many
SELECT id, source_id, type, status, priority, progress,
    retry_count, error, created_at, started_at, completed_at
FROM processing_jobs
ORDER BY created_at DESC
LIMIT ?;

-- name: DeleteJob :exec
DELETE FROM processing_jobs WHERE id = ?;

-- name: CleanupCompletedJobs :execrows
DELETE FROM processing_jobs
WHERE status = 'completed' AND completed_at < ?;

-- name: CountJobsByStatus :many
SELECT status, COUNT(*) as count FROM processing_jobs GROUP BY status;
