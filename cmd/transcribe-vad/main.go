// Experiment: VAD + ASR transcription
// Uses Silero VAD to detect speech segments, then transcribes only those segments
// Timestamps are adjusted to be relative to the original audio file
//
// Usage:
//   go run ./cmd/transcribe-vad -input audio.mp3
//   go run ./cmd/transcribe-vad -input audio.mp4 -vad-threshold 0.5

package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"zbor/internal/asr"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

const (
	sampleRate     = 16000
	bytesPerSample = 2 // 16-bit PCM
)

func main() {
	inputPath := flag.String("input", "", "Input audio/video file")
	modelDir := flag.String("model", "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01", "ASR model directory")
	vadModel := flag.String("vad-model", "models/silero_vad.onnx", "VAD model path")
	vadThreshold := flag.Float64("vad-threshold", 0.5, "VAD speech threshold (0-1)")
	minSpeech := flag.Float64("min-speech", 0.25, "Minimum speech duration (seconds)")
	minSilence := flag.Float64("min-silence", 0.5, "Minimum silence duration to split (seconds)")
	flag.Parse()

	if *inputPath == "" {
		log.Fatal("Usage: go run ./cmd/transcribe-vad -input <file>")
	}

	// Check VAD model exists
	if _, err := os.Stat(*vadModel); os.IsNotExist(err) {
		log.Fatalf("VAD model not found: %s\nDownload: curl -L -o models/silero_vad.onnx https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx", *vadModel)
	}

	// Get audio duration first
	duration, err := getAudioDuration(*inputPath)
	if err != nil {
		log.Fatalf("Failed to get duration: %v", err)
	}
	fmt.Printf("Audio duration: %.1f seconds\n", duration)

	// Create VAD
	vadConfig := sherpa.VadModelConfig{
		SileroVad: sherpa.SileroVadModelConfig{
			Model:             *vadModel,
			Threshold:         float32(*vadThreshold),
			MinSilenceDuration: float32(*minSilence),
			MinSpeechDuration:  float32(*minSpeech),
			WindowSize:        512,
		},
		SampleRate: sampleRate,
		NumThreads: 1,
		Debug:      0,
	}

	vad := sherpa.NewVoiceActivityDetector(&vadConfig, 30) // 30 seconds buffer
	if vad == nil {
		log.Fatal("Failed to create VAD")
	}
	defer sherpa.DeleteVoiceActivityDetector(vad)

	fmt.Printf("VAD initialized (threshold=%.2f, minSpeech=%.2fs, minSilence=%.2fs)\n",
		*vadThreshold, *minSpeech, *minSilence)

	// Create ASR recognizer
	asrConfig := &asr.Config{
		EncoderPath: filepath.Join(*modelDir, "encoder-epoch-99-avg-1.onnx"),
		DecoderPath: filepath.Join(*modelDir, "decoder-epoch-99-avg-1.onnx"),
		JoinerPath:  filepath.Join(*modelDir, "joiner-epoch-99-avg-1.onnx"),
		TokensPath:  filepath.Join(*modelDir, "tokens.txt"),
		SampleRate:  sampleRate,
		NumThreads:  4,
	}

	recognizer, err := asr.NewRecognizer(asrConfig)
	if err != nil {
		log.Fatalf("Failed to create recognizer: %v", err)
	}
	defer recognizer.Close()

	fmt.Println("ASR recognizer initialized")
	fmt.Println()

	// Start ffmpeg to convert to raw PCM
	cmd := exec.Command("ffmpeg",
		"-i", *inputPath,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", "1",
		"-loglevel", "error",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start ffmpeg: %v", err)
	}

	// Process audio through VAD
	reader := bufio.NewReader(stdout)
	windowSize := 512 // VAD window size
	windowBytes := windowSize * bytesPerSample

	startTime := time.Now()
	var processedSamples int64
	var speechSegments []speechSegment
	var allText string

	fmt.Println("Processing with VAD...")

	for {
		buffer := make([]byte, windowBytes)
		n, err := io.ReadFull(reader, buffer)

		if n == 0 {
			break
		}

		samples := bytesToFloat32(buffer[:n])
		vad.AcceptWaveform(samples)
		processedSamples += int64(len(samples))

		// Check for detected speech segments
		for !vad.IsEmpty() {
			segment := vad.Front()
			vad.Pop()

			startSec := float64(segment.Start) / float64(sampleRate)
			endSec := float64(segment.Start+len(segment.Samples)) / float64(sampleRate)
			durationSec := endSec - startSec

			fmt.Printf("\n--- Speech segment: %.2f - %.2f sec (%.2fs) ---\n",
				startSec, endSec, durationSec)

			// Transcribe this segment
			result, err := recognizer.TranscribeBytes(segment.Samples, sampleRate)
			if err != nil {
				log.Printf("Warning: transcription failed: %v", err)
				continue
			}

			// Adjust token timestamps with segment offset
			adjustedTokens := adjustTokenTimestamps(result.Tokens, float32(startSec))

			fmt.Printf("Text: %s\n", result.Text)
			fmt.Printf("Tokens: %d, Processing: %.2fs\n", len(adjustedTokens), result.Duration)

			speechSegments = append(speechSegments, speechSegment{
				start:  startSec,
				end:    endSec,
				text:   result.Text,
				tokens: adjustedTokens,
			})
			allText += result.Text
		}

		// Progress
		progressSec := float64(processedSamples) / float64(sampleRate)
		progress := progressSec / duration * 100
		if int(processedSamples)%(sampleRate*5) == 0 { // Every 5 seconds
			fmt.Printf("Progress: %.1f%% (%.1fs / %.1fs)\n", progress, progressSec, duration)
		}

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}

	// Flush remaining
	vad.Flush()
	for !vad.IsEmpty() {
		segment := vad.Front()
		vad.Pop()

		startSec := float64(segment.Start) / float64(sampleRate)
		endSec := float64(segment.Start+len(segment.Samples)) / float64(sampleRate)

		fmt.Printf("\n--- Final segment: %.2f - %.2f sec ---\n", startSec, endSec)

		result, err := recognizer.TranscribeBytes(segment.Samples, sampleRate)
		if err != nil {
			log.Printf("Warning: transcription failed: %v", err)
			continue
		}

		// Adjust token timestamps with segment offset
		adjustedTokens := adjustTokenTimestamps(result.Tokens, float32(startSec))

		fmt.Printf("Text: %s\n", result.Text)
		speechSegments = append(speechSegments, speechSegment{
			start:  startSec,
			end:    endSec,
			text:   result.Text,
			tokens: adjustedTokens,
		})
		allText += result.Text
	}

	cmd.Wait()

	// Summary
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Total time: %.1fs\n", time.Since(startTime).Seconds())
	fmt.Printf("Audio duration: %.1fs\n", duration)
	fmt.Printf("Speech segments: %d\n", len(speechSegments))

	var totalSpeech float64
	for _, seg := range speechSegments {
		totalSpeech += seg.end - seg.start
	}
	fmt.Printf("Total speech: %.1fs (%.1f%% of audio)\n", totalSpeech, totalSpeech/duration*100)

	fmt.Printf("\n=== Full Transcript ===\n%s\n", allText)

	// Show segments with timestamps
	fmt.Printf("\n=== Segments ===\n")
	for i, seg := range speechSegments {
		fmt.Printf("[%02d] %.2f-%.2f: %s\n", i+1, seg.start, seg.end, seg.text)
	}

	// Merge all tokens
	var allTokens []asr.Token
	for _, seg := range speechSegments {
		allTokens = append(allTokens, seg.tokens...)
	}

	// Show sample tokens with adjusted timestamps
	fmt.Printf("\n=== Sample Tokens (first 10) ===\n")
	for i, token := range allTokens {
		if i >= 10 {
			fmt.Printf("... and %d more tokens\n", len(allTokens)-10)
			break
		}
		fmt.Printf("  %.2fs: %s\n", token.StartTime, token.Text)
	}

	// Output JSON result (for integration testing)
	result := &asr.Result{
		Text:   allText,
		Tokens: allTokens,
	}
	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("\n=== JSON Result (summary) ===\n")
	fmt.Printf("Tokens: %d, Text length: %d chars\n", len(result.Tokens), len(result.Text))

	// Save to file
	outputPath := *inputPath + ".transcript.json"
	if err := os.WriteFile(outputPath, jsonBytes, 0644); err == nil {
		fmt.Printf("Saved to: %s\n", outputPath)
	}
}

type speechSegment struct {
	start  float64
	end    float64
	text   string
	tokens []asr.Token // Tokens with adjusted timestamps
}

// adjustTokenTimestamps adds offset to all token timestamps
func adjustTokenTimestamps(tokens []asr.Token, offsetSec float32) []asr.Token {
	adjusted := make([]asr.Token, len(tokens))
	for i, token := range tokens {
		adjusted[i] = asr.Token{
			Text:      token.Text,
			StartTime: token.StartTime + offsetSec,
			Duration:  token.Duration,
		}
	}
	return adjusted
}

func bytesToFloat32(data []byte) []float32 {
	samples := make([]float32, len(data)/2)
	for i := 0; i < len(samples); i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
		samples[i] = float32(sample) / 32768.0
	}
	return samples
}

func getAudioDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var duration float64
	fmt.Sscanf(string(output), "%f", &duration)
	return duration, nil
}
