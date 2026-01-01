package handlers

import (
	"net/http"
	"strconv"

	"zbor/internal/storage"
	"zbor/internal/storage/sqlc"

	"github.com/labstack/echo/v4"
)

// TagHandler はタグAPIのハンドラー
type TagHandler struct {
	repo *storage.TagRepository
}

// NewTagHandler は新しいTagHandlerを作成
func NewTagHandler(repo *storage.TagRepository) *TagHandler {
	return &TagHandler{repo: repo}
}

// List はタグ一覧を取得
func (h *TagHandler) List(c echo.Context) error {
	ctx := c.Request().Context()

	if c.QueryParam("with_count") == "true" {
		tags, err := h.repo.ListWithCount(ctx)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusOK, tags)
	}

	tags, err := h.repo.List(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, tags)
}

// Get はタグを取得
func (h *TagHandler) Get(c echo.Context) error {
	ctx := c.Request().Context()
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	tag, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if tag == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tag not found"})
	}

	return c.JSON(http.StatusOK, tag)
}

// CreateTagRequest はタグ作成リクエスト
type CreateTagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// Create はタグを作成
func (h *TagHandler) Create(c echo.Context) error {
	ctx := c.Request().Context()
	var req CreateTagRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name is required"})
	}

	tag := &sqlc.Tag{
		Name: req.Name,
	}
	if req.Color != "" {
		tag.Color = &req.Color
	}

	if err := h.repo.Create(ctx, tag); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusCreated, tag)
}

// Update はタグを更新
func (h *TagHandler) Update(c echo.Context) error {
	ctx := c.Request().Context()
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	tag, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if tag == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tag not found"})
	}

	var req CreateTagRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Name != "" {
		tag.Name = req.Name
	}
	if req.Color != "" {
		tag.Color = &req.Color
	}

	if err := h.repo.Update(ctx, tag); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, tag)
}

// Delete はタグを削除
func (h *TagHandler) Delete(c echo.Context) error {
	ctx := c.Request().Context()
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	tag, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if tag == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "tag not found"})
	}

	if err := h.repo.Delete(ctx, id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}
