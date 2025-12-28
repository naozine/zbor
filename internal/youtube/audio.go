package youtube

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	ytdl "github.com/kkdai/youtube/v2"
)

// AudioFormat は音声フォーマット情報
type AudioFormat struct {
	ItagNo        int
	MimeType      string // "audio/mp4", "audio/webm"
	Bitrate       int    // ビットレート (bps)
	ContentLength int64  // ファイルサイズ (bytes)
	Quality       string // 音質ラベル
	Language      string // 言語コード (例: "ja", "en")
	LanguageName  string // 言語表示名 (例: "日本語", "English")
	IsDefault     bool   // デフォルト音声トラックかどうか
}

// Extension はMIMEタイプから拡張子を返す
func (f *AudioFormat) Extension() string {
	if strings.Contains(f.MimeType, "mp4") {
		return ".m4a"
	}
	if strings.Contains(f.MimeType, "webm") {
		return ".webm"
	}
	return ".audio"
}

// DownloadAudioOptions はダウンロードオプション
type DownloadAudioOptions struct {
	Format     string // "mp4", "webm", "best" (default: "best")
	Language   string // 言語コード (例: "ja", "en")、空の場合はデフォルト
	OutputPath string // 出力先パス
}

// GetAudioFormats は利用可能な音声フォーマット一覧を取得
func (c *Client) GetAudioFormats(videoURL string) ([]AudioFormat, error) {
	video, err := c.client.GetVideo(videoURL)
	if err != nil {
		return nil, err
	}

	var formats []AudioFormat
	for _, f := range video.Formats {
		// 音声のみのフォーマットをフィルタ
		if !strings.HasPrefix(f.MimeType, "audio/") {
			continue
		}

		af := AudioFormat{
			ItagNo:        f.ItagNo,
			MimeType:      f.MimeType,
			Bitrate:       f.Bitrate,
			ContentLength: f.ContentLength,
			Quality:       f.AudioQuality,
		}

		// 音声トラック情報があれば追加
		if f.AudioTrack != nil {
			af.LanguageName = f.AudioTrack.DisplayName
			af.Language = f.AudioTrack.ID
			af.IsDefault = f.AudioTrack.AudioIsDefault
		}

		formats = append(formats, af)
	}

	// ビットレート降順でソート
	sort.Slice(formats, func(i, j int) bool {
		return formats[i].Bitrate > formats[j].Bitrate
	})

	return formats, nil
}

// selectAudioFormat は指定された形式と言語に基づいて最適なフォーマットを選択
func (c *Client) selectAudioFormat(videoURL string, formatType string, language string) (*AudioFormat, error) {
	formats, err := c.GetAudioFormats(videoURL)
	if err != nil {
		return nil, err
	}

	if len(formats) == 0 {
		return nil, fmt.Errorf("no audio formats available")
	}

	// 言語でフィルタ（指定されている場合）
	if language != "" {
		var langFiltered []AudioFormat
		for _, f := range formats {
			// 言語IDの先頭が一致するか確認（例: "ja" -> "ja.4" にマッチ）
			langMatch := strings.HasPrefix(strings.ToLower(f.Language), strings.ToLower(language))
			// または言語名に含まれるか（例: "japanese" -> "Japanese original" にマッチ）
			nameMatch := strings.Contains(strings.ToLower(f.LanguageName), strings.ToLower(language))
			if langMatch || nameMatch {
				langFiltered = append(langFiltered, f)
			}
		}
		if len(langFiltered) > 0 {
			formats = langFiltered
		}
		// 見つからない場合はフィルタなしで続行
	}

	// フォーマットタイプでフィルタ
	var filtered []AudioFormat
	switch formatType {
	case "mp4":
		for _, f := range formats {
			if strings.Contains(f.MimeType, "mp4") {
				filtered = append(filtered, f)
			}
		}
	case "webm":
		for _, f := range formats {
			if strings.Contains(f.MimeType, "webm") {
				filtered = append(filtered, f)
			}
		}
	default: // "best"
		filtered = formats
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("no audio formats available for type: %s", formatType)
	}

	// 最高ビットレートを返す（既にソート済み）
	return &filtered[0], nil
}

// DownloadAudio は音声をダウンロード
func (c *Client) DownloadAudio(videoURL string, opts *DownloadAudioOptions) error {
	return c.DownloadAudioWithProgress(videoURL, opts, nil)
}

// DownloadAudioWithProgress はプログレス付きでダウンロード
func (c *Client) DownloadAudioWithProgress(videoURL string, opts *DownloadAudioOptions, progress func(current, total int64)) error {
	if opts == nil {
		opts = &DownloadAudioOptions{Format: "best"}
	}
	if opts.Format == "" {
		opts.Format = "best"
	}

	// 動画情報を取得
	video, err := c.client.GetVideo(videoURL)
	if err != nil {
		return fmt.Errorf("failed to get video: %w", err)
	}

	// 最適なフォーマットを選択
	selectedFormat, err := c.selectAudioFormat(videoURL, opts.Format, opts.Language)
	if err != nil {
		return err
	}

	// 対応するyoutubeライブラリのFormatを見つける（ItagNo + 言語で一致）
	var targetFormat *ytdl.Format
	for i := range video.Formats {
		f := &video.Formats[i]
		if f.ItagNo != selectedFormat.ItagNo {
			continue
		}
		// 言語トラックも一致させる
		if selectedFormat.Language != "" {
			if f.AudioTrack == nil || f.AudioTrack.ID != selectedFormat.Language {
				continue
			}
		}
		targetFormat = f
		break
	}
	if targetFormat == nil {
		return fmt.Errorf("format not found: itag=%d lang=%s", selectedFormat.ItagNo, selectedFormat.Language)
	}

	// ストリームを取得
	ctx := context.Background()
	stream, size, err := c.client.GetStream(video, targetFormat)
	if err != nil {
		return fmt.Errorf("failed to get stream: %w", err)
	}
	defer stream.Close()

	// 出力先を決定
	outputPath := opts.OutputPath
	if outputPath == "" {
		outputPath = sanitizeFilename(video.Title) + selectedFormat.Extension()
	}

	// ファイルを作成
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// ダウンロード
	if progress != nil {
		// プログレス付きダウンロード
		err = copyWithProgress(ctx, file, stream, size, progress)
	} else {
		_, err = io.Copy(file, stream)
	}

	if err != nil {
		os.Remove(outputPath) // 失敗時はファイルを削除
		return fmt.Errorf("failed to download: %w", err)
	}

	return nil
}

// copyWithProgress はプログレスコールバック付きでコピー
func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, total int64, progress func(current, total int64)) error {
	buf := make([]byte, 32*1024)
	var written int64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		nr, err := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				written += int64(nw)
				if progress != nil {
					progress(written, total)
				}
			}
			if ew != nil {
				return ew
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	return nil
}

// sanitizeFilename はファイル名として使えない文字を置換
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}
