package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/kkdai/youtube/v2"
)

// YouTube字幕のXML構造
type Transcript struct {
	XMLName xml.Name `xml:"timedtext"`
	Text    []Text   `xml:"body>p"`
}

type Text struct {
	Start    float64   `xml:"t,attr"`
	Duration float64   `xml:"d,attr"`
	Segments []Segment `xml:"s"`
}

type Segment struct {
	Text string `xml:",chardata"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("使い方: go run cmd/youtube-test/main.go <YouTube URL>")
		fmt.Println("例: go run cmd/youtube-test/main.go https://www.youtube.com/watch?v=dQw4w9WgXcQ")
		os.Exit(1)
	}

	videoURL := os.Args[1]

	// YouTubeクライアントの作成
	client := youtube.Client{}

	fmt.Printf("動画を取得中: %s\n\n", videoURL)

	// 動画情報を取得
	video, err := client.GetVideo(videoURL)
	if err != nil {
		log.Fatalf("動画の取得に失敗: %v\n", err)
	}

	// 基本情報を表示
	fmt.Println("=== 動画情報 ===")
	fmt.Printf("タイトル: %s\n", video.Title)
	fmt.Printf("作成者: %s\n", video.Author)
	fmt.Printf("再生時間: %s\n", video.Duration)
	fmt.Printf("説明: %.100s...\n", video.Description)
	fmt.Printf("動画ID: %s\n", video.ID)
	fmt.Println()

	// 字幕情報を表示
	fmt.Println("=== 利用可能な字幕 ===")
	if len(video.CaptionTracks) == 0 {
		fmt.Println("字幕がありません")
	} else {
		for i, caption := range video.CaptionTracks {
			fmt.Printf("%d. 言語: %s (%s)\n", i+1, caption.LanguageCode, caption.Name)
		}

		// 日本語字幕を探す（なければ最初の字幕）
		var selectedCaption *youtube.CaptionTrack
		for i := range video.CaptionTracks {
			if video.CaptionTracks[i].LanguageCode == "ja" {
				selectedCaption = &video.CaptionTracks[i]
				break
			}
		}
		if selectedCaption == nil {
			selectedCaption = &video.CaptionTracks[0]
		}

		// 字幕を直接HTTPで取得
		fmt.Println("\n=== 字幕の取得テスト ===")
		fmt.Printf("言語 '%s' の字幕を取得中...\n", selectedCaption.LanguageCode)

		transcript, err := fetchTranscript(selectedCaption.BaseURL)
		if err != nil {
			fmt.Printf("字幕の取得に失敗: %v\n", err)
		} else {
			fmt.Printf("字幕エントリ数: %d\n", len(transcript.Text))
			fmt.Println("\n最初の20エントリ:")
			count := 0
			for _, p := range transcript.Text {
				// セグメントを連結してテキストを作成
				var content string
				for _, seg := range p.Segments {
					content += seg.Text
				}

				// 空白エントリをスキップ
				if len(content) == 0 {
					continue
				}

				// タイムスタンプはミリ秒単位なので秒に変換
				seconds := p.Start / 1000.0
				minutes := int(seconds / 60)
				secs := int(seconds) % 60
				fmt.Printf("  [%02d:%02d] %s\n", minutes, secs, content)

				count++
				if count >= 20 {
					break
				}
			}
		}
	}

	fmt.Println("\n✅ テスト完了")
}

// 字幕を直接HTTPリクエストで取得
func fetchTranscript(url string) (*Transcript, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTPリクエスト失敗: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("レスポンス読み込み失敗: %w", err)
	}

	var transcript Transcript
	if err := xml.Unmarshal(body, &transcript); err != nil {
		return nil, fmt.Errorf("XML解析失敗: %w", err)
	}

	return &transcript, nil
}
