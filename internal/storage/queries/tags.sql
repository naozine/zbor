-- name: CreateTag :one
INSERT INTO tags (name, color, created_at)
VALUES (?, ?, ?)
RETURNING id, name, color, created_at;

-- name: GetTagByID :one
SELECT id, name, color, created_at FROM tags WHERE id = ?;

-- name: GetTagByName :one
SELECT id, name, color, created_at FROM tags WHERE name = ?;

-- name: UpdateTag :exec
UPDATE tags SET name = ?, color = ? WHERE id = ?;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = ?;

-- name: DeleteArticleTagsByTagID :exec
DELETE FROM article_tags WHERE tag_id = ?;

-- name: ListTags :many
SELECT id, name, color, created_at FROM tags ORDER BY name;

-- name: ListTagsWithCount :many
SELECT t.id, t.name, t.color, t.created_at, COUNT(at.article_id) as count
FROM tags t
LEFT JOIN article_tags at ON t.id = at.tag_id
GROUP BY t.id
ORDER BY count DESC, t.name;
