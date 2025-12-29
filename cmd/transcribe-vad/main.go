// Experiment: VAD + ASR transcription comparison tool
// Compares three methods: vad-block, vad-stream, chunk
//
// Usage:
//   go run ./cmd/transcribe-vad -i audio.mp3 -method vad-block
//   go run ./cmd/transcribe-vad -i audio.mp3 -method vad-stream
//   go run ./cmd/transcribe-vad -i audio.mp3 -method chunk

package main

import (
	"flag"
	"fmt"
	"os"

	"zbor/internal/asr"
)

func main() {
	var (
		inputFile      = flag.String("i", "", "Input audio file")
		outputFile     = flag.String("o", "", "Output file (default: stdout)")
		format         = flag.String("format", "text", "Output format: text, json, srt")
		modelDir       = flag.String("model", "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01", "Model directory path")
		vadModelPath   = flag.String("vad", "models/silero_vad.onnx", "VAD model path")
		vadThreshold   = flag.Float64("vad-threshold", 0.5, "VAD speech threshold (0-1, lower = more sensitive)")
		silenceThresh  = flag.Float64("silence-threshold", 0.001, "Silence detection RMS threshold (0-1, lower = more sensitive)")
		minSilence     = flag.Float64("min-silence", 0.5, "Min silence duration to split blocks (seconds)")
		maxBlock       = flag.Float64("max-block", 5.0, "Max block duration before splitting (seconds, 0=no split)")
		overlap        = flag.Float64("overlap", 0.5, "Overlap duration for overlap method (seconds)")
		tempo          = flag.Float64("tempo", 0.95, "Audio tempo (0.5-1.0, lower = slower for fast speech)")
		numThreads     = flag.Int("threads", 4, "Number of threads for inference")
		method         = flag.String("method", "vad-block", "Method: vad-block, vad-stream, chunk")
		decodingMethod = flag.String("decoding", "greedy_search", "Decoding method: greedy_search or modified_beam_search")
		maxActivePaths = flag.Int("max-paths", 4, "Max active paths for modified_beam_search")
		verbose        = flag.Bool("v", false, "Verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "VAD-based transcription experiment tool\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nMethods:\n")
		fmt.Fprintf(os.Stderr, "  vad-block   VAD detects speech blocks, each block transcribed with tempo (recommended)\n")
		fmt.Fprintf(os.Stderr, "  vad-stream  VAD streaming (existing, no tempo adjustment)\n")
		fmt.Fprintf(os.Stderr, "  chunk       Fixed chunk-based with tempo (existing)\n")
		fmt.Fprintf(os.Stderr, "  silence     Energy-based silence detection (more sensitive than VAD)\n")
		fmt.Fprintf(os.Stderr, "  overlap     Silence detection with overlapping chunks (best for continuous speech)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -i audio.wav -method vad-block -tempo 0.9\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i audio.wav -method chunk -tempo 0.95\n", os.Args[0])
	}

	flag.Parse()

	if *inputFile == "" {
		fmt.Fprintf(os.Stderr, "Error: Input file is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if _, err := os.Stat(*inputFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Input file not found: %s\n", *inputFile)
		os.Exit(1)
	}

	if *format != "text" && *format != "json" && *format != "srt" {
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Must be: text, json, or srt\n", *format)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Method: %s\n", *method)
		fmt.Fprintf(os.Stderr, "Tempo: %.2f\n", *tempo)
		fmt.Fprintf(os.Stderr, "Loading model from: %s\n", *modelDir)
	}

	// Create configuration
	config, err := asr.NewConfig(*modelDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load model config: %v\n", err)
		os.Exit(1)
	}
	config.NumThreads = *numThreads
	config.DecodingMethod = *decodingMethod
	config.MaxActivePaths = *maxActivePaths

	// Create recognizer
	recognizer, err := asr.NewRecognizer(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create recognizer: %v\n", err)
		os.Exit(1)
	}
	defer recognizer.Close()

	// Progress callback
	progressCallback := func(progress int, step string) {
		if *verbose {
			fmt.Fprintf(os.Stderr, "\r[%3d%%] %s", progress, step)
		}
	}

	var result *asr.Result

	switch *method {
	case "vad-block":
		// New VAD + block-based method with tempo
		vadConfig := asr.DefaultVADConfig(*vadModelPath)
		vadConfig.Threshold = float32(*vadThreshold)
		vadConfig.MinSilenceDuration = float32(*minSilence)
		vadConfig.MaxBlockDuration = *maxBlock
		if *verbose {
			fmt.Fprintf(os.Stderr, "Using VAD+block method with tempo=%.2f, vad-threshold=%.2f, min-silence=%.2f, max-block=%.2f\n", *tempo, *vadThreshold, *minSilence, *maxBlock)
		}
		result, err = recognizer.TranscribeWithVADBlock(*inputFile, vadConfig, *tempo, progressCallback)

	case "vad-stream":
		// Existing VAD streaming method (no tempo)
		vadConfig := asr.DefaultVADConfig(*vadModelPath)
		vadConfig.Threshold = float32(*vadThreshold)
		vadConfig.MinSilenceDuration = float32(*minSilence)
		if *verbose {
			fmt.Fprintf(os.Stderr, "Using VAD streaming method (no tempo adjustment), vad-threshold=%.2f, min-silence=%.2f\n", *vadThreshold, *minSilence)
		}
		result, err = recognizer.TranscribeWithVAD(*inputFile, vadConfig, progressCallback)

	case "chunk":
		// Existing chunk-based method with tempo
		if *verbose {
			fmt.Fprintf(os.Stderr, "Using chunk method with tempo=%.2f\n", *tempo)
		}
		result, err = recognizer.TranscribeWithTempo(*inputFile, *tempo, 20, progressCallback)

	case "silence":
		// Energy-based silence detection (more sensitive than VAD)
		silenceConfig := asr.DefaultSilenceConfig()
		silenceConfig.SilenceThreshold = *silenceThresh
		silenceConfig.MinSilenceDuration = *minSilence
		silenceConfig.MaxBlockDuration = *maxBlock
		if *verbose {
			fmt.Fprintf(os.Stderr, "Using silence detection method with tempo=%.2f, threshold=%.6f, min-silence=%.2f, max-block=%.2f\n",
				*tempo, silenceConfig.SilenceThreshold, *minSilence, *maxBlock)
		}
		result, err = recognizer.TranscribeWithSilenceDetection(*inputFile, silenceConfig, *tempo, progressCallback)

	case "overlap":
		// Silence detection with overlapping chunks
		silenceConfig := asr.DefaultSilenceConfig()
		silenceConfig.SilenceThreshold = *silenceThresh
		silenceConfig.MinSilenceDuration = *minSilence
		silenceConfig.MaxBlockDuration = *maxBlock
		if *verbose {
			fmt.Fprintf(os.Stderr, "Using overlap method with tempo=%.2f, threshold=%.6f, max-block=%.2f, overlap=%.2f\n",
				*tempo, silenceConfig.SilenceThreshold, *maxBlock, *overlap)
		}
		result, err = recognizer.TranscribeWithOverlap(*inputFile, silenceConfig, *tempo, *overlap, progressCallback)

	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown method '%s'\n", *method)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "\n")
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Transcription failed: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Tokens: %d, Segments: %d, Duration: %.2fs\n",
			len(result.Tokens), len(result.Segments), result.TotalDuration)
	}

	// Format output
	var output string
	switch *format {
	case "json":
		output, err = result.FormatAsJSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to format JSON: %v\n", err)
			os.Exit(1)
		}
	case "srt":
		output = result.FormatAsSRT()
	default:
		output = result.FormatAsText()
	}

	// Write output
	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, []byte(output), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to write output file: %v\n", err)
			os.Exit(1)
		}
		if *verbose {
			fmt.Fprintf(os.Stderr, "Output written to: %s\n", *outputFile)
		}
	} else {
		fmt.Println(output)
	}
}
