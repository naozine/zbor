.PHONY: generate build run dev clean help

# デフォルトターゲット
.DEFAULT_GOAL := help

# テンプレート生成
generate:
	@echo "Generating templ files..."
	@templ generate

# ビルド
build: generate
	@echo "Building zbor..."
	@go build -o zbor ./cmd/server

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
	@find . -name "*_templ.go" -delete

# ヘルプ
help:
	@echo "Available targets:"
	@echo "  generate   - Generate templ files"
	@echo "  build      - Build the application"
	@echo "  build-prod - Build for production (optimized)"
	@echo "  run        - Build and run the application"
	@echo "  dev        - Start development mode with Air"
	@echo "  clean      - Clean generated files"
	@echo "  help       - Show this help message"
