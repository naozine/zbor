package asr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TranscriberTestCase defines a test case for transcription comparison
type TranscriberTestCase struct {
	Name           string   // Test case name
	AudioFile      string   // Relative path from testdata/
	ExpectedPhrases []string // Phrases that should be recognized by both models
}

// Common test cases for both models
var commonTestCases = []TranscriberTestCase{
	{
		Name:      "Mezurashii",
		AudioFile: "mezurashii.wav",
		ExpectedPhrases: []string{
			"今回",
			"珍しい",
		},
	},
	{
		Name:      "OhayouYoroshiku",
		AudioFile: "ohayou_yoroshiku.wav",
		ExpectedPhrases: []string{
			"おはよう",
			"よろしく",
		},
	},
}

// ModelConfig holds configuration for each ASR model
type ModelConfig struct {
	Name        string
	Setup       func(projectRoot string) (Transcriber, error)
	ChunkSec    int
	SkipMessage string
}

// Transcriber interface for both recognizer types
type Transcriber interface {
	Transcribe(audioPath string) (*Result, error)
	Close()
}

// ReazonSpeechTranscriber wraps Recognizer for testing
type ReazonSpeechTranscriber struct {
	recognizer    *Recognizer
	silenceConfig *SilenceConfig
}

func (t *ReazonSpeechTranscriber) Transcribe(audioPath string) (*Result, error) {
	return t.recognizer.TranscribeWithOverlap(audioPath, t.silenceConfig, 1.0, 2.0, nil)
}

func (t *ReazonSpeechTranscriber) Close() {
	t.recognizer.Close()
}

// SenseVoiceTranscriber wraps SenseVoiceRecognizer for testing
type SenseVoiceTranscriber struct {
	recognizer *SenseVoiceRecognizer
	chunkSec   int
}

func (t *SenseVoiceTranscriber) Transcribe(audioPath string) (*Result, error) {
	return t.recognizer.TranscribeFile(audioPath, t.chunkSec, nil)
}

func (t *SenseVoiceTranscriber) Close() {
	t.recognizer.Close()
}

// setupReazonSpeech creates a ReazonSpeech transcriber
func setupReazonSpeech(projectRoot string) (Transcriber, error) {
	modelDir := filepath.Join(projectRoot, "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01")

	config, err := NewConfig(modelDir)
	if err != nil {
		return nil, err
	}

	recognizer, err := NewRecognizer(config)
	if err != nil {
		return nil, err
	}

	silenceConfig := DefaultSilenceConfig()
	silenceConfig.SilenceThreshold = 0.0003
	silenceConfig.MinSilenceDuration = 0.5
	silenceConfig.MaxBlockDuration = 10.0

	return &ReazonSpeechTranscriber{
		recognizer:    recognizer,
		silenceConfig: silenceConfig,
	}, nil
}

// setupSenseVoice creates a SenseVoice transcriber
func setupSenseVoice(projectRoot string) (Transcriber, error) {
	modelDir := filepath.Join(projectRoot, "models/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-2024-07-17")

	config := DefaultSenseVoiceConfig(modelDir)

	recognizer, err := NewSenseVoiceRecognizer(config)
	if err != nil {
		return nil, err
	}

	return &SenseVoiceTranscriber{
		recognizer: recognizer,
		chunkSec:   20,
	}, nil
}

// modelConfigs defines all models to test
var modelConfigs = []ModelConfig{
	{
		Name:        "ReazonSpeech",
		Setup:       setupReazonSpeech,
		SkipMessage: "ReazonSpeech model not found",
	},
	{
		Name:        "SenseVoice",
		Setup:       setupSenseVoice,
		SkipMessage: "SenseVoice model not found",
	},
}

// TestComparison_BothModelsRecognizeSamePhrases tests that both models
// recognize the same expected phrases from the same audio files.
func TestComparison_BothModelsRecognizeSamePhrases(t *testing.T) {
	projectRoot := findProjectRoot(t)
	testdataDir := filepath.Join(projectRoot, "internal/asr/testdata")

	for _, tc := range commonTestCases {
		tc := tc // capture range variable
		t.Run(tc.Name, func(t *testing.T) {
			audioPath := filepath.Join(testdataDir, tc.AudioFile)

			// Skip if audio file doesn't exist
			if _, err := os.Stat(audioPath); os.IsNotExist(err) {
				t.Skipf("Test audio not found: %s (local test only)", tc.AudioFile)
			}

			// Track results for comparison
			results := make(map[string]*Result)

			for _, mc := range modelConfigs {
				mc := mc // capture range variable
				t.Run(mc.Name, func(t *testing.T) {
					transcriber, err := mc.Setup(projectRoot)
					if err != nil {
						t.Skipf("%s: %v", mc.SkipMessage, err)
					}
					defer transcriber.Close()

					result, err := transcriber.Transcribe(audioPath)
					if err != nil {
						t.Fatalf("Transcription failed: %v", err)
					}

					results[mc.Name] = result

					// Check expected phrases
					for _, phrase := range tc.ExpectedPhrases {
						if !strings.Contains(result.Text, phrase) {
							t.Errorf("Expected phrase not found: %q\nGot: %s", phrase, result.Text)
						}
					}

					t.Logf("Result: %s", result.Text)
					t.Logf("Tokens: %d", len(result.Tokens))
				})
			}
		})
	}
}

// TestComparison_TokenTimestampsValid tests that both models produce
// valid token timestamps (non-negative, monotonically increasing).
func TestComparison_TokenTimestampsValid(t *testing.T) {
	projectRoot := findProjectRoot(t)
	testdataDir := filepath.Join(projectRoot, "internal/asr/testdata")

	for _, tc := range commonTestCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			audioPath := filepath.Join(testdataDir, tc.AudioFile)

			if _, err := os.Stat(audioPath); os.IsNotExist(err) {
				t.Skipf("Test audio not found: %s", tc.AudioFile)
			}

			for _, mc := range modelConfigs {
				mc := mc
				t.Run(mc.Name, func(t *testing.T) {
					transcriber, err := mc.Setup(projectRoot)
					if err != nil {
						t.Skipf("%s: %v", mc.SkipMessage, err)
					}
					defer transcriber.Close()

					result, err := transcriber.Transcribe(audioPath)
					if err != nil {
						t.Fatalf("Transcription failed: %v", err)
					}

					// Verify timestamps
					var lastTime float32 = -1
					for i, token := range result.Tokens {
						if token.StartTime < 0 {
							t.Errorf("Token %d has negative start time: %f", i, token.StartTime)
						}
						if token.Duration < 0 {
							t.Errorf("Token %d has negative duration: %f", i, token.Duration)
						}
						// Allow some tolerance for timestamp ordering
						if token.StartTime < lastTime-0.1 {
							t.Errorf("Token %d timestamp not monotonic: %f < %f",
								i, token.StartTime, lastTime)
						}
						lastTime = token.StartTime
					}

					t.Logf("Validated %d tokens", len(result.Tokens))
				})
			}
		})
	}
}

// TestComparison_SegmentsGenerated tests that both models generate
// proper segments from the transcription.
func TestComparison_SegmentsGenerated(t *testing.T) {
	projectRoot := findProjectRoot(t)
	testdataDir := filepath.Join(projectRoot, "internal/asr/testdata")

	for _, tc := range commonTestCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			audioPath := filepath.Join(testdataDir, tc.AudioFile)

			if _, err := os.Stat(audioPath); os.IsNotExist(err) {
				t.Skipf("Test audio not found: %s", tc.AudioFile)
			}

			for _, mc := range modelConfigs {
				mc := mc
				t.Run(mc.Name, func(t *testing.T) {
					transcriber, err := mc.Setup(projectRoot)
					if err != nil {
						t.Skipf("%s: %v", mc.SkipMessage, err)
					}
					defer transcriber.Close()

					result, err := transcriber.Transcribe(audioPath)
					if err != nil {
						t.Fatalf("Transcription failed: %v", err)
					}

					// Should have at least one segment
					if len(result.Segments) == 0 {
						t.Error("No segments generated")
					}

					// Verify segment structure
					for i, seg := range result.Segments {
						if seg.EndTime < seg.StartTime {
							t.Errorf("Segment %d has invalid time range: %f - %f",
								i, seg.StartTime, seg.EndTime)
						}
						if seg.Text == "" {
							t.Errorf("Segment %d has empty text", i)
						}
					}

					t.Logf("Generated %d segments", len(result.Segments))
				})
			}
		})
	}
}
