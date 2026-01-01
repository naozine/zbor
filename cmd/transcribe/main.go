package main

import (
	"flag"
	"fmt"
	"os"

	"zbor/internal/asr"
)

func main() {
	// Define flags
	var (
		inputFile  = flag.String("i", "", "Input audio file (WAV format)")
		outputFile = flag.String("o", "", "Output file (default: stdout)")
		format     = flag.String("format", "text", "Output format: text, json, srt")
		modelDir   = flag.String("model", "models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01", "Model directory path")
		numThreads = flag.Int("threads", 2, "Number of threads for inference")
		verbose    = flag.Bool("v", false, "Verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -i audio.wav\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i audio.wav -o output.txt\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i audio.wav -format json -o output.json\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -i audio.wav -format srt -o subtitles.srt\n", os.Args[0])
	}

	flag.Parse()

	// Validate input
	if *inputFile == "" {
		fmt.Fprintf(os.Stderr, "Error: Input file is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Check if input file exists
	if _, err := os.Stat(*inputFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Input file not found: %s\n", *inputFile)
		os.Exit(1)
	}

	// Validate format
	if *format != "text" && *format != "json" && *format != "srt" {
		fmt.Fprintf(os.Stderr, "Error: Invalid format '%s'. Must be: text, json, or srt\n", *format)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Loading model from: %s\n", *modelDir)
	}

	// Create configuration
	config, err := asr.NewConfig(*modelDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load model config: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nHint: Download the model first:\n")
		fmt.Fprintf(os.Stderr, "  curl -SL -O https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01.tar.bz2\n")
		fmt.Fprintf(os.Stderr, "  tar xvf sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01.tar.bz2 -C models/\n")
		os.Exit(1)
	}
	config.NumThreads = *numThreads

	if *verbose {
		fmt.Fprintf(os.Stderr, "Creating recognizer...\n")
	}

	// Create recognizer
	recognizer, err := asr.NewRecognizer(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create recognizer: %v\n", err)
		os.Exit(1)
	}
	defer recognizer.Close()

	if *verbose {
		fmt.Fprintf(os.Stderr, "Transcribing: %s\n", *inputFile)
	}

	// Transcribe
	result, err := recognizer.TranscribeFile(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Transcription failed: %v\n", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Transcription completed in %.2f seconds\n", result.Duration)
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
	default: // text
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
