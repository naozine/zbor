package handlers

import (
	"net/http"
	"strconv"

	"zbor/internal/storage"
	"zbor/web/components"

	"github.com/labstack/echo/v4"
)

// JobHandler はジョブAPIのハンドラー
type JobHandler struct {
	repo *storage.JobRepository
}

// NewJobHandler は新しいJobHandlerを作成
func NewJobHandler(repo *storage.JobRepository) *JobHandler {
	return &JobHandler{repo: repo}
}

// List はジョブ一覧を取得
func (h *JobHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	status := c.QueryParam("status")

	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	var jobs interface{}
	var err error

	if status != "" {
		jobs, err = h.repo.ListByStatus(ctx, status, limit)
	} else {
		jobs, err = h.repo.ListRecent(ctx, limit)
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, jobs)
}

// Get はジョブを取得
func (h *JobHandler) Get(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	job, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if job == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "job not found"})
	}

	return c.JSON(http.StatusOK, job)
}

// Stats はジョブ統計を取得
func (h *JobHandler) Stats(c echo.Context) error {
	ctx := c.Request().Context()

	counts, err := h.repo.CountByStatus(ctx)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	stats := make(map[string]int64)
	for _, row := range counts {
		if row.Status != nil {
			stats[*row.Status] = row.Count
		}
	}

	return c.JSON(http.StatusOK, stats)
}

// Delete はジョブを削除
func (h *JobHandler) Delete(c echo.Context) error {
	ctx := c.Request().Context()
	id := c.Param("id")

	job, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if job == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "job not found"})
	}

	if err := h.repo.Delete(ctx, id); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.NoContent(http.StatusNoContent)
}

// ListPage はジョブ一覧ページを表示
func (h *JobHandler) ListPage(c echo.Context) error {
	ctx := c.Request().Context()
	jobs, err := h.repo.ListRecent(ctx, 50)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	return render(c, components.JobList(jobs))
}
