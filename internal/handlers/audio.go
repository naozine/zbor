package handlers

import (
	"net/http"

	"zbor/internal/ingestion"
	"zbor/web/components"

	"github.com/labstack/echo/v4"
)

// AudioHandler handles audio-related HTTP requests
type AudioHandler struct {
	ingester *ingestion.AudioIngester
}

// NewAudioHandler creates a new AudioHandler
func NewAudioHandler(ingester *ingestion.AudioIngester) *AudioHandler {
	return &AudioHandler{ingester: ingester}
}

// Upload handles audio file upload
// POST /api/ingest/audio
func (h *AudioHandler) Upload(c echo.Context) error {
	ctx := c.Request().Context()

	// Get title from form
	title := c.FormValue("title")

	// Get uploaded files
	form, err := c.MultipartForm()
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "failed to parse form"})
	}

	files := form.File["files"]
	if len(files) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no files uploaded"})
	}

	// Build AudioFile slice
	var audioFiles []ingestion.AudioFile
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to open file"})
		}
		defer f.Close()

		audioFiles = append(audioFiles, ingestion.AudioFile{
			Filename: fh.Filename,
			Reader:   f,
			Speaker:  "", // Will be extracted from filename
		})
	}

	// Ingest audio
	result, err := h.ingester.Ingest(ctx, ingestion.IngestOptions{
		Title:    title,
		Files:    audioFiles,
		Priority: 5, // Normal priority
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"source_id": result.SourceID,
		"job_id":    result.JobID,
		"message":   "Audio ingestion started",
	})
}

// UploadPage renders the audio upload page
func (h *AudioHandler) UploadPage(c echo.Context) error {
	return render(c, components.AudioUpload())
}
