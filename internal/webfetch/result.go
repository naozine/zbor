package webfetch

import (
	"encoding/json"
	"fmt"
	"time"
)

// Result はフェッチ結果
type Result struct {
	URL      string        `json:"url"`
	Content  string        `json:"content"`
	Duration time.Duration `json:"duration"`
}

// FormatAsText は結果をプレーンテキストで出力
func (r *Result) FormatAsText() string {
	return r.Content
}

// FormatAsJSON は結果をJSON形式で出力
func (r *Result) FormatAsJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}
