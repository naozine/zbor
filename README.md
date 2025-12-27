# Zbor

Zborは、Go + Echo + templを使用したシンプルなWebアプリケーションです。

## 技術スタック

- **Go 1.25**: プログラミング言語
- **Echo v4**: HTTPフレームワーク
- **templ**: 型安全なテンプレートエンジン
- **Tailwind CSS**: CSSフレームワーク（CDN経由）
- **Air**: ホットリロード開発ツール

## プロジェクト構成

```
zbor/
├── cmd/
│   └── server/          # エントリポイント
│       └── main.go
├── internal/
│   ├── handlers/        # HTTPハンドラー
│   │   ├── home.go
│   │   └── about.go
│   └── version/         # バージョン情報
│       └── version.go
├── web/
│   ├── components/      # テンプレートコンポーネント
│   │   ├── home.templ
│   │   └── about.templ
│   ├── layouts/         # ベースレイアウト
│   │   └── base.templ
│   └── static/          # 静的ファイル
│       └── css/
├── .air.toml            # Air設定
├── .env                 # 環境変数
├── go.mod               # Goモジュール定義
├── Makefile             # ビルドタスク
└── README.md
```

## セットアップ

### 前提条件

- Go 1.25以上
- templ CLI（`go install github.com/a-h/templ/cmd/templ@latest`）
- Air（オプション、ホットリロード用: `go install github.com/air-verse/air@latest`）

### インストール

```bash
# 依存関係のインストール
go mod download

# テンプレートファイルの生成
templ generate
# または
make generate
```

## 実行方法

### 通常実行

```bash
# ビルドと実行
make run

# または
make build
./zbor
```

### 開発モード（ホットリロード）

```bash
# Airを使用したホットリロード
make dev

# または
air
```

アプリケーションは `http://localhost:8080` で起動します。

## エンドポイント

| メソッド | パス | 説明 |
|---------|------|------|
| GET | `/` | ホームページ |
| GET | `/about` | Aboutページ |
| GET | `/health` | ヘルスチェック（JSON） |

## 環境変数

`.env`ファイルで設定できます：

```bash
PORT=8080                # アプリケーションポート（デフォルト: 8080）
APP_ENV=development      # 環境（development/production）
```

## ビルド

### 開発用ビルド

```bash
make build
```

### 本番用ビルド（最適化）

```bash
make build-prod
```

## Makeコマンド

```bash
make generate    # templファイルを生成
make build       # アプリケーションをビルド
make build-prod  # 本番用ビルド（最適化）
make run         # ビルドして実行
make dev         # 開発モード（ホットリロード）
make clean       # 生成ファイルを削除
make help        # ヘルプを表示
```

## 特徴

- **シンプル**: 最小限の依存関係で理解しやすい構成
- **型安全**: templによる型安全なテンプレート
- **高速**: Goの高速性能とEchoの軽量フレームワーク
- **開発体験**: Airによるホットリロードで快適な開発
- **拡張可能**: 将来的にデータベース、認証などを追加可能な設計

## ライセンス

MIT
