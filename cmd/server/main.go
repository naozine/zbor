package main

import (
	"fmt"
	"log"
	"os"

	"zbor/internal/handlers"
	"zbor/internal/version"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	// .envファイルを読み込み（存在しない場合はスキップ）
	_ = godotenv.Load()

	// 環境変数からポート番号を取得（デフォルト: 8080）
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Echoインスタンスの作成
	e := echo.New()

	// ミドルウェアの設定
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// ルートの登録
	e.GET("/", handlers.Home)
	e.GET("/about", handlers.About)
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status":  "ok",
			"version": version.Version,
		})
	})

	// サーバー起動
	log.Printf("Starting Zbor v%s on port %s", version.Version, port)
	if err := e.Start(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatal(err)
	}
}
