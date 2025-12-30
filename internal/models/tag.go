package models

import "time"

// Tag は記事に付けるタグ
type Tag struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
