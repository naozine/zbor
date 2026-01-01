.PHONY: generate generate-sqlc generate-templ build build-dev run dev clean help

# デフォルトターゲット
.DEFAULT_GOAL := help

# sqlcコード生成
generate-sqlc:
	@echo "Generating sqlc files..."
	@sqlc generate

# templコード生成
generate-templ:
	@echo "Generating templ files..."
	@templ generate

# 全コード生成
generate: generate-sqlc generate-templ

# ビルド
build: generate
	@echo "Building zbor..."
	@go build -o zbor ./cmd/server

# 開発用ビルド（air用、tmp/mainに出力）
build-dev: generate
	@go build -o ./tmp/main ./cmd/server

# 本番用ビルド
build-prod: generate
	@echo "Building zbor for production..."
	@go build -ldflags="-s -w" -o zbor ./cmd/server

# 実行
run: build
	@echo "Running zbor..."
	@./zbor

# 開発モード（Airでホットリロード）
dev:
	@echo "Starting development mode with Air..."
	@air

# クリーンアップ
clean:
	@echo "Cleaning up..."
	@rm -f zbor
	@rm -rf tmp/
	@find . -name "*_templ.go" -delete

# ヘルプ
help:
	@echo "Available targets:"
	@echo "  generate       - Generate all code (sqlc + templ)"
	@echo "  generate-sqlc  - Generate sqlc files"
	@echo "  generate-templ - Generate templ files"
	@echo "  build          - Build the application"
	@echo "  build-dev      - Build for development (air)"
	@echo "  build-prod     - Build for production (optimized)"
	@echo "  run            - Build and run the application"
	@echo "  dev            - Start development mode with Air"
	@echo "  clean          - Clean generated files"
	@echo "  help           - Show this help message"
