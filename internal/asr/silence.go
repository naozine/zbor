package asr

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
)

// SilenceConfig holds configuration for silence-based speech detection
type SilenceConfig struct {
	// SilenceThreshold is the RMS level below which audio is considered silence (0.0-1.0)
	// Lower values = more sensitive (default: 0.01)
	SilenceThreshold float64

	// MinSilenceDuration is the minimum silence duration to split blocks (seconds)
	MinSilenceDuration float64

	// MinSpeechDuration is the minimum speech duration to keep a block (seconds)
	MinSpeechDuration float64

	// MaxBlockDuration is the maximum block duration before forced split (seconds)
	MaxBlockDuration float64

	// FrameSize is the number of samples per frame for RMS calculation
	FrameSize int
}

// DefaultSilenceConfig returns default configuration for silence detection
func DefaultSilenceConfig() *SilenceConfig {
	return &SilenceConfig{
		SilenceThreshold:   0.01,  // RMS threshold (quite sensitive)
		MinSilenceDuration: 0.3,   // 300ms silence to split
		MinSpeechDuration:  0.1,   // 100ms minimum speech
		MaxBlockDuration:   5.0,   // 5 second max blocks
		FrameSize:          480,   // 30ms at 16kHz
	}
}

// detectSpeechBlocksBySilence detects speech blocks using energy-based silence detection
func (r *Recognizer) detectSpeechBlocksBySilence(inputPath string, config *SilenceConfig) ([]SpeechBlock, error) {
	if config == nil {
		config = DefaultSilenceConfig()
	}

	sampleRate := r.config.SampleRate

	// Convert audio to raw PCM using ffmpeg
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ar", fmt.Sprintf("%d", sampleRate),
		"-ac", "1",
		"-",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create pipe: %w", err)
	}
	cmd.Stderr = nil // Suppress ffmpeg output

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	reader := bufio.NewReader(stdout)

	// Read samples and calculate RMS for each frame
	var frames []float64 // RMS values for each frame
	frameSamples := make([]float32, 0, config.FrameSize)

	buf := make([]byte, 2) // 16-bit samples
	for {
		_, err := io.ReadFull(reader, buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			cmd.Wait()
			return nil, fmt.Errorf("failed to read audio: %w", err)
		}

		// Convert to float32 (-1.0 to 1.0)
		sample := float32(int16(binary.LittleEndian.Uint16(buf))) / 32768.0
		frameSamples = append(frameSamples, sample)

		// Calculate RMS when frame is complete
		if len(frameSamples) >= config.FrameSize {
			rms := calculateRMS(frameSamples)
			frames = append(frames, rms)
			frameSamples = frameSamples[:0]
		}
	}

	// Process remaining samples
	if len(frameSamples) > 0 {
		rms := calculateRMS(frameSamples)
		frames = append(frames, rms)
	}

	cmd.Wait()

	if len(frames) == 0 {
		return nil, nil
	}

	// Convert frames to speech blocks
	frameDuration := float64(config.FrameSize) / float64(sampleRate)

	minSilenceFrames := int(config.MinSilenceDuration / frameDuration)
	minSpeechFrames := int(config.MinSpeechDuration / frameDuration)

	var blocks []SpeechBlock
	inSpeech := false
	speechStart := 0
	silenceCount := 0

	for i, rms := range frames {
		isSilent := rms < config.SilenceThreshold

		if !inSpeech {
			if !isSilent {
				// Start of speech
				inSpeech = true
				speechStart = i
				silenceCount = 0
			}
		} else {
			if isSilent {
				silenceCount++
				if silenceCount >= minSilenceFrames {
					// End of speech (silence gap detected)
					speechEnd := i - silenceCount + 1
					if speechEnd-speechStart >= minSpeechFrames {
						blocks = append(blocks, SpeechBlock{
							StartTime: float64(speechStart) * frameDuration,
							EndTime:   float64(speechEnd) * frameDuration,
						})
					}
					inSpeech = false
					silenceCount = 0
				}
			} else {
				silenceCount = 0
			}
		}
	}

	// Handle speech at end of audio
	if inSpeech {
		speechEnd := len(frames)
		if speechEnd-speechStart >= minSpeechFrames {
			blocks = append(blocks, SpeechBlock{
				StartTime: float64(speechStart) * frameDuration,
				EndTime:   float64(speechEnd) * frameDuration,
			})
		}
	}

	// Split long blocks
	blocks = splitLongBlocks(blocks, config.MaxBlockDuration)

	return blocks, nil
}

// calculateRMS calculates the root mean square of samples
func calculateRMS(samples []float32) float64 {
	if len(samples) == 0 {
		return 0
	}

	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}

	return math.Sqrt(sum / float64(len(samples)))
}

// OverlapBlock represents a block with overlap information for context-aware recognition
type OverlapBlock struct {
	SpeechBlock
	MainStart float64 // Start of the "main" portion (after overlap)
	MainEnd   float64 // End of the "main" portion (before overlap)
}

// splitLongBlocksWithOverlap splits blocks with overlap for context
// overlap is the amount of overlap in seconds (e.g., 0.5s)
func splitLongBlocksWithOverlap(blocks []SpeechBlock, maxDuration float64, overlap float64) []OverlapBlock {
	if maxDuration <= 0 {
		maxDuration = 2.0
	}
	if overlap <= 0 {
		overlap = 0.5
	}

	var result []OverlapBlock
	for _, block := range blocks {
		duration := block.EndTime - block.StartTime
		if duration <= maxDuration {
			// No split needed
			result = append(result, OverlapBlock{
				SpeechBlock: block,
				MainStart:   block.StartTime,
				MainEnd:     block.EndTime,
			})
			continue
		}

		// Split into chunks with overlap
		// Each chunk: [start, start+maxDuration]
		// Main portion: [mainStart, mainEnd] where results are kept
		// Next chunk starts at mainEnd (not start+maxDuration)
		mainDuration := maxDuration - overlap
		start := block.StartTime

		for start < block.EndTime {
			end := start + maxDuration
			if end > block.EndTime {
				end = block.EndTime
			}

			mainStart := start
			mainEnd := start + mainDuration
			if mainEnd > block.EndTime {
				mainEnd = block.EndTime
			}

			// For the first chunk, mainStart is the actual start
			// For subsequent chunks, we already have overlap from previous
			result = append(result, OverlapBlock{
				SpeechBlock: SpeechBlock{
					StartTime: start,
					EndTime:   end,
				},
				MainStart: mainStart,
				MainEnd:   mainEnd,
			})

			// Next chunk starts at mainEnd (creating overlap)
			start = mainEnd
		}
	}
	return result
}

// TranscribeWithSilenceDetection transcribes audio using energy-based silence detection
// This is an alternative to VAD that detects any sound (not just voice)
func (r *Recognizer) TranscribeWithSilenceDetection(inputPath string, config *SilenceConfig, tempo float64, onProgress ProgressCallback) (*Result, error) {
	if tempo <= 0 {
		tempo = 1.0
	}
	if config == nil {
		config = DefaultSilenceConfig()
	}

	// Step 1: Detect speech blocks using silence detection
	if onProgress != nil {
		onProgress(10, "detecting speech")
	}

	blocks, err := r.detectSpeechBlocksBySilence(inputPath, config)
	if err != nil {
		return nil, fmt.Errorf("silence detection failed: %w", err)
	}

	if len(blocks) == 0 {
		return &Result{
			Text:     "",
			Tokens:   []Token{},
			Segments: []Segment{},
		}, nil
	}

	// If first detected block starts late, extend it to start from 0
	// This ensures quiet speech at the beginning is not lost
	if len(blocks) > 0 && blocks[0].StartTime > 0.5 {
		blocks[0].StartTime = 0
	}

	// Split long blocks
	blocks = splitLongBlocks(blocks, config.MaxBlockDuration)

	// Debug: print detected blocks
	for i, b := range blocks {
		fmt.Fprintf(os.Stderr, "  Block %d: %.2f - %.2f (%.2fs)\n", i+1, b.StartTime, b.EndTime, b.EndTime-b.StartTime)
	}

	if onProgress != nil {
		onProgress(20, fmt.Sprintf("found %d blocks", len(blocks)))
	}

	// Step 2: Process each block (reuse transcribeBlock from vad_block.go)
	var allTokens []Token
	var allText string

	for i, block := range blocks {
		if onProgress != nil {
			progress := 20 + int(60*float64(i)/float64(len(blocks)))
			onProgress(progress, fmt.Sprintf("transcribing block %d/%d", i+1, len(blocks)))
		}

		tokens, text, err := r.transcribeBlock(inputPath, block, tempo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to transcribe block %d: %v\n", i+1, err)
			continue
		}

		allTokens = append(allTokens, tokens...)
		allText += text
	}

	if onProgress != nil {
		onProgress(90, "finalizing")
	}

	// Calculate total duration
	var totalDuration float32
	if len(allTokens) > 0 {
		lastToken := allTokens[len(allTokens)-1]
		totalDuration = lastToken.StartTime + lastToken.Duration
	}

	return &Result{
		Text:          allText,
		Tokens:        allTokens,
		Segments:      tokensToSegments(allTokens),
		TotalDuration: totalDuration,
	}, nil
}

// TranscribeWithOverlap transcribes audio using overlapping chunks
// This method helps with continuous speech that might get cut at word boundaries
// overlap is the amount of overlap in seconds (default: 0.5s)
func (r *Recognizer) TranscribeWithOverlap(inputPath string, config *SilenceConfig, tempo float64, overlap float64, onProgress ProgressCallback) (*Result, error) {
	if tempo <= 0 {
		tempo = 1.0
	}
	if config == nil {
		config = DefaultSilenceConfig()
	}
	if overlap <= 0 {
		overlap = 0.5
	}

	// Step 1: Detect speech blocks using silence detection
	if onProgress != nil {
		onProgress(10, "detecting speech")
	}

	blocks, err := r.detectSpeechBlocksBySilence(inputPath, config)
	if err != nil {
		return nil, fmt.Errorf("silence detection failed: %w", err)
	}

	if len(blocks) == 0 {
		return &Result{
			Text:     "",
			Tokens:   []Token{},
			Segments: []Segment{},
		}, nil
	}

	// If first detected block starts late, extend it to start from 0
	if len(blocks) > 0 && blocks[0].StartTime > 0.5 {
		blocks[0].StartTime = 0
	}

	// Split long blocks WITH OVERLAP
	overlapBlocks := splitLongBlocksWithOverlap(blocks, config.MaxBlockDuration, overlap)

	// Debug: print detected blocks
	for i, b := range overlapBlocks {
		fmt.Fprintf(os.Stderr, "  Block %d: %.2f - %.2f (main: %.2f - %.2f)\n",
			i+1, b.StartTime, b.EndTime, b.MainStart, b.MainEnd)
	}

	if onProgress != nil {
		onProgress(20, fmt.Sprintf("found %d blocks", len(overlapBlocks)))
	}

	// Step 2: Process each block, keeping only tokens in the "main" portion
	var allTokens []Token

	for i, block := range overlapBlocks {
		if onProgress != nil {
			progress := 20 + int(60*float64(i)/float64(len(overlapBlocks)))
			onProgress(progress, fmt.Sprintf("transcribing block %d/%d", i+1, len(overlapBlocks)))
		}

		tokens, _, err := r.transcribeBlock(inputPath, block.SpeechBlock, tempo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to transcribe block %d: %v\n", i+1, err)
			continue
		}

		// Filter tokens: only keep those in the "main" portion
		for _, token := range tokens {
			tokenTime := float64(token.StartTime)
			// Keep token if it starts within the main portion
			if tokenTime >= block.MainStart && tokenTime < block.MainEnd {
				allTokens = append(allTokens, token)
			}
		}
	}

	if onProgress != nil {
		onProgress(90, "finalizing")
	}

	// Rebuild text from tokens
	var textBuilder strings.Builder
	for _, token := range allTokens {
		textBuilder.WriteString(token.Text)
	}

	// Calculate total duration
	var totalDuration float32
	if len(allTokens) > 0 {
		lastToken := allTokens[len(allTokens)-1]
		totalDuration = lastToken.StartTime + lastToken.Duration
	}

	return &Result{
		Text:          textBuilder.String(),
		Tokens:        allTokens,
		Segments:      tokensToSegments(allTokens),
		TotalDuration: totalDuration,
	}, nil
}
