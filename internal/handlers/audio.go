package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

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
	articleRepo  *storage.ArticleRepository
	jobRepo      *storage.JobRepository
	asrConfig    *asr.Config
}

// NewAudioHandler creates a new AudioHandler
func NewAudioHandler(
	ingester *ingestion.AudioIngester,
	sourceRepo *storage.SourceRepository,
	artifactRepo *storage.ArtifactRepository,
	articleRepo *storage.ArticleRepository,
	jobRepo *storage.JobRepository,
	asrConfig *asr.Config,
) *AudioHandler {
	return &AudioHandler{
		ingester:     ingester,
		sourceRepo:   sourceRepo,
		artifactRepo: artifactRepo,
		articleRepo:  articleRepo,
		jobRepo:      jobRepo,
		asrConfig:    asrConfig,
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

	// Parse interval parameter (default 10 seconds)
	intervalSec := 10.0
	if intervalStr := c.QueryParam("interval"); intervalStr != "" {
		if v, err := strconv.ParseFloat(intervalStr, 64); err == nil && v > 0 {
			intervalSec = v
		}
	}

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

	// Get title and duration from metadata
	title := "Audio Transcript"
	var totalDuration float64
	if source.Metadata != nil {
		var metadata struct {
			Title    string  `json:"title"`
			Duration float64 `json:"duration"`
		}
		if err := json.Unmarshal([]byte(*source.Metadata), &metadata); err == nil {
			if metadata.Title != "" {
				title = metadata.Title
			}
			totalDuration = metadata.Duration
		}
	}

	// Estimate duration from tokens if not available
	if totalDuration <= 0 && len(transcript.Tokens) > 0 {
		lastToken := transcript.Tokens[len(transcript.Tokens)-1]
		totalDuration = float64(lastToken.StartTime + lastToken.Duration + 1.0)
	}

	// Generate display segments for timeline view
	displaySegments := asr.GenerateDisplaySegments(
		transcript.Tokens,
		transcript.Segments,
		totalDuration,
		intervalSec,
		0.3,  // silenceThreshold
		5.0,  // dotsPerSecond
	)

	return render(c, components.TranscriptSync(sourceID, title, transcript, displaySegments))
}

// RetranscribeRequest represents the request body for partial re-transcription
type RetranscribeRequest struct {
	SegmentStart int     `json:"segment_start"` // Start segment index (0-based)
	SegmentEnd   int     `json:"segment_end"`   // End segment index (inclusive)
	Tempo        float64 `json:"tempo"`         // Audio tempo (0.85-1.0)
}

// Retranscribe handles partial re-transcription of audio segments
// POST /api/audio/:source_id/retranscribe
func (h *AudioHandler) Retranscribe(c echo.Context) error {
	ctx := c.Request().Context()
	sourceID := c.Param("source_id")

	// Parse request body
	var req RetranscribeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Validate tempo
	if req.Tempo <= 0 || req.Tempo > 1.0 {
		req.Tempo = 0.95
	}
	if req.Tempo < 0.5 {
		req.Tempo = 0.5
	}

	// Get source
	source, err := h.sourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	if source == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "source not found"})
	}

	// Get audio file path from metadata
	var metadata struct {
		Files []string `json:"files"`
	}
	if source.Metadata == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no metadata"})
	}
	if err := json.Unmarshal([]byte(*source.Metadata), &metadata); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to parse metadata"})
	}
	if len(metadata.Files) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no audio files"})
	}
	audioPath := metadata.Files[0]

	// Get existing transcript
	artifacts, err := h.artifactRepo.GetBySourceID(ctx, sourceID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	var transcript *asr.Result
	var artifactID string
	for _, artifact := range artifacts {
		if artifact.Type == storage.ArtifactTypeTranscription && artifact.Content != nil {
			var result asr.Result
			if err := json.Unmarshal([]byte(*artifact.Content), &result); err == nil {
				transcript = &result
				artifactID = artifact.ID
				break
			}
		}
	}

	if transcript == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "transcript not found"})
	}

	// Validate segment indices
	if len(transcript.Segments) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no segments in transcript"})
	}
	if req.SegmentStart < 0 || req.SegmentStart >= len(transcript.Segments) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid segment_start"})
	}
	if req.SegmentEnd < req.SegmentStart || req.SegmentEnd >= len(transcript.Segments) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid segment_end"})
	}

	// Get time range from segments
	startTime := transcript.Segments[req.SegmentStart].StartTime
	endTime := transcript.Segments[req.SegmentEnd].EndTime

	// Create recognizer
	recognizer, err := asr.NewRecognizer(h.asrConfig)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create recognizer"})
	}
	defer recognizer.Close()

	// Perform partial transcription
	partialResult, err := recognizer.TranscribePartial(audioPath, asr.PartialTranscribeOptions{
		StartTime: startTime,
		EndTime:   endTime,
		Tempo:     req.Tempo,
		ChunkSec:  20,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "transcription failed: " + err.Error()})
	}

	// Merge tokens
	mergedTokens := asr.MergeTokens(transcript.Tokens, partialResult.Tokens, startTime, endTime)

	// Merge segments
	mergedSegments := asr.MergeSegments(transcript.Segments, req.SegmentStart, req.SegmentEnd, partialResult.Tokens)

	// Rebuild text
	mergedText := asr.RebuildTextFromTokens(mergedTokens)

	// Create updated result
	updatedResult := &asr.Result{
		Text:          mergedText,
		Tokens:        mergedTokens,
		Segments:      mergedSegments,
		TotalDuration: transcript.TotalDuration,
		Duration:      transcript.Duration,
		Speaker:       transcript.Speaker,
	}

	// Update artifact
	artifactContent, _ := json.Marshal(updatedResult)
	if err := h.artifactRepo.UpdateContent(ctx, artifactID, string(artifactContent)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save transcript"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":        "Retranscription completed",
		"segments_range": []int{req.SegmentStart, req.SegmentEnd},
		"time_range":     []float64{startTime, endTime},
		"tempo":          req.Tempo,
		"new_tokens":     len(partialResult.Tokens),
	})
}

// RetranscribeFull handles full re-transcription of audio
// Deletes existing artifacts and articles, then creates a new transcription job
// POST /api/audio/:source_id/retranscribe-full
func (h *AudioHandler) RetranscribeFull(c echo.Context) error {
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

	// Delete existing artifacts by source_id
	if err := h.artifactRepo.DeleteBySourceID(ctx, sourceID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete artifacts: " + err.Error()})
	}

	// Delete existing articles by source_id (includes FTS)
	if err := h.articleRepo.DeleteBySourceID(ctx, sourceID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete articles: " + err.Error()})
	}

	// Create new transcription job via ingester
	jobID, err := h.ingester.CreateTranscriptionJob(ctx, sourceID, storage.JobPriorityImmediate)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create job: " + err.Error()})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"message":   "Retranscription job created",
		"source_id": sourceID,
		"job_id":    jobID,
	})
}
