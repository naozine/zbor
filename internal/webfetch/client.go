package webfetch

import (
	"context"
	"time"

	"github.com/naozine/nz-html-fetch/pkg/htmlfetch"
)

// Client はWebページ取得クライアント
type Client struct {
	fetcher *htmlfetch.Fetcher
}

// Options はクライアント作成オプション
type Options struct {
	Stealth     bool   // ボット検出回避（デフォルト: true）
	Proxy       string // プロキシアドレス
	BrowserPath string // ブラウザパス
}

// FetchOptions はフェッチ実行オプション
type FetchOptions struct {
	BlockAds    bool          // 広告ブロック
	BlockImages bool          // 画像ブロック
	WaitTime    time.Duration // 待機時間
	Selector    string        // 待機セレクタ
}

// NewClient は新しいクライアントを作成
func NewClient(opts *Options) (*Client, error) {
	var fetcherOpts []htmlfetch.Option

	if opts != nil {
		if opts.BrowserPath != "" {
			fetcherOpts = append(fetcherOpts, htmlfetch.WithBrowserPath(opts.BrowserPath))
		}
		if opts.Proxy != "" {
			fetcherOpts = append(fetcherOpts, htmlfetch.WithProxy(opts.Proxy))
		}
		// Stealthはデフォルトでtrue、明示的にfalseの場合のみ無効化
		fetcherOpts = append(fetcherOpts, htmlfetch.WithStealth(opts.Stealth))
	}

	fetcher := htmlfetch.New(fetcherOpts...)

	// ブラウザを起動（高速モード）
	if err := fetcher.Start(); err != nil {
		return nil, err
	}

	return &Client{fetcher: fetcher}, nil
}

// Close はブラウザを終了
func (c *Client) Close() error {
	if c.fetcher != nil {
		return c.fetcher.Close()
	}
	return nil
}

// FetchMarkdown はURLからMarkdownを取得
func (c *Client) FetchMarkdown(ctx context.Context, url string, opts *FetchOptions) (*Result, error) {
	fetchOpts := buildFetchOptions(opts)
	fetchOpts = append(fetchOpts, htmlfetch.WithMarkdown())

	result, err := c.fetcher.Fetch(ctx, url, fetchOpts...)
	if err != nil {
		return nil, err
	}

	return &Result{
		URL:      result.FinalURL,
		Content:  result.Markdown,
		Duration: result.Duration,
	}, nil
}

// FetchHTML はURLからHTMLを取得
func (c *Client) FetchHTML(ctx context.Context, url string, opts *FetchOptions) (*Result, error) {
	fetchOpts := buildFetchOptions(opts)

	result, err := c.fetcher.Fetch(ctx, url, fetchOpts...)
	if err != nil {
		return nil, err
	}

	return &Result{
		URL:      result.FinalURL,
		Content:  result.HTML,
		Duration: result.Duration,
	}, nil
}

// buildFetchOptions はFetchOptionsからhtmlfetch.FetchOptionを構築
func buildFetchOptions(opts *FetchOptions) []htmlfetch.FetchOption {
	var fetchOpts []htmlfetch.FetchOption

	if opts == nil {
		return fetchOpts
	}

	// ブロッキングオプション
	if opts.BlockAds || opts.BlockImages {
		blocking := htmlfetch.BlockingOptions{
			Ads:   opts.BlockAds,
			Image: opts.BlockImages,
		}
		fetchOpts = append(fetchOpts, htmlfetch.WithBlocking(blocking))
	}

	// セレクタ待機
	if opts.Selector != "" {
		timeout := 30 * time.Second
		if opts.WaitTime > 0 {
			timeout = opts.WaitTime
		}
		fetchOpts = append(fetchOpts, htmlfetch.WithSelector(opts.Selector, timeout))
	}

	return fetchOpts
}
