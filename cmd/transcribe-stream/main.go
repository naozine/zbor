// Experiment: Streaming transcription with ffmpeg pipe
// Tests:
// 1. ffmpeg pipe (any format → raw PCM)
// 2. Chunked processing with progress
//
// Usage:
//   go run ./cmd/transcribe-stream -input audio.mp4
//   go run ./cmd/transcribe-stream -input audio.wav -chunk 30

package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"zbor/internal/asr"
)

const (
	sampleRate    = 16000
	bytesPerSample = 2 // 16-bit PCM
)

func main() {
	inputPath := flag.String("input", "", "Input audio/video file")
	chunkSec := flag.Float64("chunk", 30, "Chunk duration in seconds")
	modelDir := flag.String("model", "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01", "Model directory")
	flag.Parse()

	if *inputPath == "" {
		log.Fatal("Usage: go run ./cmd/transcribe-stream -input <file>")
	}

	// Get audio duration first
	duration, err := getAudioDuration(*inputPath)
	if err != nil {
		log.Fatalf("Failed to get duration: %v", err)
	}
	fmt.Printf("Audio duration: %.1f seconds\n", duration)
	fmt.Printf("Chunk size: %.1f seconds\n", *chunkSec)

	totalChunks := int(duration / *chunkSec) + 1
	fmt.Printf("Expected chunks: %d\n\n", totalChunks)

	// Create recognizer
	config := &asr.Config{
		EncoderPath: filepath.Join(*modelDir, "encoder-epoch-99-avg-1.onnx"),
		DecoderPath: filepath.Join(*modelDir, "decoder-epoch-99-avg-1.onnx"),
		JoinerPath:  filepath.Join(*modelDir, "joiner-epoch-99-avg-1.onnx"),
		TokensPath:  filepath.Join(*modelDir, "tokens.txt"),
		SampleRate:  sampleRate,
		NumThreads:  4,
	}

	recognizer, err := asr.NewRecognizer(config)
	if err != nil {
		log.Fatalf("Failed to create recognizer: %v", err)
	}
	defer recognizer.Close()

	// Start ffmpeg process to convert to raw PCM
	cmd := exec.Command("ffmpeg",
		"-i", *inputPath,
		"-f", "s16le",        // 16-bit signed little-endian PCM
		"-acodec", "pcm_s16le",
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", "1",           // mono
		"-loglevel", "error",
		"pipe:1",             // output to stdout
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start ffmpeg: %v", err)
	}

	// Process chunks
	reader := bufio.NewReader(stdout)
	chunkSamples := int(*chunkSec * float64(sampleRate))
	chunkBytes := chunkSamples * bytesPerSample

	var allText string
	chunkIndex := 0
	startTime := time.Now()

	for {
		// Read chunk
		buffer := make([]byte, chunkBytes)
		n, err := io.ReadFull(reader, buffer)

		if n == 0 {
			break
		}

		// Convert bytes to float32 samples
		samples := bytesToFloat32(buffer[:n])

		fmt.Printf("\n--- Chunk %d (%.1f-%.1f sec) ---\n",
			chunkIndex,
			float64(chunkIndex) * *chunkSec,
			float64(chunkIndex) * *chunkSec + float64(n) / float64(bytesPerSample) / float64(sampleRate))

		// Transcribe
		result, err := recognizer.TranscribeBytes(samples, sampleRate)
		if err != nil {
			log.Printf("Warning: transcription failed for chunk %d: %v", chunkIndex, err)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			continue
		}

		fmt.Printf("Text: %s\n", result.Text)
		fmt.Printf("Tokens: %d, Duration: %.2fs\n", len(result.Tokens), result.Duration)

		allText += result.Text

		// Progress
		progress := float64(chunkIndex+1) / float64(totalChunks) * 100
		elapsed := time.Since(startTime).Seconds()
		fmt.Printf("Progress: %.1f%% (elapsed: %.1fs)\n", progress, elapsed)

		chunkIndex++

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}

	cmd.Wait()

	fmt.Printf("\n=== Final Result ===\n")
	fmt.Printf("Total chunks: %d\n", chunkIndex)
	fmt.Printf("Total time: %.1fs\n", time.Since(startTime).Seconds())
	fmt.Printf("\nFull text:\n%s\n", allText)
}

// bytesToFloat32 converts 16-bit PCM bytes to float32 samples
func bytesToFloat32(data []byte) []float32 {
	samples := make([]float32, len(data)/2)
	for i := 0; i < len(samples); i++ {
		sample := int16(binary.LittleEndian.Uint16(data[i*2:]))
		samples[i] = float32(sample) / 32768.0
	}
	return samples
}

// getAudioDuration gets audio duration using ffprobe
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
