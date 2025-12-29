package asr

import (
	"fmt"
	"strings"
)

// DisplayElement represents a single element in the timeline display
type DisplayElement struct {
	Type      string  `json:"type"`       // "text" or "silence"
	Text      string  `json:"text"`       // Text content (for type=text) or dots (for type=silence)
	StartTime float64 `json:"start_time"` // Start time in seconds
	Duration  float64 `json:"duration"`   // Duration in seconds
}

// DisplaySegment represents a fixed-interval segment for display
type DisplaySegment struct {
	Index       int              `json:"index"`        // Segment index (0-based)
	StartTime   float64          `json:"start_time"`   // Segment start time
	EndTime     float64          `json:"end_time"`     // Segment end time
	Elements    []DisplayElement `json:"elements"`     // Display elements (text and silence)
	ASRSegments []SegmentInfo    `json:"asr_segments"` // Original ASR segment info within this display segment
}

// SegmentInfo contains info about original ASR segments for display
type SegmentInfo struct {
	Index     int     `json:"index"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// GenerateDisplaySegments creates fixed-interval display segments from tokens
// intervalSec: display segment interval (e.g., 10 seconds)
// silenceThreshold: minimum gap to consider as silence (e.g., 0.3 seconds)
// dotsPerSecond: number of dots per second of silence (e.g., 5)
func GenerateDisplaySegments(tokens []Token, segments []Segment, totalDuration float64, intervalSec float64, silenceThreshold float64, dotsPerSecond float64) []DisplaySegment {
	if intervalSec <= 0 {
		intervalSec = 10.0
	}
	if silenceThreshold <= 0 {
		silenceThreshold = 0.3
	}
	if dotsPerSecond <= 0 {
		dotsPerSecond = 5.0
	}

	// Calculate number of display segments
	numSegments := int(totalDuration/intervalSec) + 1
	if totalDuration <= 0 && len(tokens) > 0 {
		// Estimate from last token
		lastToken := tokens[len(tokens)-1]
		totalDuration = float64(lastToken.StartTime + lastToken.Duration)
		numSegments = int(totalDuration/intervalSec) + 1
	}

	displaySegments := make([]DisplaySegment, numSegments)

	// Initialize display segments
	for i := 0; i < numSegments; i++ {
		displaySegments[i] = DisplaySegment{
			Index:       i,
			StartTime:   float64(i) * intervalSec,
			EndTime:     float64(i+1) * intervalSec,
			Elements:    []DisplayElement{},
			ASRSegments: []SegmentInfo{},
		}
	}

	// Map ASR segments to display segments
	for segIdx, seg := range segments {
		for i := range displaySegments {
			ds := &displaySegments[i]
			// Check if ASR segment overlaps with this display segment
			if seg.StartTime < ds.EndTime && seg.EndTime > ds.StartTime {
				ds.ASRSegments = append(ds.ASRSegments, SegmentInfo{
					Index:     segIdx + 1,
					StartTime: seg.StartTime,
					EndTime:   seg.EndTime,
				})
			}
		}
	}

	// Process tokens into display segments
	var lastEndTime float64 = 0

	for _, token := range tokens {
		tokenStart := float64(token.StartTime)
		tokenEnd := tokenStart + float64(token.Duration)
		if token.Duration == 0 {
			tokenEnd = tokenStart + 0.1 // Assume minimum duration
		}

		// Find which display segment this token belongs to
		segIdx := int(tokenStart / intervalSec)
		if segIdx >= numSegments {
			segIdx = numSegments - 1
		}
		if segIdx < 0 {
			segIdx = 0
		}

		ds := &displaySegments[segIdx]

		// Check for silence gap before this token
		gap := tokenStart - lastEndTime
		if gap >= silenceThreshold && lastEndTime > 0 {
			// Add silence to appropriate segment(s)
			addSilence(&displaySegments, lastEndTime, tokenStart, intervalSec, dotsPerSecond)
		}

		// Add text element
		ds.Elements = append(ds.Elements, DisplayElement{
			Type:      "text",
			Text:      token.Text,
			StartTime: tokenStart,
			Duration:  float64(token.Duration),
		})

		lastEndTime = tokenEnd
	}

	// Add trailing silence if needed
	if lastEndTime < totalDuration {
		addSilence(&displaySegments, lastEndTime, totalDuration, intervalSec, dotsPerSecond)
	}

	return displaySegments
}

// addSilence adds silence markers to the appropriate display segments
func addSilence(displaySegments *[]DisplaySegment, startTime, endTime, intervalSec, dotsPerSecond float64) {
	duration := endTime - startTime
	numDots := int(duration * dotsPerSecond)
	if numDots < 1 {
		numDots = 1
	}
	if numDots > 20 { // Cap at 20 dots per silence
		numDots = 20
	}

	dots := strings.Repeat("・", numDots)

	// Find starting segment
	startSegIdx := int(startTime / intervalSec)
	if startSegIdx >= len(*displaySegments) {
		startSegIdx = len(*displaySegments) - 1
	}
	if startSegIdx < 0 {
		return
	}

	(*displaySegments)[startSegIdx].Elements = append((*displaySegments)[startSegIdx].Elements, DisplayElement{
		Type:      "silence",
		Text:      dots,
		StartTime: startTime,
		Duration:  duration,
	})
}

// FormatTimeRange formats a time range as MM:SS-MM:SS
func FormatTimeRange(startSec, endSec float64) string {
	return fmt.Sprintf("%s-%s", FormatTime(startSec), FormatTime(endSec))
}

// FormatTime formats seconds as MM:SS
func FormatTime(seconds float64) string {
	mins := int(seconds) / 60
	secs := int(seconds) % 60
	return fmt.Sprintf("%02d:%02d", mins, secs)
}
