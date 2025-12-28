package youtube

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CaptionEntry は字幕の1エントリ
type CaptionEntry struct {
	StartTime time.Duration `json:"start_time"`
	Duration  time.Duration `json:"duration"`
	Text      string        `json:"text"`
}

// EndTime は終了時刻を返す
func (e *CaptionEntry) EndTime() time.Duration {
	return e.StartTime + e.Duration
}

// CaptionResult は字幕取得結果
type CaptionResult struct {
	LanguageCode string         `json:"language_code"`
	Entries      []CaptionEntry `json:"entries"`
}

// FormatAsText は字幕をプレーンテキストとして出力
func (r *CaptionResult) FormatAsText() string {
	var sb strings.Builder
	for _, entry := range r.Entries {
		sb.WriteString(entry.Text)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

// FormatAsJSON は字幕をJSON形式で出力
func (r *CaptionResult) FormatAsJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// FormatAsSRT は字幕をSRT形式で出力
func (r *CaptionResult) FormatAsSRT() string {
	var sb strings.Builder
	for i, entry := range r.Entries {
		sb.WriteString(fmt.Sprintf("%d\n", i+1))
		sb.WriteString(fmt.Sprintf("%s --> %s\n",
			formatSRTTime(entry.StartTime),
			formatSRTTime(entry.EndTime()),
		))
		sb.WriteString(entry.Text)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// FormatAsVTT は字幕をWebVTT形式で出力
func (r *CaptionResult) FormatAsVTT() string {
	var sb strings.Builder
	sb.WriteString("WEBVTT\n\n")
	for i, entry := range r.Entries {
		sb.WriteString(fmt.Sprintf("%d\n", i+1))
		sb.WriteString(fmt.Sprintf("%s --> %s\n",
			formatVTTTime(entry.StartTime),
			formatVTTTime(entry.EndTime()),
		))
		sb.WriteString(entry.Text)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// formatSRTTime はSRT形式のタイムスタンプを生成 (HH:MM:SS,mmm)
func formatSRTTime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// formatVTTTime はWebVTT形式のタイムスタンプを生成 (HH:MM:SS.mmm)
func formatVTTTime(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}
