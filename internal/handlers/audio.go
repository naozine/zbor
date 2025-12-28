package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"zbor/internal/asr"
	"zbor/internal/ingestion"
	"zbor/internal/storage"
	"zbor/web/components"

	"github.com/labstack/echo/v4"
)

// AudioHandler handles audio-related HTTP requests
type AudioHandler struct {
	ingester     *ingestion.AudioIngester
	sourceRepo   *storage.SourceRepository
	artifactRepo *storage.ArtifactRepository
}

// NewAudioHandler creates a new AudioHandler
func NewAudioHandler(
	ingester *ingestion.AudioIngester,
	sourceRepo *storage.SourceRepository,
	artifactRepo *storage.ArtifactRepository,
) *AudioHandler {
	return &AudioHandler{
		ingester:     ingester,
		sourceRepo:   sourceRepo,
		artifactRepo: artifactRepo,
	}
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

// Stream serves audio file with Range request support
// GET /api/audio/:source_id/stream
func (h *AudioHandler) Stream(c echo.Context) error {
	ctx := c.Request().Context()
	sourceID := c.Param("source_id")

	// Get source
	source, err := h.sourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if source == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "source not found"})
	}

	// Get metadata to find file path
	if source.Metadata == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no metadata"})
	}

	var metadata struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal([]byte(*source.Metadata), &metadata); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to parse metadata"})
	}

	if len(metadata.Files) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no audio files"})
	}

	// Use first file (or convert to WAV if needed)
	audioPath := metadata.Files[0]

	// Check if WAV version exists
	wavPath := audioPath
	ext := filepath.Ext(audioPath)
	if ext != ".wav" {
		// Look for converted WAV file
		wavPath = audioPath[:len(audioPath)-len(ext)] + "_converted.wav"
		if _, err := os.Stat(wavPath); os.IsNotExist(err) {
			// Convert on demand
			if err := asr.ConvertToWav(audioPath, wavPath); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to convert audio"})
			}
		}
	}

	// Serve file with Range support (Echo handles this automatically)
	return c.File(wavPath)
}

// Transcript returns the transcription artifact for a source
// GET /api/audio/:source_id/transcript
func (h *AudioHandler) Transcript(c echo.Context) error {
	ctx := c.Request().Context()
	sourceID := c.Param("source_id")

	// Get artifacts for source
	artifacts, err := h.artifactRepo.GetBySourceID(ctx, sourceID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	// Find transcription artifact
	for _, artifact := range artifacts {
		if artifact.Type == storage.ArtifactTypeTranscription {
			if artifact.Content == nil {
				continue
			}
			// Parse and return JSON content
			var result asr.Result
			if err := json.Unmarshal([]byte(*artifact.Content), &result); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to parse transcript"})
			}
			return c.JSON(http.StatusOK, result)
		}
	}

	return c.JSON(http.StatusNotFound, map[string]string{"error": "transcript not found"})
}

// TranscriptSyncPage renders the transcript sync page
// GET /audio/:source_id/sync
func (h *AudioHandler) TranscriptSyncPage(c echo.Context) error {
	ctx := c.Request().Context()
	sourceID := c.Param("source_id")

	// Get source
	source, err := h.sourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}
	if source == nil {
		return c.String(http.StatusNotFound, "Source not found")
	}

	// Get transcript
	artifacts, err := h.artifactRepo.GetBySourceID(ctx, sourceID)
	if err != nil {
		return c.String(http.StatusInternalServerError, err.Error())
	}

	var transcript *asr.Result
	for _, artifact := range artifacts {
		if artifact.Type == storage.ArtifactTypeTranscription && artifact.Content != nil {
			var result asr.Result
			if err := json.Unmarshal([]byte(*artifact.Content), &result); err == nil {
				transcript = &result
				break
			}
		}
	}

	if transcript == nil {
		return c.String(http.StatusNotFound, "Transcript not found")
	}

	// Get title from metadata
	title := "Audio Transcript"
	if source.Metadata != nil {
		var metadata struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(*source.Metadata), &metadata); err == nil && metadata.Title != "" {
			title = metadata.Title
		}
	}

	return render(c, components.TranscriptSync(sourceID, title, transcript))
}
