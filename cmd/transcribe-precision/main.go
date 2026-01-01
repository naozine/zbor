// Experiment: ASR precision parameters
// Tests different decoding methods and beam search parameters
// to improve accuracy on fast speech sections
//
// Usage:
//   go run ./cmd/transcribe-precision -input audio.mp3
//   go run ./cmd/transcribe-precision -input audio.mp3 -method modified_beam_search -beam 10

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
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

const sampleRate = 16000

func main() {
	inputPath := flag.String("input", "", "Input audio/video file")
	modelDir := flag.String("model", "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01", "ASR model directory")

	// Decoding parameters
	decodingMethod := flag.String("method", "greedy_search", "Decoding method: greedy_search or modified_beam_search")
	maxActivePaths := flag.Int("beam", 4, "Max active paths for beam search (higher = more accurate but slower)")
	blankPenalty := flag.Float64("blank-penalty", 0.0, "Penalty for blank tokens (try 1.0-2.0 for fast speech)")
	numThreads := flag.Int("threads", 4, "Number of threads")
	tempo := flag.Float64("tempo", 1.0, "Audio tempo (0.9 = slower for fast speech, timestamps auto-corrected)")
	chunkSec := flag.Int("chunk", 20, "Chunk size in seconds (without VAD)")

	// VAD parameters (optional)
	vadModel := flag.String("vad-model", "", "VAD model path (optional, empty = no VAD)")
	vadThreshold := flag.Float64("vad-threshold", 0.5, "VAD speech threshold")

	flag.Parse()

	if *inputPath == "" {
		log.Fatal("Usage: go run ./cmd/transcribe-precision -input <file>")
	}

	fmt.Println("=== ASR Precision Test ===")
	fmt.Printf("DecodingMethod: %s\n", *decodingMethod)
	fmt.Printf("MaxActivePaths: %d\n", *maxActivePaths)
	fmt.Printf("BlankPenalty: %.2f\n", *blankPenalty)
	fmt.Printf("NumThreads: %d\n", *numThreads)
	if *tempo != 1.0 {
		fmt.Printf("Tempo: %.2f (timestamps corrected by %.2fx)\n", *tempo, *tempo)
	}
	fmt.Println()

	// If tempo=0.95, audio is slower, timestamps need to be multiplied by 0.95 to get original time
	tempoFactor := *tempo

	// Create ASR config with precision parameters
	config := sherpa.OfflineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{
			SampleRate: sampleRate,
			FeatureDim: 80,
		},
		ModelConfig: sherpa.OfflineModelConfig{
			Transducer: sherpa.OfflineTransducerModelConfig{
				Encoder: *modelDir + "/encoder-epoch-99-avg-1.onnx",
				Decoder: *modelDir + "/decoder-epoch-99-avg-1.onnx",
				Joiner:  *modelDir + "/joiner-epoch-99-avg-1.onnx",
			},
			Tokens:     *modelDir + "/tokens.txt",
			NumThreads: *numThreads,
			Debug:      0,
		},
		DecodingMethod: *decodingMethod,
		MaxActivePaths: *maxActivePaths,
		BlankPenalty:   float32(*blankPenalty),
	}

	recognizer := sherpa.NewOfflineRecognizer(&config)
	if recognizer == nil {
		log.Fatal("Failed to create recognizer")
	}
	defer sherpa.DeleteOfflineRecognizer(recognizer)

	fmt.Println("Recognizer initialized")

	// Optional VAD
	var vad *sherpa.VoiceActivityDetector
	if *vadModel != "" {
		if _, err := os.Stat(*vadModel); os.IsNotExist(err) {
			log.Fatalf("VAD model not found: %s", *vadModel)
		}
		vadConfig := sherpa.VadModelConfig{
			SileroVad: sherpa.SileroVadModelConfig{
				Model:              *vadModel,
				Threshold:          float32(*vadThreshold),
				MinSilenceDuration: 0.5,
				MinSpeechDuration:  0.25,
				WindowSize:         512,
			},
			SampleRate: sampleRate,
			NumThreads: 1,
		}
		vad = sherpa.NewVoiceActivityDetector(&vadConfig, 30)
		if vad == nil {
			log.Fatal("Failed to create VAD")
		}
		defer sherpa.DeleteVoiceActivityDetector(vad)
		fmt.Println("VAD enabled")
	}

	// Get duration
	duration, _ := getAudioDuration(*inputPath)
	fmt.Printf("Audio duration: %.1fs\n\n", duration)

	// Start ffmpeg
	var cmd *exec.Cmd
	if *tempo != 1.0 {
		cmd = exec.Command("ffmpeg",
			"-i", *inputPath,
			"-af", fmt.Sprintf("atempo=%.2f", *tempo),
			"-f", "s16le",
			"-acodec", "pcm_s16le",
			"-ar", fmt.Sprintf("%d", sampleRate),
			"-ac", "1",
			"-loglevel", "error",
			"pipe:1",
		)
	} else {
		cmd = exec.Command("ffmpeg",
			"-i", *inputPath,
			"-f", "s16le",
			"-acodec", "pcm_s16le",
			"-ar", fmt.Sprintf("%d", sampleRate),
			"-ac", "1",
			"-loglevel", "error",
			"pipe:1",
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Failed to get stdout pipe: %v", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatalf("Failed to start ffmpeg: %v", err)
	}

	reader := bufio.NewReader(stdout)
	startTime := time.Now()
	var allText string
	var processedSamples int64

	if vad != nil {
		// VAD mode
		windowSize := 512
		windowBytes := windowSize * 2

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

			for !vad.IsEmpty() {
				segment := vad.Front()
				vad.Pop()

				startSec := float64(segment.Start) / float64(sampleRate)
				endSec := float64(segment.Start+len(segment.Samples)) / float64(sampleRate)

				result := transcribeBytes(recognizer, segment.Samples)
				if result != "" {
					fmt.Printf("[%.2f-%.2f] %s\n", startSec, endSec, result)
					allText += result
				}
			}

			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
		}

		vad.Flush()
		for !vad.IsEmpty() {
			segment := vad.Front()
			vad.Pop()
			result := transcribeBytes(recognizer, segment.Samples)
			if result != "" {
				allText += result
			}
		}
	} else {
		// No VAD - chunk mode
		chunkSamples := sampleRate * *chunkSec
		chunkBytes := chunkSamples * 2

		fmt.Printf("Processing in %d-second chunks...\n", *chunkSec)
		chunkNum := 0

		for {
			buffer := make([]byte, chunkBytes)
			n, err := io.ReadFull(reader, buffer)
			if n == 0 {
				break
			}

			samples := bytesToFloat32(buffer[:n])
			processedSamples += int64(len(samples))
			chunkNum++

			// Raw time in slowed audio
			rawStartSec := float64(chunkNum-1) * float64(*chunkSec)
			rawEndSec := rawStartSec + float64(*chunkSec)

			// Corrected time in original audio
			startSec := rawStartSec * tempoFactor
			endSec := rawEndSec * tempoFactor

			result := transcribeBytes(recognizer, samples)
			if result != "" {
				fmt.Printf("[%.1f-%.1fs] %s\n", startSec, endSec, result)
				allText += result
			}

			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
		}
	}

	cmd.Wait()

	// Summary
	elapsed := time.Since(startTime).Seconds()
	fmt.Printf("\n=== Summary ===\n")
	fmt.Printf("Processing time: %.1fs\n", elapsed)
	fmt.Printf("Real-time factor: %.2fx\n", duration/elapsed)
	fmt.Printf("Text length: %d chars\n", len(allText))
	fmt.Printf("\n=== Full Transcript ===\n%s\n", allText)
}

func transcribeBytes(recognizer *sherpa.OfflineRecognizer, samples []float32) string {
	stream := sherpa.NewOfflineStream(recognizer)
	defer sherpa.DeleteOfflineStream(stream)

	stream.AcceptWaveform(sampleRate, samples)
	recognizer.Decode(stream)

	result := stream.GetResult()
	return result.Text
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
