package asr

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SupportedFormats lists audio formats that can be converted
var SupportedFormats = []string{".mp3", ".m4a", ".aac", ".ogg", ".flac", ".wav", ".webm", ".opus"}

// IsSupportedFormat checks if the file extension is a supported audio format
func IsSupportedFormat(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, format := range SupportedFormats {
		if ext == format {
			return true
		}
	}
	return false
}

// ConvertToWav converts an audio file to WAV format (16kHz, mono)
// Returns the path to the converted file
func ConvertToWav(inputPath, outputPath string) error {
	// Check if ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg not found: please install ffmpeg to convert audio files")
	}

	// Check if input file exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return fmt.Errorf("input file not found: %s", inputPath)
	}

	// Create output directory if needed
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Run ffmpeg conversion
	// -i: input file
	// -ar 16000: sample rate 16kHz
	// -ac 1: mono channel
	// -f wav: output format
	// -y: overwrite output file
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-ar", "16000",
		"-ac", "1",
		"-f", "wav",
		"-y",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg conversion failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// ConvertToWavTemp converts an audio file to WAV format in a temp directory
// Returns the path to the converted file (caller should clean up)
func ConvertToWavTemp(inputPath string) (string, error) {
	// Create temp file for output
	tempDir := os.TempDir()
	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(tempDir, baseName+"_converted.wav")

	if err := ConvertToWav(inputPath, outputPath); err != nil {
		return "", err
	}

	return outputPath, nil
}

// NeedsConversion checks if the file needs to be converted
// WAV files at 16kHz mono don't need conversion
func NeedsConversion(inputPath string) (bool, error) {
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext != ".wav" {
		return true, nil
	}

	// For WAV files, check if they're already 16kHz mono
	// Use ffprobe to check audio properties
	if _, err := exec.LookPath("ffprobe"); err != nil {
		// If ffprobe is not available, assume conversion is needed
		return true, nil
	}

	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=sample_rate,channels",
		"-of", "csv=p=0",
		inputPath,
	)

	output, err := cmd.Output()
	if err != nil {
		// If ffprobe fails, assume conversion is needed
		return true, nil
	}

	// Parse output: "sample_rate,channels"
	parts := strings.Split(strings.TrimSpace(string(output)), ",")
	if len(parts) != 2 {
		return true, nil
	}

	// Check if it's 16000Hz and 1 channel
	if parts[0] == "16000" && parts[1] == "1" {
		return false, nil
	}

	return true, nil
}

// GetAudioDuration returns the duration of an audio file in seconds
func GetAudioDuration(inputPath string) (float64, error) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return 0, fmt.Errorf("ffprobe not found: please install ffmpeg")
	}

	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		inputPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get audio duration: %w", err)
	}

	var duration float64
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%f", &duration)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}
