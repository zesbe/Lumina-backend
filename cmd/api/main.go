package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"

	"github.com/zesbe/lumina-ai/internal/cache"
	"github.com/zesbe/lumina-ai/internal/config"
	"github.com/zesbe/lumina-ai/internal/database"
	"github.com/zesbe/lumina-ai/internal/handlers"
	"github.com/zesbe/lumina-ai/internal/middleware"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	cfg := config.Load()

	// Connect to database
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Initialize Redis cache
	if err := cache.InitRedis(cfg.RedisURL); err != nil {
		log.Printf("⚠️ Redis not available, running without cache: %v", err)
	} else {
		log.Println("✅ Redis cache connected")
	}

	app := fiber.New(fiber.Config{
		AppName:               "Lumina AI API",
		DisableStartupMessage: cfg.Environment == "production",
		ErrorHandler:          handlers.ErrorHandler,
		BodyLimit:             int(cfg.UploadMaxSize),
	})

	// Global middlewares
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format:     "[]  -   - \n",
		TimeFormat: "2006-01-02 15:04:05",
	}))
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-Request-ID,X-CSRF-Token,Upgrade,Connection",
		AllowCredentials: false,
		MaxAge:           86400,
	}))

	// Rate limiting
	app.Use(middleware.RateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow))

	// Health check
	app.Get("/health", handlers.HealthCheck)

	// API routes
	api := app.Group("/api/v1")

	// Public routes
	auth := api.Group("/auth")
	auth.Post("/register", middleware.StrictRateLimiter(5, cfg.RateLimitWindow), handlers.Register(db))
	auth.Post("/login", middleware.StrictRateLimiter(10, cfg.RateLimitWindow), handlers.Login(db, cfg))
	auth.Post("/refresh", handlers.RefreshToken(cfg))
	auth.Get("/csrf-token", handlers.GenerateCSRFToken)

	// Public Explore (no auth required)
	api.Get("/explore", handlers.GetPublicGenerations(db))

	// Protected routes
	protected := api.Group("/", middleware.JWTAuth(cfg.JWTSecret))

	// WebSocket for real-time updates
	protected.Use("/ws", handlers.WebSocketUpgrade())
	protected.Get("/ws", handlers.WebSocketHandler())

	// Profile
	protected.Get("/profile", handlers.GetProfile(db))
	protected.Put("/profile", handlers.UpdateProfile(db))
	protected.Post("/profile/change-password", handlers.ChangePassword(db))
	protected.Post("/logout", handlers.Logout)

	// Generations
	generations := protected.Group("/generations")
	generations.Get("/", handlers.GetGenerations(db))
	generations.Get("/:id", handlers.GetGeneration(db))
	generations.Delete("/:id", handlers.DeleteGeneration(db))
	generations.Post("/:id/favorite", handlers.ToggleFavorite(db))
	generations.Post("/:id/public", handlers.TogglePublic(db))


	// Music Generation
	music := protected.Group("/music")
	music.Post("/generate", handlers.GenerateMusic(db, cfg))

	// Video Generation
	video := protected.Group("/video")
	video.Post("/generate", handlers.GenerateVideo(db, cfg))

	// Stats (protected)
	protected.Get("/stats", handlers.ServerStats)

	// Serve uploaded files
	if cfg.StorageType == "local" {
		app.Static("/uploads", cfg.UploadPath)
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down server...")
		if cache.Cache != nil {
			cache.Cache.Close()
		}
		if err := app.Shutdown(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()

	addr := ":" + cfg.Port
	log.Printf("🚀 Lumina AI API starting on %s (env: %s)", addr, cfg.Environment)

	if err := app.Listen(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
