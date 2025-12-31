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

// WaveformResponse represents the waveform data response
type WaveformResponse struct {
	Peaks    []float64 `json:"peaks"`    // Peak amplitude values (0-1)
	Duration float64   `json:"duration"` // Total duration in seconds
}

// Waveform returns waveform peak data for visualization
// GET /api/audio/:source_id/waveform?samples_per_sec=10
func (h *AudioHandler) Waveform(c echo.Context) error {
	ctx := c.Request().Context()
	sourceID := c.Param("source_id")

	// Parse samples_per_sec parameter (default 10)
	samplesPerSec := 10.0
	if sps := c.QueryParam("samples_per_sec"); sps != "" {
		if v, err := strconv.ParseFloat(sps, 64); err == nil && v > 0 && v <= 100 {
			samplesPerSec = v
		}
	}

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
		Files    []string `json:"files"`
		Duration float64  `json:"duration"`
	}
	if err := json.Unmarshal([]byte(*source.Metadata), &metadata); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to parse metadata"})
	}

	if len(metadata.Files) == 0 {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no audio files"})
	}

	audioPath := metadata.Files[0]

	// Check if WAV version exists
	wavPath := audioPath
	ext := filepath.Ext(audioPath)
	if ext != ".wav" {
		wavPath = audioPath[:len(audioPath)-len(ext)] + "_converted.wav"
		if _, err := os.Stat(wavPath); os.IsNotExist(err) {
			if err := asr.ConvertToWav(audioPath, wavPath); err != nil {
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to convert audio"})
			}
		}
	}

	// Compute waveform peaks
	peaks, duration, err := asr.ComputeWaveformPeaks(wavPath, samplesPerSec)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to compute waveform: " + err.Error()})
	}

	return c.JSON(http.StatusOK, WaveformResponse{
		Peaks:    peaks,
		Duration: duration,
	})
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
// GET /audio/:source_id/sync?interval=10&start=0&end=300&waveform=1
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

	// Parse range parameters (start/end in seconds)
	// Default: show first 5 minutes
	rangeStart := 0.0
	rangeEnd := 300.0 // 5 minutes default
	hasRangeParams := false

	if startStr := c.QueryParam("start"); startStr != "" {
		if v, err := strconv.ParseFloat(startStr, 64); err == nil && v >= 0 {
			rangeStart = v
			hasRangeParams = true
		}
	}
	if endStr := c.QueryParam("end"); endStr != "" {
		if v, err := strconv.ParseFloat(endStr, 64); err == nil && v > rangeStart {
			rangeEnd = v
			hasRangeParams = true
		}
	}

	// Parse waveform display flag
	showWaveform := c.QueryParam("waveform") == "1"

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

	// Get title, filename, and duration from metadata
	title := "Audio Transcript"
	filename := ""
	var totalDuration float64
	if source.Metadata != nil {
		var metadata struct {
			Title    string   `json:"title"`
			Files    []string `json:"files"`
			Duration float64  `json:"duration"`
		}
		if err := json.Unmarshal([]byte(*source.Metadata), &metadata); err == nil {
			if metadata.Title != "" {
				title = metadata.Title
			}
			// Extract filename from first file path
			if len(metadata.Files) > 0 {
				filename = filepath.Base(metadata.Files[0])
			}
			totalDuration = metadata.Duration
		}
	}

	// Estimate duration from tokens if not available
	if totalDuration <= 0 && len(transcript.Tokens) > 0 {
		lastToken := transcript.Tokens[len(transcript.Tokens)-1]
		totalDuration = float64(lastToken.StartTime + lastToken.Duration + 1.0)
	}

	// Adjust range based on total duration
	if !hasRangeParams {
		// If no range specified, show first 5 minutes or entire file if shorter
		if totalDuration < rangeEnd {
			rangeEnd = totalDuration
		}
	} else {
		// Clamp to total duration
		if rangeEnd > totalDuration {
			rangeEnd = totalDuration
		}
	}

	// Generate display segments for timeline view
	allDisplaySegments := asr.GenerateDisplaySegments(
		transcript.Tokens,
		transcript.Segments,
		totalDuration,
		intervalSec,
		0.3,  // silenceThreshold
		5.0,  // dotsPerSecond
	)

	// Filter display segments based on range
	var displaySegments []asr.DisplaySegment
	for _, ds := range allDisplaySegments {
		// Include segment if it overlaps with the range
		if ds.EndTime > rangeStart && ds.StartTime < rangeEnd {
			displaySegments = append(displaySegments, ds)
		}
	}

	// Build sync options for template
	syncOpts := components.TranscriptSyncOptions{
		SourceID:      sourceID,
		Title:         title,
		Filename:      filename,
		Transcript:    transcript,
		Segments:      displaySegments,
		RangeStart:    rangeStart,
		RangeEnd:      rangeEnd,
		TotalDuration: totalDuration,
		IntervalSec:   intervalSec,
		ShowWaveform:  showWaveform,
	}

	return render(c, components.TranscriptSyncWithOptions(syncOpts))
}

// RetranscribeRequest represents the request body for partial re-transcription
type RetranscribeRequest struct {
	SegmentStart int     `json:"segment_start"` // Start segment index (0-based)
	SegmentEnd   int     `json:"segment_end"`   // End segment index (inclusive)
	Tempo        float64 `json:"tempo"`         // Audio tempo (0.85-1.0)
	Model        string  `json:"model"`         // "reazonspeech", "sensevoice", or "whisper"
	Preview      bool    `json:"preview"`       // If true, return result without saving
}

// RetranscribeResponse represents the response for preview mode
type RetranscribeResponse struct {
	Success          bool                      `json:"success"`
	Message          string                    `json:"message,omitempty"`
	Error            string                    `json:"error,omitempty"`
	OriginalSegments []RetranscribeSegmentInfo `json:"original_segments,omitempty"`
	NewSegments      []RetranscribeSegmentInfo `json:"new_segments,omitempty"`
	Model            string                    `json:"model,omitempty"`
	Tempo            float64                   `json:"tempo,omitempty"`
	// Whisper Align specific fields
	WhisperRawText  string                    `json:"whisper_raw_text,omitempty"`
	AlignmentDiff   []AlignmentDiffItem       `json:"alignment_diff,omitempty"`
	OriginalText    string                    `json:"original_text,omitempty"`
}

// AlignmentDiffItem represents a single character in the alignment diff
type AlignmentDiffItem struct {
	Char string `json:"char"` // The character
	Op   string `json:"op"`   // "match", "insert", or "delete"
}

// RetranscribeSegmentInfo contains segment info for display
type RetranscribeSegmentInfo struct {
	Index     int                      `json:"index"`
	StartTime float64                  `json:"start_time"`
	EndTime   float64                  `json:"end_time"`
	Text      string                   `json:"text"`
	Tokens    []RetranscribeTokenInfo  `json:"tokens"`
}

// RetranscribeTokenInfo contains token info for display
type RetranscribeTokenInfo struct {
	Text      string  `json:"text"`
	StartTime float64 `json:"start_time"`
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

	// Default to reazonspeech if not specified
	model := req.Model
	if model == "" {
		model = storage.ASRModelReazonSpeech
	}

	// Perform partial transcription based on model
	opts := asr.PartialTranscribeOptions{
		StartTime: startTime,
		EndTime:   endTime,
		Tempo:     req.Tempo,
		ChunkSec:  20,
	}

	var partialResult *asr.Result
	switch model {
	case storage.ASRModelSenseVoice:
		svConfig := asr.DefaultSenseVoiceConfig("models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17")
		svRecognizer, err := asr.NewSenseVoiceRecognizer(svConfig)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create sensevoice recognizer: " + err.Error()})
		}
		defer svRecognizer.Close()
		partialResult, err = svRecognizer.TranscribePartial(audioPath, opts)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "transcription failed: " + err.Error()})
		}
	case storage.ASRModelWhisper, storage.ASRModelWhisperAlign:
		wConfig := asr.DefaultWhisperConfig("models/sherpa-onnx-whisper-turbo")
		wRecognizer, err := asr.NewWhisperRecognizer(wConfig)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create whisper recognizer: " + err.Error()})
		}
		defer wRecognizer.Close()
		partialResult, err = wRecognizer.TranscribePartial(audioPath, opts)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "transcription failed: " + err.Error()})
		}
	default: // reazonspeech
		recognizer, err := asr.NewRecognizer(h.asrConfig)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create recognizer: " + err.Error()})
		}
		defer recognizer.Close()
		partialResult, err = recognizer.TranscribePartial(audioPath, opts)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "transcription failed: " + err.Error()})
		}
	}

	// Merge tokens and segments based on model type
	var mergedTokens []asr.Token
	var mergedSegments []asr.Segment
	var alignResult *asr.AlignResult // For Whisper Align diff info

	switch model {
	case storage.ASRModelWhisperAlign:
		// Use LCS-based alignment to preserve original timestamps where characters match
		alignResult = asr.AlignTokensForSegmentsWithDiff(
			transcript.Tokens,
			partialResult.Text,
			transcript.Segments,
			req.SegmentStart,
			req.SegmentEnd,
		)

		// Merge aligned tokens with original tokens (outside the range)
		mergedTokens = asr.MergeTokens(transcript.Tokens, alignResult.Tokens, startTime, endTime)

		// Merge aligned segments with original segments
		mergedSegments = make([]asr.Segment, 0, len(transcript.Segments))
		for i := 0; i < req.SegmentStart && i < len(transcript.Segments); i++ {
			mergedSegments = append(mergedSegments, transcript.Segments[i])
		}
		mergedSegments = append(mergedSegments, alignResult.Segments...)
		for i := req.SegmentEnd + 1; i < len(transcript.Segments); i++ {
			mergedSegments = append(mergedSegments, transcript.Segments[i])
		}

	case storage.ASRModelWhisper:
		// Use ratio-based distribution since timestamps are uniformly distributed
		// and don't align with segment boundaries (especially when there are gaps)
		mergedTokens = asr.MergeTokensBySegmentRatio(transcript.Tokens, partialResult.Tokens, transcript.Segments, req.SegmentStart, req.SegmentEnd, startTime, endTime)
		mergedSegments = asr.MergeSegmentsByRatio(transcript.Segments, req.SegmentStart, req.SegmentEnd, partialResult.Tokens)

	default:
		// ReazonSpeech, SenseVoice: use timestamp-based merge
		mergedTokens = asr.MergeTokens(transcript.Tokens, partialResult.Tokens, startTime, endTime)
		mergedSegments = asr.MergeSegments(transcript.Segments, req.SegmentStart, req.SegmentEnd, partialResult.Tokens)
	}

	// Build original segments info for response
	originalSegments := make([]RetranscribeSegmentInfo, 0, req.SegmentEnd-req.SegmentStart+1)
	for i := req.SegmentStart; i <= req.SegmentEnd && i < len(transcript.Segments); i++ {
		seg := transcript.Segments[i]
		// Find tokens in this segment
		var segTokens []RetranscribeTokenInfo
		for _, t := range transcript.Tokens {
			if float64(t.StartTime) >= seg.StartTime && float64(t.StartTime) < seg.EndTime {
				segTokens = append(segTokens, RetranscribeTokenInfo{
					Text:      t.Text,
					StartTime: float64(t.StartTime),
				})
			}
		}
		originalSegments = append(originalSegments, RetranscribeSegmentInfo{
			Index:     i + 1, // 1-based for display
			StartTime: seg.StartTime,
			EndTime:   seg.EndTime,
			Text:      seg.Text,
			Tokens:    segTokens,
		})
	}

	// Build new segments info for response
	// Use mergedTokens which has adjusted timestamps (important for Whisper)
	newSegments := make([]RetranscribeSegmentInfo, 0, req.SegmentEnd-req.SegmentStart+1)
	for i := req.SegmentStart; i <= req.SegmentEnd && i < len(mergedSegments); i++ {
		seg := mergedSegments[i]
		// Find tokens in this segment from merged result (which has correct timestamps)
		var segTokens []RetranscribeTokenInfo
		for _, t := range mergedTokens {
			if float64(t.StartTime) >= seg.StartTime && float64(t.StartTime) < seg.EndTime {
				segTokens = append(segTokens, RetranscribeTokenInfo{
					Text:      t.Text,
					StartTime: float64(t.StartTime),
				})
			}
		}
		// Handle edge case for last segment
		if i == req.SegmentEnd {
			for _, t := range mergedTokens {
				if float64(t.StartTime) >= seg.EndTime && float64(t.StartTime) <= seg.EndTime+0.01 {
					segTokens = append(segTokens, RetranscribeTokenInfo{
						Text:      t.Text,
						StartTime: float64(t.StartTime),
					})
				}
			}
		}
		newSegments = append(newSegments, RetranscribeSegmentInfo{
			Index:     i + 1, // 1-based for display
			StartTime: seg.StartTime,
			EndTime:   seg.EndTime,
			Text:      seg.Text,
			Tokens:    segTokens,
		})
	}

	// If preview mode, return without saving
	if req.Preview {
		response := RetranscribeResponse{
			Success:          true,
			OriginalSegments: originalSegments,
			NewSegments:      newSegments,
			Model:            model,
			Tempo:            req.Tempo,
		}

		// Add Whisper Align specific fields if available
		if alignResult != nil {
			response.WhisperRawText = alignResult.RawText
			response.OriginalText = alignResult.OriginalText
			// Convert asr.AlignmentDiffItem to handlers.AlignmentDiffItem
			for _, d := range alignResult.Diff {
				response.AlignmentDiff = append(response.AlignmentDiff, AlignmentDiffItem{
					Char: d.Char,
					Op:   d.Op,
				})
			}
		}

		return c.JSON(http.StatusOK, response)
	}

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

	return c.JSON(http.StatusOK, RetranscribeResponse{
		Success:          true,
		Message:          "Retranscription completed",
		OriginalSegments: originalSegments,
		NewSegments:      newSegments,
		Model:            model,
		Tempo:            req.Tempo,
	})
}

// RetranscribeFullRequest represents the request body for full re-transcription
type RetranscribeFullRequest struct {
	Model string `json:"model"` // "reazonspeech" (default) or "sensevoice"
}

// RetranscribeFull handles full re-transcription of audio
// Deletes existing artifacts and articles, then creates a new transcription job
// POST /api/audio/:source_id/retranscribe-full
func (h *AudioHandler) RetranscribeFull(c echo.Context) error {
	ctx := c.Request().Context()
	sourceID := c.Param("source_id")

	// Parse request body (optional)
	var req RetranscribeFullRequest
	c.Bind(&req) // Ignore error - model will default to empty string

	// Default to reazonspeech if not specified
	model := req.Model
	if model == "" {
		model = storage.ASRModelReazonSpeech
	}

	// Validate model
	validModels := map[string]bool{
		storage.ASRModelReazonSpeech: true,
		storage.ASRModelSenseVoice:   true,
		// Note: sensevoice:beam is not supported yet by sherpa-onnx
	}
	if !validModels[model] {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid model: must be 'reazonspeech' or 'sensevoice'"})
	}

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

	// Create new transcription job via ingester with model selection
	jobID, err := h.ingester.CreateTranscriptionJob(ctx, sourceID, storage.JobPriorityImmediate, model)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create job: " + err.Error()})
	}

	return c.JSON(http.StatusAccepted, map[string]string{
		"message":   "Retranscription job created",
		"source_id": sourceID,
		"job_id":    jobID,
		"model":     model,
	})
}
