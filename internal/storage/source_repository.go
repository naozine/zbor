package storage

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"zbor/internal/storage/sqlc"
)

// SourceRepository はソースのデータアクセス層
type SourceRepository struct {
	db *DB
}

// NewSourceRepository は新しいSourceRepositoryを作成
func NewSourceRepository(db *DB) *SourceRepository {
	return &SourceRepository{db: db}
}

// Create は新しいソースを作成
func (r *SourceRepository) Create(ctx context.Context, source *sqlc.Source) error {
	if source.ID == "" {
		source.ID = uuid.New().String()
	}
	source.CreatedAt = time.Now()
	if source.Status == nil {
		status := "pending"
		source.Status = &status
	}

	return r.db.Queries.CreateSource(ctx, sqlc.CreateSourceParams{
		ID:          source.ID,
		Type:        source.Type,
		OriginalUrl: source.OriginalUrl,
		FilePath:    source.FilePath,
		Metadata:    source.Metadata,
		CreatedAt:   source.CreatedAt,
		Status:      source.Status,
	})
}

// GetByID はIDでソースを取得
func (r *SourceRepository) GetByID(ctx context.Context, id string) (*sqlc.Source, error) {
	source, err := r.db.Queries.GetSourceByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &source, nil
}

// UpdateStatus はソースのステータスを更新
func (r *SourceRepository) UpdateStatus(ctx context.Context, id, status string) error {
	return r.db.Queries.UpdateSourceStatus(ctx, sqlc.UpdateSourceStatusParams{
		Status: &status,
		ID:     id,
	})
}

// Delete はソースを削除
func (r *SourceRepository) Delete(ctx context.Context, id string) error {
	return r.db.Queries.DeleteSource(ctx, id)
}

// List はソース一覧を取得
func (r *SourceRepository) List(ctx context.Context, limit, offset int) ([]sqlc.Source, error) {
	if limit == 0 {
		limit = 20
	}
	return r.db.Queries.ListSources(ctx, sqlc.ListSourcesParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
}

// ArtifactRepository はアーティファクトのデータアクセス層
type ArtifactRepository struct {
	db *DB
}

// NewArtifactRepository は新しいArtifactRepositoryを作成
func NewArtifactRepository(db *DB) *ArtifactRepository {
	return &ArtifactRepository{db: db}
}

// Create は新しいアーティファクトを作成
func (r *ArtifactRepository) Create(ctx context.Context, artifact *sqlc.ProcessingArtifact) error {
	if artifact.ID == "" {
		artifact.ID = uuid.New().String()
	}
	artifact.CreatedAt = time.Now()

	return r.db.Queries.CreateArtifact(ctx, sqlc.CreateArtifactParams{
		ID:        artifact.ID,
		SourceID:  artifact.SourceID,
		Type:      artifact.Type,
		Content:   artifact.Content,
		Format:    artifact.Format,
		FilePath:  artifact.FilePath,
		Metadata:  artifact.Metadata,
		CreatedAt: artifact.CreatedAt,
	})
}

// GetByID はIDでアーティファクトを取得
func (r *ArtifactRepository) GetByID(ctx context.Context, id string) (*sqlc.ProcessingArtifact, error) {
	artifact, err := r.db.Queries.GetArtifactByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &artifact, nil
}

// GetBySourceID はソースIDでアーティファクト一覧を取得
func (r *ArtifactRepository) GetBySourceID(ctx context.Context, sourceID string) ([]sqlc.ProcessingArtifact, error) {
	return r.db.Queries.GetArtifactsBySourceID(ctx, &sourceID)
}

// Delete はアーティファクトを削除
func (r *ArtifactRepository) Delete(ctx context.Context, id string) error {
	return r.db.Queries.DeleteArtifact(ctx, id)
}

// DeleteBySourceID はソースIDでアーティファクトを削除
func (r *ArtifactRepository) DeleteBySourceID(ctx context.Context, sourceID string) error {
	return r.db.Queries.DeleteArtifactsBySourceID(ctx, &sourceID)
}

// ソースステータス定数
const (
	SourceStatusPending    = "pending"
	SourceStatusProcessing = "processing"
	SourceStatusCompleted  = "completed"
	SourceStatusFailed     = "failed"
)

// アーティファクトタイプ定数
const (
	ArtifactTypeTranscription = "transcription"
	ArtifactTypeSummary       = "summary"
	ArtifactTypeTranslation   = "translation"
)

// Ptr はstring型のポインタを返すヘルパー
func Ptr[T any](v T) *T {
	return &v
}
