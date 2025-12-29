package asr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTranscribeWithVADBlock_Mezurashii tests that the problematic audio
// "mezurashii.wav" correctly recognizes both "今回が一番" and "珍しいですけどね".
//
// This test requires:
// - testdata/mezurashii.wav (not committed, local only)
// - models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01/
// - models/silero_vad.onnx
func TestTranscribeWithVADBlock_Mezurashii(t *testing.T) {
	// Get paths relative to project root
	projectRoot := findProjectRoot(t)
	testAudio := filepath.Join(projectRoot, "internal/asr/testdata/mezurashii.wav")
	modelDir := filepath.Join(projectRoot, "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01")
	vadModel := filepath.Join(projectRoot, "models/silero_vad.onnx")

	// Skip if test audio doesn't exist
	if _, err := os.Stat(testAudio); os.IsNotExist(err) {
		t.Skip("Test audio not found: testdata/mezurashii.wav (local test only)")
	}

	// Skip if models don't exist
	if _, err := os.Stat(modelDir); os.IsNotExist(err) {
		t.Skip("Model not found: " + modelDir)
	}
	if _, err := os.Stat(vadModel); os.IsNotExist(err) {
		t.Skip("VAD model not found: " + vadModel)
	}

	// Create config with optimal settings
	config, err := NewConfig(modelDir)
	if err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}
	config.DecodingMethod = "modified_beam_search"
	config.MaxActivePaths = 4

	// Create recognizer
	recognizer, err := NewRecognizer(config)
	if err != nil {
		t.Fatalf("Failed to create recognizer: %v", err)
	}
	defer recognizer.Close()

	// VAD config with optimal settings for this audio
	vadConfig := DefaultVADConfig(vadModel)
	vadConfig.Threshold = 0.1          // More sensitive
	vadConfig.MinSilenceDuration = 6.0 // Merge blocks
	vadConfig.MaxBlockDuration = 5.0   // Split long blocks

	// Transcribe
	result, err := recognizer.TranscribeWithVADBlock(testAudio, vadConfig, 1.0, nil)
	if err != nil {
		t.Fatalf("Transcription failed: %v", err)
	}

	// Check expected phrases are recognized
	expectedPhrases := []string{
		"今回が一番",
		"珍しい",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(result.Text, phrase) {
			t.Errorf("Expected phrase not found: %q\nGot: %s", phrase, result.Text)
		}
	}

	t.Logf("Transcription result: %s", result.Text)
	t.Logf("Tokens: %d, Segments: %d", len(result.Tokens), len(result.Segments))
}

// TestSplitLongBlocks tests the block splitting logic
func TestSplitLongBlocks(t *testing.T) {
	tests := []struct {
		name        string
		blocks      []SpeechBlock
		maxDuration float64
		wantCount   int
		wantFirst   SpeechBlock
		wantLast    SpeechBlock
	}{
		{
			name: "no split needed",
			blocks: []SpeechBlock{
				{StartTime: 0, EndTime: 3},
				{StartTime: 5, EndTime: 8},
			},
			maxDuration: 5,
			wantCount:   2,
			wantFirst:   SpeechBlock{StartTime: 0, EndTime: 3},
			wantLast:    SpeechBlock{StartTime: 5, EndTime: 8},
		},
		{
			name: "split 20s block into 5s chunks",
			blocks: []SpeechBlock{
				{StartTime: 0, EndTime: 20},
			},
			maxDuration: 5,
			wantCount:   4,
			wantFirst:   SpeechBlock{StartTime: 0, EndTime: 5},
			wantLast:    SpeechBlock{StartTime: 15, EndTime: 20},
		},
		{
			name: "split with remainder",
			blocks: []SpeechBlock{
				{StartTime: 10, EndTime: 23},
			},
			maxDuration: 5,
			wantCount:   3,
			wantFirst:   SpeechBlock{StartTime: 10, EndTime: 15},
			wantLast:    SpeechBlock{StartTime: 20, EndTime: 23},
		},
		{
			name: "maxDuration 0 disables splitting",
			blocks: []SpeechBlock{
				{StartTime: 0, EndTime: 100},
			},
			maxDuration: 0,
			wantCount:   1,
			wantFirst:   SpeechBlock{StartTime: 0, EndTime: 100},
			wantLast:    SpeechBlock{StartTime: 0, EndTime: 100},
		},
		{
			name: "mixed blocks",
			blocks: []SpeechBlock{
				{StartTime: 0, EndTime: 3},   // no split
				{StartTime: 5, EndTime: 17},  // split into 3
				{StartTime: 20, EndTime: 22}, // no split
			},
			maxDuration: 5,
			wantCount:   5,
			wantFirst:   SpeechBlock{StartTime: 0, EndTime: 3},
			wantLast:    SpeechBlock{StartTime: 20, EndTime: 22},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLongBlocks(tt.blocks, tt.maxDuration)

			if len(result) != tt.wantCount {
				t.Errorf("got %d blocks, want %d", len(result), tt.wantCount)
				for i, b := range result {
					t.Logf("  block %d: %.2f - %.2f", i, b.StartTime, b.EndTime)
				}
			}

			if len(result) > 0 {
				if result[0] != tt.wantFirst {
					t.Errorf("first block = %+v, want %+v", result[0], tt.wantFirst)
				}
				if result[len(result)-1] != tt.wantLast {
					t.Errorf("last block = %+v, want %+v", result[len(result)-1], tt.wantLast)
				}
			}
		})
	}
}

// findProjectRoot finds the project root by looking for go.mod
func findProjectRoot(t *testing.T) string {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find project root (go.mod)")
		}
		dir = parent
	}
}
