-- Zbor Database Schema
-- Version: 1.0

-- 記事テーブル
CREATE TABLE IF NOT EXISTS articles (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    summary TEXT,

    -- メタデータ
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

    FOREIGN KEY (source_id) REFERENCES sources(id),
    FOREIGN KEY (parent_id) REFERENCES articles(id)
);

-- 全文検索用仮想テーブル（FTS5 + trigram）
CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
    article_id UNINDEXED,
    title,
    content,
    summary,
    tokenize = 'trigram'
);

-- タグテーブル
CREATE TABLE IF NOT EXISTS tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    color TEXT,
    created_at DATETIME NOT NULL
);

-- 記事とタグの多対多リレーション
CREATE TABLE IF NOT EXISTS article_tags (
    article_id TEXT,
    tag_id INTEGER,
    PRIMARY KEY (article_id, tag_id),
    FOREIGN KEY (article_id) REFERENCES articles(id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);

-- 記事間リレーション
CREATE TABLE IF NOT EXISTS article_relations (
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
CREATE TABLE IF NOT EXISTS sources (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    original_url TEXT,
    file_path TEXT,
    metadata TEXT,
    created_at DATETIME NOT NULL,
    status TEXT DEFAULT 'pending'
);

-- 処理ジョブテーブル
CREATE TABLE IF NOT EXISTS processing_jobs (
    id TEXT PRIMARY KEY,
    source_id TEXT,
    type TEXT NOT NULL,
    status TEXT DEFAULT 'queued',
    priority INTEGER DEFAULT 5,
    progress INTEGER DEFAULT 0,
    retry_count INTEGER DEFAULT 0,
    error TEXT,
    created_at DATETIME NOT NULL,
    started_at DATETIME,
    completed_at DATETIME,
    FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE
);

-- 処理成果物テーブル
CREATE TABLE IF NOT EXISTS processing_artifacts (
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
CREATE INDEX IF NOT EXISTS idx_articles_created_at ON articles(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_articles_source_type ON articles(source_type);
CREATE INDEX IF NOT EXISTS idx_articles_status ON articles(status);
CREATE INDEX IF NOT EXISTS idx_sources_status ON sources(status);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON processing_jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_priority ON processing_jobs(priority, created_at);
