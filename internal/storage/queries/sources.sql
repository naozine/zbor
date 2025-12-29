-- name: CreateSource :exec
INSERT INTO sources (id, type, original_url, file_path, metadata, created_at, status)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetSourceByID :one
SELECT id, type, original_url, file_path, metadata, created_at, status
FROM sources WHERE id = ?;

-- name: UpdateSourceStatus :exec
UPDATE sources SET status = ? WHERE id = ?;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = ?;

-- name: ListSources :many
SELECT id, type, original_url, file_path, metadata, created_at, status
FROM sources
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: CreateArtifact :exec
INSERT INTO processing_artifacts (id, source_id, type, content, format, file_path, metadata, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetArtifactByID :one
SELECT id, source_id, type, content, format, file_path, metadata, created_at
FROM processing_artifacts WHERE id = ?;

-- name: GetArtifactsBySourceID :many
SELECT id, source_id, type, content, format, file_path, metadata, created_at
FROM processing_artifacts
WHERE source_id = ?
ORDER BY created_at;

-- name: DeleteArtifact :exec
DELETE FROM processing_artifacts WHERE id = ?;

-- name: DeleteArtifactsBySourceID :exec
DELETE FROM processing_artifacts WHERE source_id = ?;

-- name: UpdateArtifactContent :exec
UPDATE processing_artifacts SET content = ? WHERE id = ?;
