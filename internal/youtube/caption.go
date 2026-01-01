package youtube

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

// YouTube字幕のXML構造
type xmlTranscript struct {
	XMLName xml.Name  `xml:"timedtext"`
	Text    []xmlText `xml:"body>p"`
}

type xmlText struct {
	Start    int64        `xml:"t,attr"`  // ミリ秒
	Duration int64        `xml:"d,attr"`  // ミリ秒
	Segments []xmlSegment `xml:"s"`
}

type xmlSegment struct {
	Text string `xml:",chardata"`
}

// FetchCaption は指定言語の字幕を取得
func (c *Client) FetchCaption(video *VideoInfo, lang string) (*CaptionResult, error) {
	track := video.FindCaption(lang)
	if track == nil {
		return nil, fmt.Errorf("no captions available")
	}

	result, err := c.FetchCaptionByURL(track.BaseURL)
	if err != nil {
		return nil, err
	}

	result.LanguageCode = track.LanguageCode
	return result, nil
}

// FetchCaptionByURL はURLから直接字幕を取得
func (c *Client) FetchCaptionByURL(url string) (*CaptionResult, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return parseTranscriptXML(body)
}

// parseTranscriptXML はXMLをパースしてCaptionResultを返す
func parseTranscriptXML(data []byte) (*CaptionResult, error) {
	var transcript xmlTranscript
	if err := xml.Unmarshal(data, &transcript); err != nil {
		return nil, fmt.Errorf("XML parse failed: %w", err)
	}

	entries := make([]CaptionEntry, 0, len(transcript.Text))
	for _, p := range transcript.Text {
		// セグメントを連結してテキストを作成
		var text string
		for _, seg := range p.Segments {
			text += seg.Text
		}

		// 空エントリをスキップ
		if len(text) == 0 {
			continue
		}

		entries = append(entries, CaptionEntry{
			StartTime: time.Duration(p.Start) * time.Millisecond,
			Duration:  time.Duration(p.Duration) * time.Millisecond,
			Text:      text,
		})
	}

	return &CaptionResult{
		Entries: entries,
	}, nil
}
