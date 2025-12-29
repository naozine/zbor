package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"zbor/internal/asr"
	"zbor/internal/storage"
	"zbor/internal/storage/sqlc"

	"github.com/google/uuid"
)

// AudioIngester handles audio file ingestion and transcription
type AudioIngester struct {
	sourceRepo        *storage.SourceRepository
	artifactRepo      *storage.ArtifactRepository
	articleRepo       *storage.ArticleRepository
	jobRepo           *storage.JobRepository
	asrConfig         *asr.Config
	senseVoiceConfig  *asr.SenseVoiceConfig
	dataDir           string
}

// NewAudioIngester creates a new AudioIngester
func NewAudioIngester(
	sourceRepo *storage.SourceRepository,
	artifactRepo *storage.ArtifactRepository,
	articleRepo *storage.ArticleRepository,
	jobRepo *storage.JobRepository,
	asrConfig *asr.Config,
	dataDir string,
) *AudioIngester {
	// SenseVoice model path (relative to project root)
	senseVoiceModelDir := "models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17"

	return &AudioIngester{
		sourceRepo:        sourceRepo,
		artifactRepo:      artifactRepo,
		articleRepo:       articleRepo,
		jobRepo:           jobRepo,
		asrConfig:         asrConfig,
		senseVoiceConfig:  asr.DefaultSenseVoiceConfig(senseVoiceModelDir),
		dataDir:           dataDir,
	}
}

// AudioFile represents an uploaded audio file
type AudioFile struct {
	Filename string
	Reader   io.Reader
	Speaker  string // optional speaker label
}

// IngestOptions contains options for audio ingestion
type IngestOptions struct {
	Title    string       // optional title for the article
	Files    []AudioFile  // audio files to process
	Priority int          // job priority (0-9, lower is higher priority)
}

// IngestResult contains the result of audio ingestion
type IngestResult struct {
	SourceID string
	JobID    string
}

// ProgressCallback is called to report progress during processing
type ProgressCallback func(progress int, step string)

// Ingest starts the audio ingestion process
// It saves the files, creates a source record, and queues a job for processing
func (i *AudioIngester) Ingest(ctx context.Context, opts IngestOptions) (*IngestResult, error) {
	if len(opts.Files) == 0 {
		return nil, fmt.Errorf("no audio files provided")
	}

	// Generate source ID
	sourceID := uuid.New().String()

	// Create directory for source files
	sourceDir := filepath.Join(i.dataDir, "sources", "audio", sourceID)
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create source directory: %w", err)
	}

	// Save uploaded files
	var filePaths []string
	var speakers []string
	for _, file := range opts.Files {
		if !asr.IsSupportedFormat(file.Filename) {
			return nil, fmt.Errorf("unsupported audio format: %s", file.Filename)
		}

		destPath := filepath.Join(sourceDir, file.Filename)
		dest, err := os.Create(destPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create file: %w", err)
		}

		_, err = io.Copy(dest, file.Reader)
		dest.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to save file: %w", err)
		}

		filePaths = append(filePaths, destPath)

		// Extract speaker from filename if not provided
		speaker := file.Speaker
		if speaker == "" {
			speaker = strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
		}
		speakers = append(speakers, speaker)
	}

	// Create metadata
	metadata := map[string]interface{}{
		"files":    filePaths,
		"speakers": speakers,
		"title":    opts.Title,
	}
	metadataJSON, _ := json.Marshal(metadata)

	// Create source record
	source := &sqlc.Source{
		ID:       sourceID,
		Type:     "audio",
		FilePath: storage.Ptr(sourceDir),
		Metadata: storage.Ptr(string(metadataJSON)),
		Status:   storage.Ptr(storage.SourceStatusPending),
	}
	if err := i.sourceRepo.Create(ctx, source); err != nil {
		return nil, fmt.Errorf("failed to create source: %w", err)
	}

	// Create job for processing
	job := &sqlc.ProcessingJob{
		SourceID: &sourceID,
		Type:     storage.JobTypeTranscribe,
		Priority: storage.Ptr(int64(opts.Priority)),
	}
	if err := i.jobRepo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return &IngestResult{
		SourceID: sourceID,
		JobID:    job.ID,
	}, nil
}

// CreateTranscriptionJob creates a new transcription job for an existing source
// Used for retranscription (re-processing an existing source)
// model: "reazonspeech" (default), "sensevoice"
func (i *AudioIngester) CreateTranscriptionJob(ctx context.Context, sourceID string, priority int, model string) (string, error) {
	// Verify source exists
	source, err := i.sourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return "", fmt.Errorf("failed to get source: %w", err)
	}
	if source == nil {
		return "", fmt.Errorf("source not found: %s", sourceID)
	}

	// Reset source status
	if err := i.sourceRepo.UpdateStatus(ctx, sourceID, storage.SourceStatusPending); err != nil {
		return "", fmt.Errorf("failed to update source status: %w", err)
	}

	// Determine job type based on model
	jobType := storage.JobTypeTranscribe // default
	switch model {
	case storage.ASRModelSenseVoice:
		jobType = storage.JobTypeTranscribeSenseVoice
	case storage.ASRModelReazonSpeech:
		jobType = storage.JobTypeTranscribeReazonSpeech
	}

	// Create job for processing
	job := &sqlc.ProcessingJob{
		SourceID: &sourceID,
		Type:     jobType,
		Priority: storage.Ptr(int64(priority)),
	}
	if err := i.jobRepo.Create(ctx, job); err != nil {
		return "", fmt.Errorf("failed to create job: %w", err)
	}

	return job.ID, nil
}

// ProcessTranscription processes a transcription job
// This is called by the worker when processing the job
func (i *AudioIngester) ProcessTranscription(ctx context.Context, job *sqlc.ProcessingJob, onProgress ProgressCallback) error {
	if job.SourceID == nil {
		return fmt.Errorf("job has no source ID")
	}

	// Helper to report progress (nil-safe)
	reportProgress := func(progress int, step string) {
		if onProgress != nil {
			onProgress(progress, step)
		}
	}

	reportProgress(5, "preparing")

	// Get source
	source, err := i.sourceRepo.GetByID(ctx, *job.SourceID)
	if err != nil {
		return fmt.Errorf("failed to get source: %w", err)
	}
	if source == nil {
		return fmt.Errorf("source not found: %s", *job.SourceID)
	}

	// Update source status
	if err := i.sourceRepo.UpdateStatus(ctx, source.ID, storage.SourceStatusProcessing); err != nil {
		return fmt.Errorf("failed to update source status: %w", err)
	}

	// Parse metadata
	var metadata struct {
		Files    []string `json:"files"`
		Speakers []string `json:"speakers"`
		Title    string   `json:"title"`
	}
	if source.Metadata != nil {
		if err := json.Unmarshal([]byte(*source.Metadata), &metadata); err != nil {
			return fmt.Errorf("failed to parse metadata: %w", err)
		}
	}

	reportProgress(10, "initializing")

	// Determine which model to use based on job type
	useSenseVoice := job.Type == storage.JobTypeTranscribeSenseVoice

	// Process each file
	var allResults []*asr.Result
	fileCount := len(metadata.Files)
	if fileCount == 0 {
		return fmt.Errorf("no audio files in source metadata")
	}

	if useSenseVoice {
		// === SenseVoice Model ===
		svRecognizer, err := asr.NewSenseVoiceRecognizer(i.senseVoiceConfig)
		if err != nil {
			return fmt.Errorf("failed to create SenseVoice recognizer: %w", err)
		}
		defer svRecognizer.Close()

		for idx, filePath := range metadata.Files {
			fileProgressStart := 30 + (60 * idx / fileCount)
			fileProgressEnd := 30 + (60 * (idx + 1) / fileCount)

			result, err := svRecognizer.TranscribeFile(filePath, 20, func(progress int, step string) {
				fileProgress := fileProgressStart + (progress-10)*(fileProgressEnd-fileProgressStart)/80
				reportProgress(fileProgress, step)
			})
			if err != nil {
				return fmt.Errorf("failed to transcribe %s with SenseVoice: %w", filePath, err)
			}

			// Add speaker label
			if idx < len(metadata.Speakers) {
				result.Speaker = metadata.Speakers[idx]
			}

			allResults = append(allResults, result)
		}
	} else {
		// === ReazonSpeech Model (default) ===
		recognizer, err := asr.NewRecognizer(i.asrConfig)
		if err != nil {
			return fmt.Errorf("failed to create recognizer: %w", err)
		}
		defer recognizer.Close()

		// Determine transcription method
		// VADモデルがあれば TranscribeWithOverlap を使用（本番推奨）
		useOverlap := i.asrConfig.VADModelPath != ""

		for idx, filePath := range metadata.Files {
			// Calculate progress: transcribing takes 30-90%
			// Each file gets an equal share of that range
			fileProgressStart := 30 + (60 * idx / fileCount)
			fileProgressEnd := 30 + (60 * (idx + 1) / fileCount)

			var result *asr.Result

			if useOverlap {
				// 【本番用】オーバーラップ付きsilence検出による文字起こし
				// RMSベースの無音検出 + オーバーラップで連続発話も正確に認識
				silenceConfig := asr.DefaultSilenceConfig()
				silenceConfig.SilenceThreshold = 0.0003 // 静かな音声も検出
				silenceConfig.MinSilenceDuration = 0.5  // 500ms以上の無音で分割
				silenceConfig.MaxBlockDuration = 10.0   // 10秒チャンク

				tempo := 1.0   // 通常は速度調整不要
				overlap := 2.0 // 2秒オーバーラップ

				result, err = recognizer.TranscribeWithOverlap(filePath, silenceConfig, tempo, overlap, func(progress int, step string) {
					fileProgress := fileProgressStart + (progress-30)*(fileProgressEnd-fileProgressStart)/60
					reportProgress(fileProgress, step)
				})
				if err != nil {
					return fmt.Errorf("failed to transcribe %s: %w", filePath, err)
				}
			} else {
				// Fallback: Convert to WAV and use standard transcription
				reportProgress(fileProgressStart, "converting")
				needsConvert, _ := asr.NeedsConversion(filePath)
				wavPath := filePath
				if needsConvert {
					wavPath, err = asr.ConvertToWavTemp(filePath)
					if err != nil {
						return fmt.Errorf("failed to convert audio: %w", err)
					}
					defer os.Remove(wavPath)
				}

				reportProgress(fileProgressStart+10, "transcribing")
				result, err = recognizer.TranscribeFile(wavPath)
				if err != nil {
					return fmt.Errorf("failed to transcribe %s: %w", filePath, err)
				}
			}

			// Add speaker label
			if idx < len(metadata.Speakers) {
				result.Speaker = metadata.Speakers[idx]
			}

			allResults = append(allResults, result)
		}
	}

	reportProgress(90, "saving")

	// Merge results if multiple files
	var finalResult *asr.Result
	if len(allResults) == 1 {
		finalResult = allResults[0]
	} else {
		finalResult = mergeResults(allResults)
	}

	// Save transcription artifact
	artifactContent, _ := json.Marshal(finalResult)
	artifact := &sqlc.ProcessingArtifact{
		SourceID: &source.ID,
		Type:     storage.ArtifactTypeTranscription,
		Content:  storage.Ptr(string(artifactContent)),
		Format:   storage.Ptr("json"),
	}
	if err := i.artifactRepo.Create(ctx, artifact); err != nil {
		return fmt.Errorf("failed to save artifact: %w", err)
	}

	// Generate article
	title := metadata.Title
	if title == "" {
		title = fmt.Sprintf("Meeting %s", time.Now().Format("2006-01-02"))
	}

	article := &sqlc.Article{
		Title:      title,
		Content:    finalResult.FormatAsText(),
		SourceType: storage.Ptr("audio"),
		SourceID:   &source.ID,
		Language:   storage.Ptr("ja"),
	}
	if err := i.articleRepo.Create(ctx, article); err != nil {
		return fmt.Errorf("failed to create article: %w", err)
	}

	// Update source status to completed
	if err := i.sourceRepo.UpdateStatus(ctx, source.ID, storage.SourceStatusCompleted); err != nil {
		return fmt.Errorf("failed to update source status: %w", err)
	}

	reportProgress(100, "")

	return nil
}

// mergeResults merges multiple transcription results sorted by timestamp
func mergeResults(results []*asr.Result) *asr.Result {
	if len(results) == 0 {
		return &asr.Result{}
	}

	// Collect all tokens with speaker labels
	type tokenWithSpeaker struct {
		token   asr.Token
		speaker string
	}
	var allTokens []tokenWithSpeaker

	for _, r := range results {
		for _, t := range r.Tokens {
			allTokens = append(allTokens, tokenWithSpeaker{
				token:   t,
				speaker: r.Speaker,
			})
		}
	}

	// Sort by start time
	for i := 0; i < len(allTokens); i++ {
		for j := i + 1; j < len(allTokens); j++ {
			if allTokens[j].token.StartTime < allTokens[i].token.StartTime {
				allTokens[i], allTokens[j] = allTokens[j], allTokens[i]
			}
		}
	}

	// Build merged result
	merged := &asr.Result{
		Tokens: make([]asr.Token, 0, len(allTokens)),
	}

	var textBuilder strings.Builder
	var lastSpeaker string

	for _, t := range allTokens {
		// Add speaker label when speaker changes
		if t.speaker != lastSpeaker && t.speaker != "" {
			if textBuilder.Len() > 0 {
				textBuilder.WriteString("\n")
			}
			textBuilder.WriteString(fmt.Sprintf("[%s] ", t.speaker))
			lastSpeaker = t.speaker
		}
		textBuilder.WriteString(t.token.Text)
		merged.Tokens = append(merged.Tokens, t.token)
	}

	merged.Text = textBuilder.String()

	// Calculate total duration
	if len(merged.Tokens) > 0 {
		lastToken := merged.Tokens[len(merged.Tokens)-1]
		merged.TotalDuration = lastToken.StartTime + lastToken.Duration
	}

	return merged
}
