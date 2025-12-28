package storage

import (
	"context"
	"database/sql"
	"time"

	"zbor/internal/storage/sqlc"
)

// TagRepository はタグのデータアクセス層
type TagRepository struct {
	db *DB
}

// NewTagRepository は新しいTagRepositoryを作成
func NewTagRepository(db *DB) *TagRepository {
	return &TagRepository{db: db}
}

// Create は新しいタグを作成
func (r *TagRepository) Create(ctx context.Context, tag *sqlc.Tag) error {
	created, err := r.db.Queries.CreateTag(ctx, sqlc.CreateTagParams{
		Name:      tag.Name,
		Color:     tag.Color,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return err
	}
	tag.ID = created.ID
	tag.CreatedAt = created.CreatedAt
	return nil
}

// GetByID はIDでタグを取得
func (r *TagRepository) GetByID(ctx context.Context, id int64) (*sqlc.Tag, error) {
	tag, err := r.db.Queries.GetTagByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tag, nil
}

// GetByName は名前でタグを取得
func (r *TagRepository) GetByName(ctx context.Context, name string) (*sqlc.Tag, error) {
	tag, err := r.db.Queries.GetTagByName(ctx, name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tag, nil
}

// GetOrCreate は名前でタグを取得し、なければ作成
func (r *TagRepository) GetOrCreate(ctx context.Context, name string) (*sqlc.Tag, error) {
	tag, err := r.GetByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if tag != nil {
		return tag, nil
	}

	tag = &sqlc.Tag{Name: name}
	err = r.Create(ctx, tag)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

// Update はタグを更新
func (r *TagRepository) Update(ctx context.Context, tag *sqlc.Tag) error {
	return r.db.Queries.UpdateTag(ctx, sqlc.UpdateTagParams{
		Name:  tag.Name,
		Color: tag.Color,
		ID:    tag.ID,
	})
}

// Delete はタグを削除
func (r *TagRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := r.db.Queries.WithTx(tx)

	// 関連するarticle_tagsを削除
	err = qtx.DeleteArticleTagsByTagID(ctx, &id)
	if err != nil {
		return err
	}

	err = qtx.DeleteTag(ctx, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// List はタグ一覧を取得
func (r *TagRepository) List(ctx context.Context) ([]sqlc.Tag, error) {
	return r.db.Queries.ListTags(ctx)
}

// ListWithCount は記事数付きでタグ一覧を取得
func (r *TagRepository) ListWithCount(ctx context.Context) ([]sqlc.ListTagsWithCountRow, error) {
	return r.db.Queries.ListTagsWithCount(ctx)
}
