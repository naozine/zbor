package asr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTranscribeWithOverlap_OhayouYoroshiku tests that the overlap method
// correctly recognizes both "おはようございます" and "よろしくお願いします"
// which are spoken continuously without a pause.
//
// This test requires:
// - testdata/ohayou_yoroshiku.wav (local only)
// - models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01/
func TestTranscribeWithOverlap_OhayouYoroshiku(t *testing.T) {
	projectRoot := findProjectRoot(t)
	testAudio := filepath.Join(projectRoot, "internal/asr/testdata/ohayou_yoroshiku.wav")
	modelDir := filepath.Join(projectRoot, "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01")

	// Skip if test audio doesn't exist
	if _, err := os.Stat(testAudio); os.IsNotExist(err) {
		t.Skip("Test audio not found: testdata/ohayou_yoroshiku.wav (local test only)")
	}

	// Skip if models don't exist
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		t.Skip("Model not found: " + modelDir)
	}

	// Create config
	config, err := NewConfig(modelDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Create recognizer
	recognizer, err := NewRecognizer(config)
	if err != nil {
		t.Fatalf("Failed to create recognizer: %v", err)
	}
	defer recognizer.Close()

	// Silence config optimized for this audio
	silenceConfig := DefaultSilenceConfig()
	silenceConfig.SilenceThreshold = 0.0003 // Detect quiet speech
	silenceConfig.MaxBlockDuration = 2.5    // 2.5 second chunks

	tempo := 1.0
	overlap := 0.5 // 0.5 second overlap

	// Transcribe with overlap
	result, err := recognizer.TranscribeWithOverlap(testAudio, silenceConfig, tempo, overlap, nil)
	if err != nil {
		t.Fatalf("Transcription failed: %v", err)
	}

	// Check expected phrases are recognized
	expectedPhrases := []string{
		"おはよう",
		"よろしく",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(result.Text, phrase) {
			t.Errorf("Expected phrase not found: %q\nGot: %s", phrase, result.Text)
		}
	}

	t.Logf("Transcription result: %s", result.Text)
	t.Logf("Tokens: %d, Segments: %d", len(result.Tokens), len(result.Segments))
}

// TestSplitLongBlocksWithOverlap tests the overlap block splitting logic
func TestSplitLongBlocksWithOverlap(t *testing.T) {
	tests := []struct {
		name        string
		blocks      []SpeechBlock
		maxDuration float64
		overlap     float64
		wantCount   int
	}{
		{
			name: "no split needed - short block",
			blocks: []SpeechBlock{
				{StartTime: 0, EndTime: 2},
			},
			maxDuration: 3,
			overlap:     0.5,
			wantCount:   1,
		},
		{
			name: "split 6s block into overlapping chunks",
			blocks: []SpeechBlock{
				{StartTime: 0, EndTime: 6},
			},
			maxDuration: 3,
			overlap:     0.5,
			// main = 3 - 0.5 = 2.5s
			// chunk 1: 0-3 (main: 0-2.5)
			// chunk 2: 2.5-5.5 (main: 2.5-5)
			// chunk 3: 5-6 (main: 5-6)
			wantCount: 3,
		},
		{
			name: "split 10s block with 2s overlap",
			blocks: []SpeechBlock{
				{StartTime: 0, EndTime: 10},
			},
			maxDuration: 5,
			overlap:     2,
			// main = 5 - 2 = 3s
			// chunk 1: 0-5 (main: 0-3)
			// chunk 2: 3-8 (main: 3-6)
			// chunk 3: 6-10 (main: 6-9)
			// chunk 4: 9-10 (main: 9-10)
			wantCount: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLongBlocksWithOverlap(tt.blocks, tt.maxDuration, tt.overlap)

			if len(result) != tt.wantCount {
				t.Errorf("got %d blocks, want %d", len(result), tt.wantCount)
				for i, b := range result {
					t.Logf("  block %d: %.2f - %.2f (main: %.2f - %.2f)",
						i, b.StartTime, b.EndTime, b.MainStart, b.MainEnd)
				}
			}

			// Verify main portions don't overlap
			for i := 1; i < len(result); i++ {
				if result[i].MainStart < result[i-1].MainEnd {
					t.Errorf("main portions overlap: block %d ends at %.2f, block %d starts at %.2f",
						i-1, result[i-1].MainEnd, i, result[i].MainStart)
				}
			}
		})
	}
}
