-- name: CreateArticle :exec
INSERT INTO articles (
    id, title, content, summary,
    source_type, source_url, author, published_at, language,
    created_at, updated_at, status,
    source_id, parent_id, sections, custom_metadata
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetArticleByID :one
SELECT id, title, content, summary,
    source_type, source_url, author, published_at, language,
    created_at, updated_at, status,
    source_id, parent_id, sections, custom_metadata
FROM articles WHERE id = ?;

-- name: UpdateArticle :exec
UPDATE articles SET
    title = ?, content = ?, summary = ?,
    source_type = ?, source_url = ?, author = ?, published_at = ?, language = ?,
    updated_at = ?, status = ?,
    source_id = ?, parent_id = ?, sections = ?, custom_metadata = ?
WHERE id = ?;

-- name: DeleteArticle :exec
DELETE FROM articles WHERE id = ?;

-- name: ListArticlesAll :many
SELECT id, title, content, summary,
    source_type, source_url, author, published_at, language,
    created_at, updated_at, status,
    source_id, parent_id, sections, custom_metadata
FROM articles
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: ListArticlesByStatus :many
SELECT id, title, content, summary,
    source_type, source_url, author, published_at, language,
    created_at, updated_at, status,
    source_id, parent_id, sections, custom_metadata
FROM articles
WHERE status = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: ListArticlesBySourceType :many
SELECT id, title, content, summary,
    source_type, source_url, author, published_at, language,
    created_at, updated_at, status,
    source_id, parent_id, sections, custom_metadata
FROM articles
WHERE source_type = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: ListArticlesByStatusAndSourceType :many
SELECT id, title, content, summary,
    source_type, source_url, author, published_at, language,
    created_at, updated_at, status,
    source_id, parent_id, sections, custom_metadata
FROM articles
WHERE status = ? AND source_type = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: SearchArticlesLike :many
SELECT id, title, content, summary,
    source_type, source_url, author, published_at, language,
    created_at, updated_at, status,
    source_id, parent_id, sections, custom_metadata
FROM articles
WHERE title LIKE ? OR content LIKE ?
ORDER BY created_at DESC
LIMIT ?;

-- name: CountArticles :one
SELECT COUNT(*) FROM articles;

-- name: InsertArticleFTS :exec
INSERT INTO articles_fts (article_id, title, content, summary)
VALUES (?, ?, ?, ?);

-- name: DeleteArticleFTS :exec
DELETE FROM articles_fts WHERE article_id = ?;

-- name: GetArticleTags :many
SELECT t.id, t.name, t.color, t.created_at
FROM tags t
JOIN article_tags at ON t.id = at.tag_id
WHERE at.article_id = ?;

-- name: AddArticleTag :exec
INSERT OR IGNORE INTO article_tags (article_id, tag_id) VALUES (?, ?);

-- name: RemoveArticleTag :exec
DELETE FROM article_tags WHERE article_id = ? AND tag_id = ?;

-- name: GetArticlesBySourceID :many
SELECT id, title, content, summary,
    source_type, source_url, author, published_at, language,
    created_at, updated_at, status,
    source_id, parent_id, sections, custom_metadata
FROM articles WHERE source_id = ?;

-- name: DeleteArticlesBySourceID :exec
DELETE FROM articles WHERE source_id = ?;
