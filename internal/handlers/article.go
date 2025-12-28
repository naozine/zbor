package handlers

import (
	"net/http"
	"strconv"

	"zbor/internal/storage"
	"zbor/internal/storage/sqlc"

	"github.com/labstack/echo/v4"
)

// ArticleHandler は記事APIのハンドラー
type ArticleHandler struct {
	repo *storage.ArticleRepository
}

// NewArticleHandler は新しいArticleHandlerを作成
func NewArticleHandler(repo *storage.ArticleRepository) *ArticleHandler {
	return &ArticleHandler{repo: repo}
}

// List は記事一覧を取得
func (h *ArticleHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	opts := storage.ListOptions{
		Limit:      20,
		Status:     c.QueryParam("status"),
		SourceType: c.QueryParam("source_type"),
	}

	if limit := c.QueryParam("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil {
			opts.Limit = l
		}
	}
	if offset := c.QueryParam("offset"); offset != "" {
		if o, err := strconv.Atoi(offset); err == nil {
			opts.Offset = o
		}
	}

	articles, err := h.repo.List(ctx, opts)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, articles)
}

// Get は記事を取得
func (h *ArticleHandler) Get(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	article, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if article == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "article not found"})
	}

	// タグを取得して追加
	tags, err := h.repo.GetArticleTags(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// レスポンス用の構造体
	type ArticleResponse struct {
		sqlc.Article
		Tags []sqlc.Tag `json:"tags,omitempty"`
	}

	return c.JSON(http.StatusOK, ArticleResponse{
		Article: *article,
		Tags:    tags,
	})
}

// CreateRequest は記事作成リクエスト
type CreateRequest struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Summary    string `json:"summary,omitempty"`
	SourceType string `json:"source_type,omitempty"`
	SourceURL  string `json:"source_url,omitempty"`
	Author     string `json:"author,omitempty"`
	Language   string `json:"language,omitempty"`
}

// Create は記事を作成
func (h *ArticleHandler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	var req CreateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Title == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "title is required"})
	}

	article := &sqlc.Article{
		Title:   req.Title,
		Content: req.Content,
	}

	if req.Summary != "" {
		article.Summary = &req.Summary
	}
	if req.SourceType != "" {
		article.SourceType = &req.SourceType
	}
	if req.SourceURL != "" {
		article.SourceUrl = &req.SourceURL
	}
	if req.Author != "" {
		article.Author = &req.Author
	}
	if req.Language != "" {
		article.Language = &req.Language
	}

	if err := h.repo.Create(ctx, article); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, article)
}

// UpdateRequest は記事更新リクエスト
type UpdateRequest struct {
	Title      *string `json:"title,omitempty"`
	Content    *string `json:"content,omitempty"`
	Summary    *string `json:"summary,omitempty"`
	SourceType *string `json:"source_type,omitempty"`
	SourceURL  *string `json:"source_url,omitempty"`
	Author     *string `json:"author,omitempty"`
	Language   *string `json:"language,omitempty"`
	Status     *string `json:"status,omitempty"`
}

// Update は記事を更新
func (h *ArticleHandler) Update(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	article, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if article == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "article not found"})
	}

	var req UpdateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// 部分更新
	if req.Title != nil {
		article.Title = *req.Title
	}
	if req.Content != nil {
		article.Content = *req.Content
	}
	if req.Summary != nil {
		article.Summary = req.Summary
	}
	if req.SourceType != nil {
		article.SourceType = req.SourceType
	}
	if req.SourceURL != nil {
		article.SourceUrl = req.SourceURL
	}
	if req.Author != nil {
		article.Author = req.Author
	}
	if req.Language != nil {
		article.Language = req.Language
	}
	if req.Status != nil {
		article.Status = req.Status
	}

	if err := h.repo.Update(ctx, article); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, article)
}

// Delete は記事を削除
func (h *ArticleHandler) Delete(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	article, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if article == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "article not found"})
	}

	if err := h.repo.Delete(ctx, id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// Search は記事を検索
func (h *ArticleHandler) Search(c echo.Context) error {
	ctx := c.Request().Context()
	query := c.QueryParam("q")
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required"})
	}

	limit := 20
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	articles, err := h.repo.Search(ctx, query, limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, articles)
}

// AddTag は記事にタグを追加
func (h *ArticleHandler) AddTag(c echo.Context) error {
	ctx := c.Request().Context()
	articleID := c.Param("id")
	tagIDStr := c.Param("tag_id")

	tagID, err := strconv.ParseInt(tagIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid tag_id"})
	}

	if err := h.repo.AddTag(ctx, articleID, tagID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// RemoveTag は記事からタグを削除
func (h *ArticleHandler) RemoveTag(c echo.Context) error {
	ctx := c.Request().Context()
	articleID := c.Param("id")
	tagIDStr := c.Param("tag_id")

	tagID, err := strconv.ParseInt(tagIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid tag_id"})
	}

	if err := h.repo.RemoveTag(ctx, articleID, tagID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
