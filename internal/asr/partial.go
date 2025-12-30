package asr

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
)

// PartialTranscribeOptions contains options for partial transcription
type PartialTranscribeOptions struct {
	StartTime float64 // Start time in seconds
	EndTime   float64 // End time in seconds
	Tempo     float64 // Audio tempo (0.85-1.0, lower = slower)
	ChunkSec  int     // Chunk size in seconds (default 20)
}

// TranscribePartial transcribes a specific time range of an audio file
// Returns tokens with timestamps adjusted to the original audio time
func (r *Recognizer) TranscribePartial(filePath string, opts PartialTranscribeOptions) (*Result, error) {
	if opts.Tempo <= 0 {
		opts.Tempo = 0.95
	}
	if opts.ChunkSec <= 0 {
		opts.ChunkSec = 20
	}

	duration := opts.EndTime - opts.StartTime
	if duration <= 0 {
		return nil, fmt.Errorf("invalid time range: %.2f - %.2f", opts.StartTime, opts.EndTime)
	}

	// Build ffmpeg command to extract and process the time range
	// -ss: seek to start time
	// -t: duration to extract
	// -af atempo: adjust tempo
	args := []string{
		"-ss", fmt.Sprintf("%.3f", opts.StartTime),
		"-i", filePath,
		"-t", fmt.Sprintf("%.3f", duration),
	}

	// Add tempo filter if not 1.0
	if opts.Tempo != 1.0 {
		args = append(args, "-af", fmt.Sprintf("atempo=%.2f", opts.Tempo))
	}

	args = append(args,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ar", fmt.Sprintf("%d", r.config.SampleRate),
		"-ac", "1",
		"-loglevel", "error",
		"pipe:1",
	)

	cmd := exec.Command("ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Process audio in chunks
	reader := bufio.NewReader(stdout)
	chunkSamples := r.config.SampleRate * opts.ChunkSec
	chunkBytes := chunkSamples * 2 // 16-bit = 2 bytes per sample

	var allTokens []Token
	var allText string
	var processedSamples int64

	for {
		buffer := make([]byte, chunkBytes)
		n, err := io.ReadFull(reader, buffer)
		if n == 0 {
			break
		}

		samples := bytesToFloat32(buffer[:n])

		// Transcribe chunk
		result, err := r.TranscribeBytes(samples, r.config.SampleRate)
		if err != nil {
			cmd.Wait()
			return nil, fmt.Errorf("transcription failed: %w", err)
		}

		// Calculate time offset for this chunk (in slowed audio time)
		rawChunkOffset := float64(processedSamples) / float64(r.config.SampleRate)

		// Adjust token timestamps:
		// 1. Add chunk offset (in slowed audio time)
		// 2. Multiply by tempo to get slowed audio time
		// 3. Add original start time to get absolute time
		for _, token := range result.Tokens {
			adjustedToken := Token{
				Text:      token.Text,
				StartTime: float32(opts.StartTime + (rawChunkOffset+float64(token.StartTime))*opts.Tempo),
				Duration:  token.Duration * float32(opts.Tempo),
			}
			allTokens = append(allTokens, adjustedToken)
		}
		allText += result.Text

		processedSamples += int64(len(samples))

		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
	}

	cmd.Wait()

	return &Result{
		Text:   allText,
		Tokens: allTokens,
	}, nil
}

// MergeTokens replaces tokens in the specified time range with new tokens
// Original tokens outside the range are preserved
func MergeTokens(original []Token, replacement []Token, startTime, endTime float64) []Token {
	var result []Token

	// Add tokens before the replacement range
	for _, token := range original {
		if float64(token.StartTime) < startTime {
			result = append(result, token)
		}
	}

	// Add replacement tokens
	result = append(result, replacement...)

	// Add tokens after the replacement range
	for _, token := range original {
		if float64(token.StartTime) >= endTime {
			result = append(result, token)
		}
	}

	return result
}

// MergeSegments replaces segments in the specified index range with new segment info
// Preserves the original segment time boundaries and count to maintain SenseVoice's segmentation
func MergeSegments(original []Segment, startIdx, endIdx int, newTokens []Token) []Segment {
	var result []Segment

	// Keep segments before the replacement range
	for i := 0; i < startIdx && i < len(original); i++ {
		result = append(result, original[i])
	}

	// Distribute new tokens to original segments based on time overlap
	for i := startIdx; i <= endIdx && i < len(original); i++ {
		seg := original[i]

		// Find tokens that fall within this segment's time range
		var segText string
		for _, token := range newTokens {
			tokenStart := float64(token.StartTime)
			// Token belongs to this segment if its start time is within the segment
			if tokenStart >= seg.StartTime && tokenStart < seg.EndTime {
				segText += token.Text
			}
		}

		// Handle edge case: last segment should include tokens at exactly EndTime
		if i == endIdx {
			for _, token := range newTokens {
				tokenStart := float64(token.StartTime)
				if tokenStart >= seg.EndTime && tokenStart <= seg.EndTime+0.01 {
					segText += token.Text
				}
			}
		}

		// Create new segment with original boundaries but new text
		result = append(result, Segment{
			Text:      segText,
			StartTime: seg.StartTime,
			EndTime:   seg.EndTime,
		})
	}

	// Keep segments after the replacement range
	for i := endIdx + 1; i < len(original); i++ {
		result = append(result, original[i])
	}

	return result
}

// RebuildTextFromTokens rebuilds the full text from tokens
func RebuildTextFromTokens(tokens []Token) string {
	var text string
	for _, token := range tokens {
		text += token.Text
	}
	return text
}

// MergeSegmentsByRatio distributes tokens to segments based on segment duration ratio
// This is useful for Whisper which returns uniformly distributed timestamps that don't
// align with segment boundaries (especially when there are gaps between segments)
func MergeSegmentsByRatio(original []Segment, startIdx, endIdx int, newTokens []Token) []Segment {
	var result []Segment

	// Keep segments before the replacement range
	for i := 0; i < startIdx && i < len(original); i++ {
		result = append(result, original[i])
	}

	// Calculate total duration of target segments (excluding gaps)
	var totalDuration float64
	for i := startIdx; i <= endIdx && i < len(original); i++ {
		seg := original[i]
		duration := seg.EndTime - seg.StartTime
		if duration > 0 {
			totalDuration += duration
		}
	}

	// Distribute tokens proportionally to segment duration
	tokenIndex := 0
	for i := startIdx; i <= endIdx && i < len(original); i++ {
		seg := original[i]
		duration := seg.EndTime - seg.StartTime

		var segText string
		var segTokens []Token

		if totalDuration > 0 && duration > 0 {
			// Calculate number of tokens for this segment based on duration ratio
			ratio := duration / totalDuration
			tokenCount := int(float64(len(newTokens)) * ratio)

			// Last segment gets all remaining tokens
			if i == endIdx {
				tokenCount = len(newTokens) - tokenIndex
			}

			// Assign tokens and adjust their timestamps to fit within segment
			for j := 0; j < tokenCount && tokenIndex < len(newTokens); j++ {
				token := newTokens[tokenIndex]
				segText += token.Text

				// Recalculate timestamp to fit within segment
				tokenRatio := float64(j) / float64(max(tokenCount, 1))
				adjustedToken := Token{
					Text:      token.Text,
					StartTime: float32(seg.StartTime + duration*tokenRatio),
					Duration:  float32(duration / float64(max(tokenCount, 1))),
				}
				segTokens = append(segTokens, adjustedToken)
				tokenIndex++
			}
		}

		result = append(result, Segment{
			Text:      segText,
			StartTime: seg.StartTime,
			EndTime:   seg.EndTime,
		})
	}

	// Keep segments after the replacement range
	for i := endIdx + 1; i < len(original); i++ {
		result = append(result, original[i])
	}

	return result
}

// MergeTokensBySegmentRatio redistributes tokens with adjusted timestamps based on segment boundaries
// Returns tokens with timestamps recalculated to fit within segment time ranges
func MergeTokensBySegmentRatio(original []Token, newTokens []Token, segments []Segment, startIdx, endIdx int, startTime, endTime float64) []Token {
	var result []Token

	// Add tokens before the replacement range
	for _, token := range original {
		if float64(token.StartTime) < startTime {
			result = append(result, token)
		}
	}

	// Calculate total duration of target segments (excluding gaps)
	var totalDuration float64
	for i := startIdx; i <= endIdx && i < len(segments); i++ {
		seg := segments[i]
		duration := seg.EndTime - seg.StartTime
		if duration > 0 {
			totalDuration += duration
		}
	}

	// Distribute and adjust tokens
	tokenIndex := 0
	for i := startIdx; i <= endIdx && i < len(segments); i++ {
		seg := segments[i]
		duration := seg.EndTime - seg.StartTime

		if totalDuration > 0 && duration > 0 {
			ratio := duration / totalDuration
			tokenCount := int(float64(len(newTokens)) * ratio)

			if i == endIdx {
				tokenCount = len(newTokens) - tokenIndex
			}

			for j := 0; j < tokenCount && tokenIndex < len(newTokens); j++ {
				token := newTokens[tokenIndex]
				tokenRatio := float64(j) / float64(max(tokenCount, 1))
				adjustedToken := Token{
					Text:      token.Text,
					StartTime: float32(seg.StartTime + duration*tokenRatio),
					Duration:  float32(duration / float64(max(tokenCount, 1))),
				}
				result = append(result, adjustedToken)
				tokenIndex++
			}
		}
	}

	// Add tokens after the replacement range
	for _, token := range original {
		if float64(token.StartTime) >= endTime {
			result = append(result, token)
		}
	}

	return result
}
