package main

import (
	"fmt"
	"os"
	"zbor/internal/asr"
)

func main() {
	modelDir := "models/sherpa-onnx-whisper-turbo"
	testAudio := "internal/asr/testdata/mezurashii.wav"

	// Allow model dir override via command line
	if len(os.Args) > 1 {
		modelDir = os.Args[1]
	}
	if len(os.Args) > 2 {
		testAudio = os.Args[2]
	}

	config := asr.DefaultWhisperConfig(modelDir)

	fmt.Printf("Creating Whisper recognizer...\n")
	recognizer, err := asr.NewWhisperRecognizer(config)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer recognizer.Close()

	fmt.Printf("Transcribing %s...\n", testAudio)
	result, err := recognizer.TranscribeFile(testAudio, 30, func(p int, s string) {
		fmt.Printf("  [%d%%] %s\n", p, s)
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("\n=== Result ===\n")
	fmt.Printf("Text: %s\n", result.Text)
	fmt.Printf("Tokens: %d\n", len(result.Tokens))
	fmt.Printf("Duration: %.2f seconds\n", result.TotalDuration)

	fmt.Printf("\nFirst 10 tokens:\n")
	for i := 0; i < len(result.Tokens) && i < 10; i++ {
		t := result.Tokens[i]
		fmt.Printf("  [%.2fs +%.2fs] %q\n", t.StartTime, t.Duration, t.Text)
	}
}
