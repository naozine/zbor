package asr

import (
	"encoding/json"
	"fmt"
	"time"
)

// Segment represents a timestamped text segment in the transcription
type Segment struct {
	Text      string  `json:"text"`
	StartTime float64 `json:"start_time"` // in seconds
	EndTime   float64 `json:"end_time"`   // in seconds
}

// Result represents the complete transcription result
type Result struct {
	Text     string    `json:"text"`               // full transcription text
	Segments []Segment `json:"segments,omitempty"` // timestamped segments (if available)
	Duration float64   `json:"duration"`           // processing time in seconds
}

// FormatAsText returns the transcription as plain text
func (r *Result) FormatAsText() string {
	return r.Text
}

// FormatAsJSON returns the transcription as formatted JSON
func (r *Result) FormatAsJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// FormatAsSRT returns the transcription as SRT subtitle format
func (r *Result) FormatAsSRT() string {
	if len(r.Segments) == 0 {
		// If no segments available, create a single segment
		return formatSRTSegment(1, 0, 0, r.Text)
	}

	var srt string
	for i, seg := range r.Segments {
		srt += formatSRTSegment(i+1, seg.StartTime, seg.EndTime, seg.Text)
		if i < len(r.Segments)-1 {
			srt += "\n"
		}
	}
	return srt
}

// formatSRTSegment formats a single SRT subtitle entry
func formatSRTSegment(index int, startSec, endSec float64, text string) string {
	return fmt.Sprintf("%d\n%s --> %s\n%s\n",
		index,
		formatSRTTime(startSec),
		formatSRTTime(endSec),
		text,
	)
}

// formatSRTTime converts seconds to SRT time format (HH:MM:SS,mmm)
func formatSRTTime(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}
