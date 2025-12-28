package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"zbor/internal/storage/sqlc"
)

// ArticleRepository は記事のデータアクセス層
type ArticleRepository struct {
	db *DB
}

// NewArticleRepository は新しいArticleRepositoryを作成
func NewArticleRepository(db *DB) *ArticleRepository {
	return &ArticleRepository{db: db}
}

// Create は新しい記事を作成
func (r *ArticleRepository) Create(ctx context.Context, article *sqlc.Article) error {
	if article.ID == "" {
		article.ID = uuid.New().String()
	}
	now := time.Now()
	article.CreatedAt = now
	article.UpdatedAt = now
	if article.Status == nil {
		status := "draft"
		article.Status = &status
	}
	if article.Language == nil {
		lang := "ja"
		article.Language = &lang
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := r.db.Queries.WithTx(tx)

	// 記事を挿入
	err = qtx.CreateArticle(ctx, sqlc.CreateArticleParams{
		ID:             article.ID,
		Title:          article.Title,
		Content:        article.Content,
		Summary:        article.Summary,
		SourceType:     article.SourceType,
		SourceUrl:      article.SourceUrl,
		Author:         article.Author,
		PublishedAt:    article.PublishedAt,
		Language:       article.Language,
		CreatedAt:      article.CreatedAt,
		UpdatedAt:      article.UpdatedAt,
		Status:         article.Status,
		SourceID:       article.SourceID,
		ParentID:       article.ParentID,
		Sections:       article.Sections,
		CustomMetadata: article.CustomMetadata,
	})
	if err != nil {
		return fmt.Errorf("failed to insert article: %w", err)
	}

	// FTSインデックスを更新
	summary := ""
	if article.Summary != nil {
		summary = *article.Summary
	}
	err = qtx.InsertArticleFTS(ctx, sqlc.InsertArticleFTSParams{
		ArticleID: article.ID,
		Title:     article.Title,
		Content:   article.Content,
		Summary:   summary,
	})
	if err != nil {
		return fmt.Errorf("failed to insert FTS: %w", err)
	}

	return tx.Commit()
}

// GetByID はIDで記事を取得
func (r *ArticleRepository) GetByID(ctx context.Context, id string) (*sqlc.Article, error) {
	article, err := r.db.Queries.GetArticleByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &article, nil
}

// Update は記事を更新
func (r *ArticleRepository) Update(ctx context.Context, article *sqlc.Article) error {
	article.UpdatedAt = time.Now()

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := r.db.Queries.WithTx(tx)

	err = qtx.UpdateArticle(ctx, sqlc.UpdateArticleParams{
		Title:          article.Title,
		Content:        article.Content,
		Summary:        article.Summary,
		SourceType:     article.SourceType,
		SourceUrl:      article.SourceUrl,
		Author:         article.Author,
		PublishedAt:    article.PublishedAt,
		Language:       article.Language,
		UpdatedAt:      article.UpdatedAt,
		Status:         article.Status,
		SourceID:       article.SourceID,
		ParentID:       article.ParentID,
		Sections:       article.Sections,
		CustomMetadata: article.CustomMetadata,
		ID:             article.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to update article: %w", err)
	}

	// FTSインデックスを更新
	err = qtx.DeleteArticleFTS(ctx, article.ID)
	if err != nil {
		return err
	}

	summary := ""
	if article.Summary != nil {
		summary = *article.Summary
	}
	err = qtx.InsertArticleFTS(ctx, sqlc.InsertArticleFTSParams{
		ArticleID: article.ID,
		Title:     article.Title,
		Content:   article.Content,
		Summary:   summary,
	})
	if err != nil {
		return err
	}

	return tx.Commit()
}

// Delete は記事を削除
func (r *ArticleRepository) Delete(ctx context.Context, id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	qtx := r.db.Queries.WithTx(tx)

	err = qtx.DeleteArticleFTS(ctx, id)
	if err != nil {
		return err
	}

	err = qtx.DeleteArticle(ctx, id)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ListOptions はリスト取得のオプション
type ListOptions struct {
	Limit      int
	Offset     int
	Status     string
	SourceType string
}

// List は記事一覧を取得
func (r *ArticleRepository) List(ctx context.Context, opts ListOptions) ([]sqlc.Article, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}

	// フィルタ条件に応じて適切なクエリを選択
	if opts.Status != "" && opts.SourceType != "" {
		return r.db.Queries.ListArticlesByStatusAndSourceType(ctx, sqlc.ListArticlesByStatusAndSourceTypeParams{
			Status:     &opts.Status,
			SourceType: &opts.SourceType,
			Limit:      int64(opts.Limit),
			Offset:     int64(opts.Offset),
		})
	}
	if opts.Status != "" {
		return r.db.Queries.ListArticlesByStatus(ctx, sqlc.ListArticlesByStatusParams{
			Status: &opts.Status,
			Limit:  int64(opts.Limit),
			Offset: int64(opts.Offset),
		})
	}
	if opts.SourceType != "" {
		return r.db.Queries.ListArticlesBySourceType(ctx, sqlc.ListArticlesBySourceTypeParams{
			SourceType: &opts.SourceType,
			Limit:      int64(opts.Limit),
			Offset:     int64(opts.Offset),
		})
	}
	return r.db.Queries.ListArticlesAll(ctx, sqlc.ListArticlesAllParams{
		Limit:  int64(opts.Limit),
		Offset: int64(opts.Offset),
	})
}

// Search は記事を検索
func (r *ArticleRepository) Search(ctx context.Context, query string, limit int) ([]sqlc.Article, error) {
	if limit == 0 {
		limit = 20
	}

	// 3文字未満はLIKEで検索
	if utf8.RuneCountInString(query) < 3 {
		pattern := "%" + query + "%"
		return r.db.Queries.SearchArticlesLike(ctx, sqlc.SearchArticlesLikeParams{
			Title:   pattern,
			Content: pattern,
			Limit:   int64(limit),
		})
	}

	// FTS5で検索（sqlcではなく手動で実行）
	rows, err := r.db.Query(`
		SELECT a.id, a.title, a.content, a.summary,
			a.source_type, a.source_url, a.author, a.published_at, a.language,
			a.created_at, a.updated_at, a.status,
			a.source_id, a.parent_id, a.sections, a.custom_metadata
		FROM articles a
		JOIN articles_fts f ON a.id = f.article_id
		WHERE articles_fts MATCH ?
		ORDER BY rank
		LIMIT ?`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []sqlc.Article
	for rows.Next() {
		var a sqlc.Article
		err := rows.Scan(
			&a.ID, &a.Title, &a.Content, &a.Summary,
			&a.SourceType, &a.SourceUrl, &a.Author, &a.PublishedAt, &a.Language,
			&a.CreatedAt, &a.UpdatedAt, &a.Status,
			&a.SourceID, &a.ParentID, &a.Sections, &a.CustomMetadata,
		)
		if err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}

	return articles, rows.Err()
}

// GetArticleTags は記事のタグを取得
func (r *ArticleRepository) GetArticleTags(ctx context.Context, articleID string) ([]sqlc.Tag, error) {
	return r.db.Queries.GetArticleTags(ctx, &articleID)
}

// AddTag は記事にタグを追加
func (r *ArticleRepository) AddTag(ctx context.Context, articleID string, tagID int64) error {
	return r.db.Queries.AddArticleTag(ctx, sqlc.AddArticleTagParams{
		ArticleID: &articleID,
		TagID:     &tagID,
	})
}

// RemoveTag は記事からタグを削除
func (r *ArticleRepository) RemoveTag(ctx context.Context, articleID string, tagID int64) error {
	return r.db.Queries.RemoveArticleTag(ctx, sqlc.RemoveArticleTagParams{
		ArticleID: &articleID,
		TagID:     &tagID,
	})
}

// Count は記事数を取得
func (r *ArticleRepository) Count(ctx context.Context) (int64, error) {
	return r.db.Queries.CountArticles(ctx)
}
