package models

import (
	"encoding/json"
	"time"
)

// Article はナレッジベースの記事
type Article struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Summary string `json:"summary,omitempty"`

	// メタデータ
	SourceType  string     `json:"source_type,omitempty"`
	SourceURL   string     `json:"source_url,omitempty"`
	Author      string     `json:"author,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	Language    string     `json:"language"`

	// システム情報
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Status    string    `json:"status"`

	// リレーション
	SourceID *string `json:"source_id,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`

	// 構造化データ
	Sections       []Section              `json:"sections,omitempty"`
	CustomMetadata map[string]interface{} `json:"custom_metadata,omitempty"`

	// 関連データ（JOINで取得）
	Tags      []Tag             `json:"tags,omitempty"`
	Relations []ArticleRelation `json:"relations,omitempty"`
}

// Section は記事内のセクション
type Section struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	StartTime *int   `json:"start_time,omitempty"`
	EndTime   *int   `json:"end_time,omitempty"`
	Order     int    `json:"order"`
}

// ArticleRelation は記事間のリレーション
type ArticleRelation struct {
	FromArticleID  string            `json:"from_article_id"`
	ToArticleID    string            `json:"to_article_id"`
	Type           string            `json:"type"`
	ToArticleTitle string            `json:"to_article_title,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
}

// 記事ステータス
const (
	ArticleStatusDraft     = "draft"
	ArticleStatusPublished = "published"
)

// ソースタイプ
const (
	SourceTypeYouTube  = "youtube"
	SourceTypeAudio    = "audio"
	SourceTypeURL      = "url"
	SourceTypeText     = "text"
	SourceTypeMarkdown = "markdown"
)

// リレーションタイプ
const (
	RelationTypeReference = "reference"
	RelationTypeDerived   = "derived"
	RelationTypeRelated   = "related"
	RelationTypeSeries    = "series"
)

// SectionsJSON はSectionsをJSON文字列に変換
func (a *Article) SectionsJSON() string {
	if len(a.Sections) == 0 {
		return ""
	}
	data, _ := json.Marshal(a.Sections)
	return string(data)
}

// CustomMetadataJSON はCustomMetadataをJSON文字列に変換
func (a *Article) CustomMetadataJSON() string {
	if len(a.CustomMetadata) == 0 {
		return ""
	}
	data, _ := json.Marshal(a.CustomMetadata)
	return string(data)
}

// ParseSections はJSON文字列からSectionsをパース
func (a *Article) ParseSections(data string) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), &a.Sections)
}

// ParseCustomMetadata はJSON文字列からCustomMetadataをパース
func (a *Article) ParseCustomMetadata(data string) error {
	if data == "" {
		return nil
	}
	return json.Unmarshal([]byte(data), &a.CustomMetadata)
}
