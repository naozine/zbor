# Zbor（ずぼら）- ナレッジベース統合システム 仕様書

バージョン: 1.0
最終更新: 2025-12-28

---

## 目次

1. [システム概要](#1-システム概要)
2. [コンセプト](#2-コンセプト)
3. [システムアーキテクチャ](#3-システムアーキテクチャ)
4. [データモデル](#4-データモデル)
5. [データベース設計](#5-データベース設計)
6. [処理パイプライン](#6-処理パイプライン)
7. [ディレクトリ構造](#7-ディレクトリ構造)
8. [API設計](#8-api設計)
9. [UI画面構成](#9-ui画面構成)
10. [技術スタック](#10-技術スタック)
11. [実装フェーズ](#11-実装フェーズ)

---

## 1. システム概要

Zborは、多様な情報源（YouTube動画、音声ファイル、Web記事、テキスト）を統一的に処理し、検索可能なナレッジベースとして蓄積・活用するシステムです。

### 主な機能

- **多様な情報源の取り込み**
  - YouTube動画のURL
  - オンライン会議の録音音声（単一/複数ファイル）
  - ニュース/ブログ記事のURL
  - Markdown/テキスト

- **自動処理**
  - 音声文字起こし（Sherpa-ONNX + ReazonSpeech）
  - YouTube字幕取得
  - Web記事抽出・変換
  - セクション分割

- **ナレッジ管理**
  - 全文検索
  - タグ管理
  - 記事間リレーション
  - LLM連携（将来）

---

## 2. コンセプト

**「ずぼらに情報を放り込むだけで、整理されたナレッジベースになる」**

ユーザーは情報のURLやファイルを投げ込むだけで、システムが自動的に：
1. 内容を取得・変換
2. 文字起こし・構造化
3. 検索可能な記事として保存
4. 関連記事との紐付け

将来的にはLLMを活用して：
- 記事の要約生成
- 複数記事の統合・再構成
- ナレッジベース全体を使った推論

---

## 3. システムアーキテクチャ

```
┌─────────────────────────────────────────────────────────┐
│                    Frontend (Web UI)                     │
│  - 記事一覧・閲覧・編集                                  │
│  - ソース追加フォーム                                    │
│  - 処理状況ダッシュボード                                │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│                    API Server (Echo)                     │
│  - RESTful API                                           │
│  - WebSocket (処理進捗通知)                              │
└─────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┼───────────────┐
              ▼               ▼               ▼
    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
    │   Ingestion │  │  Processing │  │   Storage   │
    │   Pipeline  │  │   Pipeline  │  │   Layer     │
    └─────────────┘  └─────────────┘  └─────────────┘
         │                  │                  │
         └──────────────────┴──────────────────┘
                           │
                ┌──────────┴──────────┐
                ▼                     ▼
        ┌──────────────┐      ┌──────────────┐
        │  Job Queue   │      │   Database   │
        │  (SQLite)    │      │   (SQLite)   │
        └──────────────┘      └──────────────┘
```

### 主要コンポーネント

- **Frontend**: templ + Tailwind CSS によるWebインターフェース
- **API Server**: Echo v4 による RESTful API
- **Ingestion Pipeline**: 各種ソースからのデータ取り込み
- **Processing Pipeline**: 文字起こし、変換、構造化
- **Storage Layer**: SQLite データベース + ファイルストレージ
- **Worker**: バックグラウンドジョブ処理

---

## 4. データモデル

### 4.1 Article（記事）

最終的なナレッジベースの単位。

```go
type Article struct {
    // 基本情報
    ID          string    `json:"id"`           // UUID
    Title       string    `json:"title"`        // 記事タイトル
    Content     string    `json:"content"`      // Markdown本文
    Summary     string    `json:"summary,omitempty"` // 要約

    // メタデータ（頻繁に検索される項目）
    SourceType  string    `json:"source_type"`  // youtube, audio, url, text
    SourceURL   string    `json:"source_url,omitempty"`
    Author      string    `json:"author,omitempty"`
    PublishedAt *time.Time `json:"published_at,omitempty"`
    Language    string    `json:"language"`     // ja, en

    // システム情報
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
    Status      string    `json:"status"`       // draft, published

    // リレーション（別テーブルから取得）
    Tags        []Tag             `json:"tags,omitempty"`
    Relations   []ArticleRelation `json:"relations,omitempty"`
    SourceID    string            `json:"source_id,omitempty"`
    ParentID    *string           `json:"parent_id,omitempty"` // LLM再構成元

    // 構造化データ
    Sections    []Section         `json:"sections,omitempty"`

    // カスタムメタデータ（柔軟な拡張用）
    CustomMetadata map[string]interface{} `json:"custom_metadata,omitempty"`
}
```

### 4.2 Source（ソース）

入力された元データ。

```go
type Source struct {
    ID          string    `json:"id"`
    Type        string    `json:"type"`        // youtube, audio, url, text, markdown
    OriginalURL string    `json:"original_url,omitempty"`
    FilePath    string    `json:"file_path,omitempty"`
    Metadata    string    `json:"metadata"`    // JSON
    CreatedAt   time.Time `json:"created_at"`
    Status      string    `json:"status"`      // pending, processing, completed, failed
}
```

### 4.3 Tag（タグ）

```go
type Tag struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Color string `json:"color,omitempty"` // UI用の色
}
```

### 4.4 ArticleRelation（記事間リレーション）

```go
type ArticleRelation struct {
    FromArticleID string            `json:"from_article_id"`
    ToArticleID   string            `json:"to_article_id"`
    Type          string            `json:"type"` // reference, derived, related, series
    ToArticleTitle string           `json:"to_article_title,omitempty"`
    Metadata      map[string]string `json:"metadata,omitempty"`
}

// リレーションタイプ
const (
    RelationTypeReference = "reference" // 引用・参照
    RelationTypeDerived   = "derived"   // LLMで再構成
    RelationTypeRelated   = "related"   // 関連記事
    RelationTypeSeries    = "series"    // シリーズ物
)
```

### 4.5 Section（セクション）

記事内のセクション分割。

```go
type Section struct {
    ID        string `json:"id"`
    Title     string `json:"title"`
    Content   string `json:"content"`           // Markdown
    StartTime *int   `json:"start_time,omitempty"` // 秒（動画・音声用）
    EndTime   *int   `json:"end_time,omitempty"`
    Order     int    `json:"order"`
}
```

### 4.6 ProcessingJob（処理ジョブ）

非同期処理タスク。

```go
type ProcessingJob struct {
    ID          string     `json:"id"`
    SourceID    string     `json:"source_id"`
    Type        string     `json:"type"`     // transcribe, fetch, summarize
    Status      string     `json:"status"`   // queued, running, completed, failed
    Priority    int        `json:"priority"` // 0-9 (0が最高優先度)
    Progress    int        `json:"progress"` // 0-100
    RetryCount  int        `json:"retry_count"`
    Error       string     `json:"error,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    StartedAt   *time.Time `json:"started_at,omitempty"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
}
```

#### ジョブキュー設計

**ワーカー設定：**
- デフォルトワーカー数: 1（環境変数 `ZBOR_WORKERS` で変更可能）
- 音声文字起こしはCPU負荷が高いため、同時実行数を制限

**リトライ戦略：**
- 最大リトライ回数: 3回
- リトライ間隔: 指数バックオフ（1分, 5分, 15分）
- リトライ対象: ネットワークエラー、一時的な障害
- リトライ対象外: バリデーションエラー、認証エラー

**タイムアウト：**
- YouTube字幕取得: 60秒
- YouTube音声ダウンロード: 10分
- 音声文字起こし: 音声長 × 2（最大60分）
- Webページ取得: 60秒

**優先度：**
- 0: 即時処理（ユーザー待機中）
- 5: 通常処理（デフォルト）
- 9: バッチ処理（夜間など）

### 4.7 ProcessingArtifact（処理成果物）

処理途中で生成されるデータ。

```go
type ProcessingArtifact struct {
    ID        string    `json:"id"`
    SourceID  string    `json:"source_id"`
    Type      string    `json:"type"`     // transcription, summary, translation
    Content   string    `json:"content"`
    Format    string    `json:"format"`   // text, json, srt
    FilePath  string    `json:"file_path,omitempty"`
    Metadata  string    `json:"metadata"` // JSON
    CreatedAt time.Time `json:"created_at"`
}
```

---

## 5. データベース設計

### 5.1 スキーマ定義

```sql
-- 記事テーブル
CREATE TABLE articles (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    summary TEXT,

    -- メタデータ（頻繁にクエリされる）
    source_type TEXT,
    source_url TEXT,
    author TEXT,
    published_at DATETIME,
    language TEXT DEFAULT 'ja',

    -- システム情報
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    status TEXT DEFAULT 'draft',

    -- リレーション
    source_id TEXT,
    parent_id TEXT,

    -- 構造化データ（JSON）
    sections TEXT,
    custom_metadata TEXT,
    embeddings BLOB,

    FOREIGN KEY (source_id) REFERENCES sources(id),
    FOREIGN KEY (parent_id) REFERENCES articles(id)
);

-- 全文検索用仮想テーブル（SQLite FTS5 + trigram）
-- trigramトークナイザーで日本語の部分一致検索に対応
CREATE VIRTUAL TABLE articles_fts USING fts5(
    article_id UNINDEXED,
    title,
    content,
    summary,
    tokenize = 'trigram'
);

-- タグテーブル
CREATE TABLE tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    color TEXT,
    created_at DATETIME NOT NULL
);

-- 記事とタグの多対多リレーション
CREATE TABLE article_tags (
    article_id TEXT,
    tag_id INTEGER,
    PRIMARY KEY (article_id, tag_id),
    FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

-- 記事間リレーション
CREATE TABLE article_relations (
    from_article_id TEXT,
    to_article_id TEXT,
    relation_type TEXT,
    metadata TEXT,
    created_at DATETIME NOT NULL,
    PRIMARY KEY (from_article_id, to_article_id, relation_type),
    FOREIGN KEY (from_article_id) REFERENCES articles(id) ON DELETE CASCADE,
    FOREIGN KEY (to_article_id) REFERENCES articles(id) ON DELETE CASCADE
);

-- ソーステーブル
CREATE TABLE sources (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    original_url TEXT,
    file_path TEXT,
    metadata TEXT,
    created_at DATETIME NOT NULL,
    status TEXT DEFAULT 'pending'
);

-- 処理ジョブテーブル
CREATE TABLE processing_jobs (
    id TEXT PRIMARY KEY,
    source_id TEXT,
    type TEXT NOT NULL,
    status TEXT DEFAULT 'queued',
    progress INTEGER DEFAULT 0,
    error TEXT,
    created_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME,
    FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

-- 処理成果物テーブル
CREATE TABLE processing_artifacts (
    id TEXT PRIMARY KEY,
    source_id TEXT,
    type TEXT NOT NULL,
    content TEXT,
    format TEXT,
    file_path TEXT,
    metadata TEXT,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

-- インデックス
CREATE INDEX idx_articles_created_at ON articles(created_at DESC);
CREATE INDEX idx_articles_source_type ON articles(source_type);
CREATE INDEX idx_articles_status ON articles(status);
CREATE INDEX idx_sources_status ON sources(status);
CREATE INDEX idx_jobs_status ON processing_jobs(status);
```

### 5.2 全文検索戦略

#### trigramトークナイザー

日本語は空白で区切られないため、FTS5のデフォルトトークナイザーでは検索できない。
trigramトークナイザーを使用し、テキストを3文字ずつに分割してインデックスを作成する。

```
入力: "今日は天気がいい"
トークン: ["今日は", "日は天", "は天気", "天気が", "気がい", "がいい"]
```

**メリット:**
- 日本語の部分一致検索が可能
- 外部依存なし（MeCab不要）
- SQLite 3.34.0+ で標準サポート

**制限:**
- 2文字以下のクエリは検索不可（trigramが生成できない）
- インデックスサイズがやや大きくなる

#### 検索クエリの処理

```go
// 検索クエリの長さに応じて戦略を切り替え
func Search(query string) ([]Article, error) {
    runeCount := len([]rune(query))

    if runeCount < 3 {
        // 2文字以下: LIKEにフォールバック（低頻度なので許容）
        return db.Query(`
            SELECT * FROM articles
            WHERE content LIKE ? OR title LIKE ?
            ORDER BY created_at DESC`,
            "%"+query+"%", "%"+query+"%")
    }

    // 3文字以上: FTS5で高速検索
    return db.Query(`
        SELECT a.* FROM articles a
        JOIN articles_fts f ON a.id = f.article_id
        WHERE articles_fts MATCH ?
        ORDER BY rank`,
        query)
}
```

---

## 6. 処理パイプライン

### 6.1 YouTube動画の処理フロー

```
1. URL入力
   ↓
2. メタデータ取得（タイトル、再生時間、作成者など）
   ↓
3. 字幕取得を試行
   ├─ 字幕あり → 字幕をArtifactとして保存
   └─ 字幕なし → 音声ダウンロード → 文字起こし
   ↓
4. 文字起こし結果をクリーニング・整形
   ↓
5. Markdown記事生成
   ├─ タイトル: 動画タイトル
   ├─ メタデータ: URL、作成者、公開日時
   ├─ 本文: 文字起こしテキスト
   └─ カスタムメタデータ: 再生時間、サムネイルなど
   ↓
6. Article保存
   ↓
7. 全文検索インデックス更新
```

### 6.2 音声ファイルの処理フロー

#### 前提

複数ファイルの場合、全員が同時に録音開始することを想定。
オフセット検出・音声アライメントは不要。

#### 処理フロー

```
1. ファイルアップロード（単一 or 複数）
   ↓
2. 音声形式検証・変換（WAV 16kHz モノラル）
   ↓
3. 話者情報の付与
   ├─ 単一ファイル → 話者不明 or ユーザー指定
   └─ 複数ファイル → ファイル名から話者ラベル抽出
   ↓
4. 文字起こし（Sherpa-ONNX + ReazonSpeech）
   ├─ 単一ファイル → そのまま処理
   └─ 複数ファイル → 各ファイルを個別に処理
   ↓
5. タイムスタンプ付きテキスト生成
   ├─ 単一ファイル → タイムスタンプ付きテキスト
   └─ 複数ファイル → 全結果をタイムスタンプでソート・マージ
   ↓
6. Markdown記事生成
   ├─ タイトル: ユーザー入力 or "会議録 YYYY-MM-DD"
   ├─ 本文: 話者ラベル・タイムスタンプ付き会議録
   └─ カスタムメタデータ: 音声長、話者情報
   ↓
7. Article保存
   ↓
8. 元音声ファイルを関連付けて保持
```

#### 複数ファイルのマージ例

```
入力:
  田中.m4a → [00:00]こんにちは [00:05]今日は...
  佐藤.m4a → [00:02]よろしく  [00:08]はい

出力（タイムスタンプでソート）:
  [00:00] 田中: こんにちは
  [00:02] 佐藤: よろしく
  [00:05] 田中: 今日は...
  [00:08] 佐藤: はい
```

#### 音声変換パイプライン

Sherpa-ONNX + ReazonSpeechは **WAV 16kHz モノラル** 形式を要求する。
様々な入力形式に対応するため、ffmpegによる前処理を行う。

**依存関係：**
- ffmpeg（システムにインストール済みであること）

**変換コマンド：**
```bash
ffmpeg -i input.mp3 -ar 16000 -ac 1 -f wav output.wav
```

**対応入力形式：**
- MP3, M4A, AAC, OGG, FLAC, WAV, WebM

**変換処理の実装（将来）：**
```go
// internal/asr/converter.go
func ConvertToWav(inputPath, outputPath string) error {
    cmd := exec.Command("ffmpeg",
        "-i", inputPath,
        "-ar", "16000",  // サンプルレート
        "-ac", "1",      // モノラル
        "-f", "wav",
        "-y",            // 上書き
        outputPath,
    )
    return cmd.Run()
}
```

**エラーハンドリング：**
- ffmpegが見つからない → エラーメッセージで依存関係を案内
- 変換失敗 → ジョブをfailed状態に、エラー詳細を記録

### 6.3 Web記事の処理フロー

```
1. URL入力
   ↓
2. HTMLフェッチ
   ↓
3. 記事本文抽出（Readabilityアルゴリズム）
   ↓
4. HTML → Markdown変換
   ↓
5. 画像ダウンロード（オプション）
   ↓
6. Markdown記事生成
   ├─ タイトル: 記事タイトル
   ├─ メタデータ: URL、著者、公開日
   ├─ 本文: 変換されたMarkdown
   └─ カスタムメタデータ: サイト名、OGP情報など
   ↓
7. Article保存
```

### 6.4 テキスト・Markdownの処理フロー

```
1. テキスト/Markdown入力（貼り付け or ファイルアップロード）
   ↓
2. タイトル抽出 or ユーザー入力
   ↓
3. 軽微な整形（必要に応じて）
   ↓
4. Article保存
```

---

## 7. ディレクトリ構造

```
zbor/
├── cmd/
│   ├── server/              # Webサーバー
│   │   └── main.go
│   ├── transcribe/          # 音声文字起こしCLI
│   │   └── main.go
│   ├── youtube-caption/     # YouTube字幕・音声取得CLI
│   │   └── main.go
│   ├── webfetch/            # Webページ取得CLI
│   │   └── main.go
│   └── worker/              # バックグラウンド処理ワーカー（将来）
│       └── main.go
├── internal/
│   ├── asr/                 # 音声認識コアモジュール
│   │   ├── config.go        # モデル設定管理
│   │   ├── recognizer.go    # Sherpa-ONNXラッパー
│   │   └── result.go        # 結果型・フォーマット変換
│   ├── youtube/             # YouTube操作ライブラリ
│   │   ├── client.go        # YouTubeクライアント
│   │   ├── caption.go       # 字幕取得・パース
│   │   ├── audio.go         # 音声ダウンロード（多言語対応）
│   │   └── result.go        # 結果型・フォーマット変換
│   ├── webfetch/            # Webページ取得ライブラリ
│   │   ├── client.go        # nz-html-fetchラッパー
│   │   └── result.go        # 結果型・フォーマット変換
│   ├── handlers/            # HTTPハンドラー
│   │   ├── home.go
│   │   ├── about.go
│   │   ├── articles.go      # 記事管理（将来）
│   │   ├── sources.go       # ソース管理（将来）
│   │   └── jobs.go          # ジョブ管理（将来）
│   ├── version/             # バージョン情報
│   │   └── version.go
│   ├── models/              # 定数・ヘルパー（モデル型はsqlc生成）
│   │   ├── article.go       # ソースタイプ定数など
│   │   ├── source.go        # ステータス定数
│   │   ├── tag.go
│   │   └── job.go           # ジョブタイプ・ステータス定数
│   ├── ingestion/           # データ取り込みオーケストレーション（将来）
│   │   ├── youtube.go       # internal/youtubeを使用
│   │   ├── audio.go         # internal/asrを使用
│   │   ├── url.go           # internal/webfetchを使用
│   │   └── text.go
│   ├── processing/          # 処理パイプライン（将来）
│   │   ├── transcriber.go
│   │   ├── summarizer.go
│   │   ├── converter.go
│   │   └── pipeline.go
│   ├── storage/             # ストレージ層
│   │   ├── db.go            # DB接続・スキーマ初期化
│   │   ├── schema.sql       # DDL定義（embed）
│   │   ├── queries/         # sqlcクエリ定義
│   │   │   ├── articles.sql
│   │   │   ├── sources.sql
│   │   │   ├── tags.sql
│   │   │   └── jobs.sql
│   │   ├── sqlc/            # sqlc生成コード
│   │   │   ├── db.go
│   │   │   ├── models.go
│   │   │   └── *.sql.go
│   │   ├── article_repository.go
│   │   ├── source_repository.go
│   │   ├── tag_repository.go
│   │   └── job_repository.go
│   └── worker/              # ジョブ処理（将来）
│       ├── queue.go
│       └── executor.go
├── web/
│   ├── components/
│   │   ├── home.templ
│   │   ├── about.templ
│   │   ├── article_list.templ    # 将来
│   │   ├── article_view.templ    # 将来
│   │   ├── article_edit.templ    # 将来
│   │   └── source_add.templ      # 将来
│   ├── layouts/
│   │   └── base.templ
│   └── static/
│       ├── css/
│       └── js/
├── data/                    # データ保存ディレクトリ
│   ├── zbor.db              # SQLiteデータベース
│   ├── sources/             # アップロードされたソースファイル
│   │   ├── audio/
│   │   └── downloads/
│   ├── artifacts/           # 処理成果物
│   │   ├── transcriptions/
│   │   ├── summaries/
│   │   └── exports/
│   └── exports/             # エクスポートファイル
│       └── markdown/
├── models/                  # ASRモデル
│   └── sherpa-onnx-zipformer-ja-reazonspeech-2024-08-01/
├── docs/                    # ドキュメント
│   └── specification.md
├── tmp/                     # 一時ファイル
├── .air.toml
├── .env
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 8. API設計

### 8.1 記事管理API

```
GET    /api/articles              記事一覧取得
GET    /api/articles/:id          記事詳細取得
POST   /api/articles              記事作成
PUT    /api/articles/:id          記事更新
DELETE /api/articles/:id          記事削除
GET    /api/articles/search       記事検索
  Query Parameters:
    - q: 検索クエリ
    - tags: タグ（カンマ区切り）
    - source_type: ソースタイプ
    - from: 開始日時
    - to: 終了日時
    - limit: 取得件数
    - offset: オフセット
```

### 8.2 ソース管理API

```
POST   /api/sources               ソース追加
GET    /api/sources/:id           ソース詳細取得
GET    /api/sources/:id/artifacts 処理成果物一覧
DELETE /api/sources/:id           ソース削除
```

### 8.3 取り込み処理API

```
POST   /api/ingest/youtube        YouTube URL取り込み
  Body: { "url": "https://...", "title": "..." }

POST   /api/ingest/audio          音声ファイルアップロード
  Content-Type: multipart/form-data

POST   /api/ingest/url            Web記事URL取り込み
  Body: { "url": "https://..." }

POST   /api/ingest/text           テキスト取り込み
  Body: { "title": "...", "content": "...", "format": "text|markdown" }
```

### 8.4 ジョブ管理API

```
GET    /api/jobs                  ジョブ一覧
GET    /api/jobs/:id              ジョブ詳細・進捗
POST   /api/jobs/:id/cancel       ジョブキャンセル
WS     /api/jobs/ws               ジョブ進捗WebSocket
```

### 8.5 タグ管理API

```
GET    /api/tags                  タグ一覧
POST   /api/tags                  タグ作成
PUT    /api/tags/:id              タグ更新
DELETE /api/tags/:id              タグ削除
```

### 8.6 記事リレーションAPI

```
POST   /api/articles/:id/relations     リレーション追加
DELETE /api/articles/:id/relations/:to_id  リレーション削除
GET    /api/articles/:id/related       関連記事取得
```

---

## 9. UI画面構成

### 9.1 ダッシュボード

- 最近追加された記事（カード表示）
- 処理中のジョブ一覧（プログレスバー）
- 統計情報
  - 総記事数
  - 総ソース数
  - よく使われるタグ

### 9.2 記事一覧

- フィルター
  - タグ（複数選択）
  - ソースタイプ
  - 日付範囲
  - ステータス
- 検索バー（全文検索）
- ソート
  - 新しい順
  - 古い順
  - タイトル順
- 表示切り替え
  - カード表示
  - リスト表示

### 9.3 記事閲覧

- Markdownプレビュー
- 目次（セクションから自動生成）
- メタデータ表示
  - ソース情報
  - タグ
  - 作成日時・更新日時
- 関連記事リンク
- アクションボタン
  - 編集
  - 削除
  - エクスポート

### 9.4 記事編集

- Markdownエディタ（プレビュー付き）
- タイトル編集
- タグ編集（自動補完）
- メタデータ編集
- セクション管理
- 保存・キャンセルボタン

### 9.5 ソース追加

タブ切り替え式のフォーム：

**YouTubeタブ：**
- URL入力フィールド
- タイトル入力（オプション）
- タグ選択

**音声ファイルタブ：**
- ドラッグ&ドロップアップロード
- 複数ファイル対応
- タイトル入力
- 話者情報入力（オプション）

**Web記事タブ：**
- URL入力フィールド
- 画像ダウンロードオプション

**テキストタブ：**
- タイトル入力
- テキストエリア
- フォーマット選択（Plain Text / Markdown）

---

## 10. 技術スタック

### 10.1 既存技術（継続使用）

- **Go 1.25**: プログラミング言語
- **Echo v4**: HTTPフレームワーク
- **templ**: 型安全なテンプレートエンジン
- **Tailwind CSS**: CSSフレームワーク（CDN）
- **Air**: ホットリロード開発ツール
- **Sherpa-ONNX v1.12.20**: 音声認識フレームワーク
- **ReazonSpeech**: 日本語ASRモデル

### 10.2 新規追加予定

**データベース：**
- SQLite3（データベース）
- modernc.org/sqlite（Pure Go SQLiteドライバー）
- sqlc（SQLからGo型安全コード生成）

**HTML/Web処理：**
- goquery（HTML解析）
- html-to-markdown（HTML→Markdown変換）

**ユーティリティ：**
- google/uuid（UUID生成）
- fsnotify（ファイル監視・将来）

**LLM連携（将来）：**
- OpenAI Go SDK
- Anthropic Go SDK

---

## 11. 実装フェーズ

### Phase 1: 基盤構築 ✅ 完了

- [x] データベーススキーマ設計・実装
- [x] ストレージ層実装（repository パターン + sqlc）
- [x] 基本的なCRUD API
- [x] ジョブキュー実装
- [x] 記事一覧・詳細画面

**成果物：**
- データベース構造
- 基本的なAPI
- 記事の手動作成・閲覧が可能

### Phase 2: 音声処理統合

- [ ] 音声ファイルアップロード機能
- [ ] ffmpegによる形式変換（MP3, M4A等 → WAV 16kHz）
- [ ] 既存ASRモジュールの統合
- [ ] 複数ファイル対応（話者ラベル・タイムスタンプマージ）
- [ ] 音声取り込みUI

**成果物：**
- 音声ファイルから記事作成が可能

### Phase 3: YouTube統合

- [ ] YouTube取り込みパイプライン
- [ ] 既存のyoutube-testコードの統合
- [ ] 字幕取得・音声ダウンロード
- [ ] Markdown記事生成（ASRはPhase 2を再利用）
- [ ] YouTube取り込みUI

**成果物：**
- YouTube URLから記事作成が可能

### Phase 4: Web記事取り込み

- [ ] URL取り込みパイプライン
- [ ] HTML解析・記事抽出
- [ ] HTML→Markdown変換
- [ ] 画像ダウンロード（オプション）
- [ ] URL取り込みUI

**成果物：**
- Web記事URLから記事作成が可能

### Phase 5: 記事管理機能

- [ ] 全文検索機能（FTS5）
- [ ] タグ管理
- [ ] 記事編集機能
- [ ] 記事削除機能
- [ ] フィルター・ソート機能

**成果物：**
- 完全な記事管理機能

### Phase 6a: LLM基本統合（将来）

難易度: 低〜中

- [ ] LLM API連携基盤（OpenAI / Anthropic）
- [ ] 記事要約生成
- [ ] タグ自動付与
- [ ] Markdownエクスポート機能

**成果物：**
- 記事作成時に自動要約・タグ付け

### Phase 6b: 高度な検索機能（将来）

難易度: 中

- [ ] セマンティック検索（ベクトル埋め込み）
- [ ] ベクトルDB統合（SQLite-vec or 外部DB）
- [ ] 類似記事推薦
- [ ] グラフビュー（記事間リンク可視化）

**成果物：**
- 意味ベースの記事検索

### Phase 6c: RAG・高度なLLM機能（将来）

難易度: 高

- [ ] RAG（Retrieval-Augmented Generation）
- [ ] LLMによる記事再構成
- [ ] ナレッジベース全体を使った質問応答
- [ ] 翻訳機能

**成果物：**
- ナレッジベースを活用したAIアシスタント

### Phase 6d: 音声処理の高度化（将来）

難易度: 高（別モデル必要）

- [ ] 話者分離（Speaker Diarization）
- [ ] 話者識別・ラベリング
- [ ] 感情分析

**成果物：**
- 会議録の話者別整理

---

## 付録

### sqlc設定

プロジェクトルートの`sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: "sqlite"
    queries: "internal/storage/queries"
    schema: "internal/storage/schema.sql"
    gen:
      go:
        package: "sqlc"
        out: "internal/storage/sqlc"
        emit_json_tags: true
        emit_empty_slices: true
        emit_pointers_for_null_types: true
```

**コード生成:**
```bash
sqlc generate
```

**注意事項:**
- SQLファイルには日本語コメントを書かない（エンコーディング問題を回避）
- FTS5のMATCH構文はsqlcで扱えないため、FTS検索は手動SQLで実装
- `sqlc.narg()`はSQLiteで問題があるため、条件別に複数クエリを定義

---

## 付録（続き）

### A. API レスポンス例

#### 記事詳細取得

```json
GET /api/articles/550e8400-e29b-41d4-a716-446655440000

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "title": "YouTube動画: AIの未来について",
  "content": "# AIの未来について\n\n## はじめに\n...",
  "summary": "この動画ではAIの将来的な発展について...",
  "source_type": "youtube",
  "source_url": "https://www.youtube.com/watch?v=xxxxx",
  "author": "Tech Channel",
  "published_at": "2025-12-20T10:00:00Z",
  "language": "ja",
  "created_at": "2025-12-28T10:00:00Z",
  "updated_at": "2025-12-28T10:30:00Z",
  "status": "published",

  "tags": [
    {"id": 1, "name": "AI", "color": "#FF5733"},
    {"id": 5, "name": "技術", "color": "#3498DB"}
  ],

  "relations": [
    {
      "to_article_id": "abc123",
      "type": "related",
      "to_article_title": "機械学習の基礎"
    }
  ],

  "sections": [
    {
      "id": "sec1",
      "title": "はじめに",
      "content": "この動画では...",
      "start_time": 0,
      "end_time": 120,
      "order": 1
    }
  ],

  "custom_metadata": {
    "duration": 1530,
    "view_count": 12345,
    "thumbnail": "https://..."
  }
}
```

### B. Markdownエクスポート形式

```markdown
---
id: 550e8400-e29b-41d4-a716-446655440000
title: "YouTube動画: AIの未来について"
source_type: youtube
source_url: https://www.youtube.com/watch?v=xxxxx
author: Tech Channel
published_at: 2025-12-20T10:00:00Z
created_at: 2025-12-28T10:00:00Z
language: ja
tags:
  - AI
  - 技術
related:
  - "[[機械学習の基礎]]"
---

# AIの未来について

## はじめに

[00:00] この動画では...

## AIの現状

[02:00] 現在のAI技術は...
```

---

**仕様書 終わり**
