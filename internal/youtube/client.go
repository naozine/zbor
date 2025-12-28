package youtube

import (
	"time"

	"github.com/kkdai/youtube/v2"
)

// Client はYouTube API操作を抽象化するクライアント
type Client struct {
	client youtube.Client
}

// NewClient は新しいYouTubeクライアントを作成
func NewClient() *Client {
	return &Client{
		client: youtube.Client{},
	}
}

// VideoInfo は動画のメタ情報
type VideoInfo struct {
	ID          string
	Title       string
	Author      string
	Duration    time.Duration
	Description string
	Captions    []CaptionTrack
}

// CaptionTrack は字幕トラックの情報
type CaptionTrack struct {
	LanguageCode string
	Name         string
	BaseURL      string
}

// GetVideo は動画情報を取得
func (c *Client) GetVideo(url string) (*VideoInfo, error) {
	video, err := c.client.GetVideo(url)
	if err != nil {
		return nil, err
	}

	// 字幕トラックを変換
	captions := make([]CaptionTrack, len(video.CaptionTracks))
	for i, track := range video.CaptionTracks {
		captions[i] = CaptionTrack{
			LanguageCode: track.LanguageCode,
			Name:         track.Name.SimpleText,
			BaseURL:      track.BaseURL,
		}
	}

	return &VideoInfo{
		ID:          video.ID,
		Title:       video.Title,
		Author:      video.Author,
		Duration:    video.Duration,
		Description: video.Description,
		Captions:    captions,
	}, nil
}

// FindCaption は指定言語の字幕トラックを検索
// 見つからない場合は最初の字幕トラックを返す
func (v *VideoInfo) FindCaption(lang string) *CaptionTrack {
	if len(v.Captions) == 0 {
		return nil
	}

	// 指定言語を検索
	for i := range v.Captions {
		if v.Captions[i].LanguageCode == lang {
			return &v.Captions[i]
		}
	}

	// 見つからなければ最初の字幕を返す
	return &v.Captions[0]
}

// HasCaptions は字幕が利用可能かどうかを返す
func (v *VideoInfo) HasCaptions() bool {
	return len(v.Captions) > 0
}
