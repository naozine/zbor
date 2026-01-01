package models

import (
	"encoding/json"
	"time"
)

// Source は入力された元データ
type Source struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	OriginalURL string    `json:"original_url,omitempty"`
	FilePath    string    `json:"file_path,omitempty"`
	Metadata    string    `json:"metadata,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	Status      string    `json:"status"`
}

// ソースステータス
const (
	SourceStatusPending    = "pending"
	SourceStatusProcessing = "processing"
	SourceStatusCompleted  = "completed"
	SourceStatusFailed     = "failed"
)

// ProcessingArtifact は処理途中で生成されるデータ
type ProcessingArtifact struct {
	ID        string    `json:"id"`
	SourceID  string    `json:"source_id"`
	Type      string    `json:"type"`
	Content   string    `json:"content,omitempty"`
	Format    string    `json:"format,omitempty"`
	FilePath  string    `json:"file_path,omitempty"`
	Metadata  string    `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// アーティファクトタイプ
const (
	ArtifactTypeTranscription = "transcription"
	ArtifactTypeSummary       = "summary"
	ArtifactTypeTranslation   = "translation"
)

// GetMetadata はメタデータをmapとして取得
func (s *Source) GetMetadata() (map[string]interface{}, error) {
	if s.Metadata == "" {
		return nil, nil
	}
	var m map[string]interface{}
	err := json.Unmarshal([]byte(s.Metadata), &m)
	return m, err
}

// SetMetadata はメタデータをJSON文字列として設定
func (s *Source) SetMetadata(m map[string]interface{}) error {
	if m == nil {
		s.Metadata = ""
		return nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s.Metadata = string(data)
	return nil
}
