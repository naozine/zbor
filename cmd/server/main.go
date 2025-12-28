package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"zbor/internal/handlers"
	"zbor/internal/storage"
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

	// データベースパスを取得（デフォルト: ~/.zbor/zbor.db）
	dbPath := os.Getenv("ZBOR_DB_PATH")
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}
		dbPath = filepath.Join(home, ".zbor", "zbor.db")
	}

	// データベース初期化
	db, err := storage.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	log.Printf("Database initialized at %s", dbPath)

	// リポジトリ作成
	articleRepo := storage.NewArticleRepository(db)
	tagRepo := storage.NewTagRepository(db)

	// ハンドラー作成
	articleHandler := handlers.NewArticleHandler(articleRepo)
	tagHandler := handlers.NewTagHandler(tagRepo)

	// Echoインスタンスの作成
	e := echo.New()

	// ミドルウェアの設定
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// ルートの登録（Web UI）
	e.GET("/", handlers.Home)
	e.GET("/about", handlers.About)
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{
			"status":  "ok",
			"version": version.Version,
		})
	})

	// API ルートの登録
	api := e.Group("/api")

	// Articles API
	api.GET("/articles", articleHandler.List)
	api.GET("/articles/search", articleHandler.Search)
	api.POST("/articles", articleHandler.Create)
	api.GET("/articles/:id", articleHandler.Get)
	api.PUT("/articles/:id", articleHandler.Update)
	api.DELETE("/articles/:id", articleHandler.Delete)
	api.POST("/articles/:id/tags/:tag_id", articleHandler.AddTag)
	api.DELETE("/articles/:id/tags/:tag_id", articleHandler.RemoveTag)

	// Tags API
	api.GET("/tags", tagHandler.List)
	api.POST("/tags", tagHandler.Create)
	api.GET("/tags/:id", tagHandler.Get)
	api.PUT("/tags/:id", tagHandler.Update)
	api.DELETE("/tags/:id", tagHandler.Delete)

	// サーバー起動
	log.Printf("Starting Zbor v%s on port %s", version.Version, port)
	if err := e.Start(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatal(err)
	}
}
