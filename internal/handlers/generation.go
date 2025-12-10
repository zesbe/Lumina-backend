package handlers

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/zesbe/lumina-ai/internal/cache"
	"github.com/gofiber/websocket/v2"
	"gorm.io/gorm"

	"github.com/zesbe/lumina-ai/internal/config"
	"github.com/zesbe/lumina-ai/internal/middleware"
	"github.com/zesbe/lumina-ai/internal/models"
	"github.com/zesbe/lumina-ai/internal/services"
)

type WSClient struct {
	Conn   *websocket.Conn
	UserID uint
}

type WSHub struct {
	clients map[*websocket.Conn]*WSClient
	mu      sync.RWMutex
}

var hub = &WSHub{
	clients: make(map[*websocket.Conn]*WSClient),
}

func (h *WSHub) Register(conn *websocket.Conn, userID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = &WSClient{Conn: conn, UserID: userID}
}

func (h *WSHub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
}

func (h *WSHub) SendToUser(userID uint, message interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, client := range h.clients {
		if client.UserID == userID {
			client.Conn.WriteJSON(message)
		}
	}
}

func WebSocketHandler() fiber.Handler {
	return websocket.New(func(c *websocket.Conn) {
		userID := c.Locals("userID").(uint)
		hub.Register(c, userID)
		defer hub.Unregister(c)

		for {
			_, _, err := c.ReadMessage()
			if err != nil {
				break
			}
		}
	})
}

func WebSocketUpgrade() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	}
}

func GenerateMusic(db *gorm.DB, cfg *config.Config) fiber.Handler {
	minimax := services.NewMiniMaxService(cfg.MiniMaxAPIKey, cfg.MiniMaxGroupID)

	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)

		var req models.GenerateMusicRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid request body",
			})
		}

		v := middleware.NewValidator()
		v.Required("prompt", req.Prompt).MinLength("prompt", req.Prompt, 10).NoXSS("prompt", req.Prompt)
		v.Required("lyrics", req.Lyrics).MinLength("lyrics", req.Lyrics, 10).NoXSS("lyrics", req.Lyrics)
		if req.Title != "" {
			v.NoXSS("title", req.Title)
		}
		if req.Style != "" {
			v.NoXSS("style", req.Style)
		}

		if v.HasErrors() {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation Failed",
				"details": v.Errors(),
			})
		}

		var user models.User
		if err := db.First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Not Found",
				"message": "User not found",
			})
		}

		if user.Credits < 1 {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":   "Payment Required",
				"message": "Insufficient credits. Please upgrade your plan.",
			})
		}

		generation := models.Generation{
			UserID:      userID,
			Type:        models.TypeMusic,
			Status:      models.StatusProcessing,
			Title:       middleware.SanitizeInput(req.Title),
			Prompt:      middleware.SanitizeInput(req.Prompt),
			Lyrics:      middleware.SanitizeInput(req.Lyrics),
			Style:       middleware.SanitizeInput(req.Style),
			CreditsCost: 1,
		}

		if err := db.Create(&generation).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to create generation",
			})
		}

		hub.SendToUser(userID, fiber.Map{
			"type":       "generation_started",
			"generation": generation.ToResponse(),
		})

		if !minimax.IsConfigured() {
			generation.Status = models.StatusCompleted
			generation.OutputURL = "https://www.soundhelix.com/examples/mp3/SoundHelix-Song-1.mp3"
			db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

			hub.SendToUser(userID, fiber.Map{
				"type":       "generation_completed",
				"generation": generation.ToResponse(),
			})

			return c.JSON(fiber.Map{
				"message":    "Music generated (demo mode)",
				"generation": generation.ToResponse(),
			})
		}

		go func() {
			fullPrompt := req.Prompt
			if req.Style != "" {
				fullPrompt = req.Style + ", " + req.Prompt
			}

			log.Printf("[Music] Starting generation for user %d, generation %d", userID, generation.ID)

			// Step 1: Generate music
			hub.SendToUser(userID, fiber.Map{
				"type":       "generation_progress",
				"generation": generation.ToResponse(),
				"message":    "Creating music...",
				"step":       1,
				"totalSteps": 2,
			})

			format := req.Format
			if format == "" { format = "mp3" }
			bitrate := req.Bitrate
			if bitrate <= 0 { bitrate = 256000 }
			model := req.Model
			if model == "" { model = "music-2.0" }
			resp, err := minimax.GenerateMusic(fullPrompt, req.Lyrics, format, model, bitrate)
			if err != nil {
				log.Printf("[Music] Generation failed: %v", err)
				generation.Status = models.StatusFailed
				generation.ErrorMessage = err.Error()
				db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

				hub.SendToUser(userID, fiber.Map{
					"type":       "generation_failed",
					"generation": generation.ToResponse(),
					"error":      err.Error(),
				})
				return
			}

			var audioURL string
			audioData := resp.Data.Audio

			if audioData != "" {
				if strings.HasPrefix(audioData, "http") {
					audioURL = audioData
				} else {
					audioBytes, err := hex.DecodeString(audioData)
					if err != nil {
						log.Printf("[Music] Failed to decode audio: %v", err)
						generation.Status = models.StatusFailed
						generation.ErrorMessage = "Failed to decode audio data"
						db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

						hub.SendToUser(userID, fiber.Map{
							"type":       "generation_failed",
							"generation": generation.ToResponse(),
							"error":      "Failed to decode audio data",
						})
						return
					}

					fileName := fmt.Sprintf("%d.mp3", generation.ID)
					filePath := filepath.Join("uploads", "audio", fileName)

					os.MkdirAll(filepath.Dir(filePath), 0755)

					if err := os.WriteFile(filePath, audioBytes, 0644); err != nil {
						log.Printf("[Music] Failed to save audio: %v", err)
						generation.Status = models.StatusFailed
						generation.ErrorMessage = "Failed to save audio file"
						db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

						hub.SendToUser(userID, fiber.Map{
							"type":       "generation_failed",
							"generation": generation.ToResponse(),
							"error":      "Failed to save audio file",
						})
						return
					}

					audioURL = "/uploads/audio/" + fileName
					log.Printf("[Music] Saved audio file: %s (size: %d bytes)", fileName, len(audioBytes))
				}
			}

			generation.OutputURL = audioURL

			// Step 2: Generate album art
			hub.SendToUser(userID, fiber.Map{
				"type":       "generation_progress",
				"generation": generation.ToResponse(),
				"message":    "Creating album art...",
				"step":       2,
				"totalSteps": 2,
			})

			// Create album art prompt from style/genre
			artPrompt := fmt.Sprintf("Album cover art, %s music, %s, modern design, professional artwork, high quality, artistic, beautiful colors", 
				req.Style, req.Title)
			
			albumArtURL, err := minimax.GenerateImage(artPrompt)
			if err != nil {
				log.Printf("[Music] Album art generation failed: %v", err)
				// Use placeholder gradient based on genre
				colors := []string{"6366f1", "8b5cf6", "ec4899", "f43f5e", "f97316", "eab308", "22c55e", "14b8a6", "06b6d4", "3b82f6"}
				colorIdx := int(generation.ID) % len(colors)
				generation.ThumbnailURL = fmt.Sprintf("https://placehold.co/400x400/%s/white?text=%s", colors[colorIdx], "♪")
			} else {
				generation.ThumbnailURL = albumArtURL
				log.Printf("[Music] Album art generated: %s", albumArtURL)
			}

			generation.Status = models.StatusCompleted
			generation.Metadata = string(resp.ExtraInfo)
			db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

			db.Model(&user).Update("credits", gorm.Expr("credits - ?", 1))

			db.Create(&models.CreditTransaction{
				UserID:        userID,
				Amount:        -1,
				Type:          "usage",
				Description:   "Music generation",
				GenerationID:  &generation.ID,
				BalanceBefore: user.Credits,
				BalanceAfter:  user.Credits - 1,
			})

			log.Printf("[Music] Generation completed: %d, URL: %s", generation.ID, audioURL)

			hub.SendToUser(userID, fiber.Map{
				"type":       "generation_completed",
				"generation": generation.ToResponse(),
				"audioUrl":   audioURL,
			})
		}()

		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"message":    "Music generation started",
			"generation": generation.ToResponse(),
		})
	}
}

func GenerateVideo(db *gorm.DB, cfg *config.Config) fiber.Handler {
	minimax := services.NewMiniMaxService(cfg.MiniMaxAPIKey, cfg.MiniMaxGroupID)

	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)

		var req models.GenerateVideoRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid request body",
			})
		}

		v := middleware.NewValidator()
		v.Required("prompt", req.Prompt).MinLength("prompt", req.Prompt, 10).NoXSS("prompt", req.Prompt)
		if req.Title != "" {
			v.NoXSS("title", req.Title)
		}
		if req.Narration != "" {
			v.NoXSS("narration", req.Narration)
		}

		if v.HasErrors() {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation Failed",
				"details": v.Errors(),
			})
		}

		var user models.User
		if err := db.First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Not Found",
				"message": "User not found",
			})
		}

		creditCost := 2
		if req.Narration != "" {
			creditCost = 3
		}

		if user.Credits < creditCost {
			return c.Status(fiber.StatusPaymentRequired).JSON(fiber.Map{
				"error":   "Payment Required",
				"message": "Insufficient credits. Please upgrade your plan.",
			})
		}

		model := req.Model
		if model == "" {
			model = "video-01"
		}
		duration := req.Duration
		if duration == 0 {
			duration = 6
		}
		resolution := req.Resolution
		if resolution == "" {
			resolution = "768P"
		}

		if req.Narration != "" {
			_, err := services.CalculateOptimalSpeed(req.Narration, duration)
			if err == services.ErrNarrationTooLong {
				wordCount := len(strings.Fields(req.Narration))
				maxWords := int(float64(duration) * 2.5 * 1.3)
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error":   "Narration Too Long",
					"message": fmt.Sprintf("Narration has %d words, max ~%d words for %ds video.", wordCount, maxWords, duration),
				})
			}
		}

		generation := models.Generation{
			UserID:      userID,
			Type:        models.TypeVideo,
			Status:      models.StatusProcessing,
			Title:       middleware.SanitizeInput(req.Title),
			Prompt:      middleware.SanitizeInput(req.Prompt),
			Narration:   middleware.SanitizeInput(req.Narration),
			VoiceID:     req.VoiceID,
			Duration:    duration,
			Resolution:  resolution,
			Model:       model,
			CreditsCost: creditCost,
		}

		if err := db.Create(&generation).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to create generation",
			})
		}

		hub.SendToUser(userID, fiber.Map{
			"type":       "generation_started",
			"generation": generation.ToResponse(),
		})

		if !minimax.IsConfigured() {
			generation.Status = models.StatusCompleted
			generation.OutputURL = "https://www.w3schools.com/html/mov_bbb.mp4"
			db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

			hub.SendToUser(userID, fiber.Map{
				"type":       "generation_completed",
				"generation": generation.ToResponse(),
			})

			return c.JSON(fiber.Map{
				"message":    "Video generated (demo mode)",
				"generation": generation.ToResponse(),
			})
		}

		go func() {
			log.Printf("[Video] Starting generation for user %d, generation %d, model: %s", userID, generation.ID, model)

			totalSteps := 2
			if req.Narration != "" {
				totalSteps = 3
			}

			hub.SendToUser(userID, fiber.Map{
				"type":       "generation_progress",
				"generation": generation.ToResponse(),
				"message":    "Generating video...",
				"step":       1,
				"totalSteps": totalSteps,
			})

			resp, err := minimax.GenerateVideo(req.Prompt, duration, resolution, model)
			if err != nil {
				log.Printf("[Video] API call failed: %v", err)
				generation.Status = models.StatusFailed
				generation.ErrorMessage = err.Error()
				db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

				hub.SendToUser(userID, fiber.Map{
					"type":       "generation_failed",
					"generation": generation.ToResponse(),
					"error":      err.Error(),
				})
				return
			}

			generation.MiniMaxJobID = resp.TaskID
			db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

			timeout := time.Duration(300) * time.Second
			if model == "MiniMax-Hailuo-02" {
				timeout = time.Duration(600) * time.Second
			}

			status, err := minimax.WaitForCompletion(resp.TaskID, timeout)
			if err != nil {
				log.Printf("[Video] Processing failed: %v", err)
				generation.Status = models.StatusFailed
				generation.ErrorMessage = err.Error()
				db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

				hub.SendToUser(userID, fiber.Map{
					"type":       "generation_failed",
					"generation": generation.ToResponse(),
					"error":      err.Error(),
				})
				return
			}

			videoURL := status.File.DownloadURL
			log.Printf("[Video] Video generated: %s", videoURL)

			if req.Narration != "" {
				hub.SendToUser(userID, fiber.Map{
					"type":       "generation_progress",
					"generation": generation.ToResponse(),
					"message":    "Generating voiceover...",
					"step":       2,
					"totalSteps": 3,
				})

				optimalSpeed, _ := services.CalculateOptimalSpeed(req.Narration, duration)
				if optimalSpeed < 1.0 {
					optimalSpeed = 1.0
				}

				ttsResp, err := minimax.GenerateTTSWithSpeed(req.Narration, req.VoiceID, optimalSpeed)
				if err != nil {
					log.Printf("[Video] TTS failed: %v", err)
					generation.ErrorMessage = "TTS failed: " + err.Error()
				} else {
					hub.SendToUser(userID, fiber.Map{
						"type":       "generation_progress",
						"generation": generation.ToResponse(),
						"message":    "Combining video with voiceover...",
						"step":       3,
						"totalSteps": 3,
					})

					outputFileName := fmt.Sprintf("%d_with_audio.mp4", generation.ID)
					outputPath := filepath.Join("uploads", "video", outputFileName)
					os.MkdirAll(filepath.Dir(outputPath), 0755)

					err = minimax.CombineVideoWithAudio(videoURL, ttsResp.Data.Audio, outputPath)
					if err != nil {
						log.Printf("[Video] Combine failed: %v", err)
						generation.ErrorMessage = "Combine failed: " + err.Error()
					} else {
						videoURL = "/uploads/video/" + outputFileName
					}
				}
			}

			generation.Status = models.StatusCompleted
			generation.OutputURL = videoURL
			db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

			db.Model(&user).Update("credits", gorm.Expr("credits - ?", creditCost))

			db.Create(&models.CreditTransaction{
				UserID:        userID,
				Amount:        -creditCost,
				Type:          "usage",
				Description:   "Video generation",
				GenerationID:  &generation.ID,
				BalanceBefore: user.Credits,
				BalanceAfter:  user.Credits - creditCost,
			})

			log.Printf("[Video] Generation completed: %d, URL: %s", generation.ID, videoURL)

			hub.SendToUser(userID, fiber.Map{
				"type":       "generation_completed",
				"generation": generation.ToResponse(),
				"videoUrl":   videoURL,
			})
		}()

		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
			"message":    "Video generation started",
			"generation": generation.ToResponse(),
		})
	}
}

func GetGenerations(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)

		page, _ := strconv.Atoi(c.Query("page", "1"))
		limit, _ := strconv.Atoi(c.Query("limit", "20"))
		genType := c.Query("type")
		status := c.Query("status")

		if page < 1 {
			page = 1
		}
		if limit < 1 || limit > 100 {
			limit = 20
		}

		// Try cache first
		cacheKey := fmt.Sprintf("generations:%d:%d:%d:%s:%s", userID, page, limit, genType, status)
		if cache.Cache != nil {
			var cachedResult fiber.Map
			if err := cache.Cache.Get(cacheKey, &cachedResult); err == nil {
				log.Println("[Cache HIT] GetGenerations for user:", userID)
				return c.JSON(cachedResult)
			}
		}

		offset := (page - 1) * limit

		query := db.Where("user_id = ?", userID)

		if genType != "" {
			query = query.Where("type = ?", genType)
		}
		if status != "" {
			query = query.Where("status = ?", status)
		}

		var total int64
		query.Model(&models.Generation{}).Count(&total)

		var generations []models.Generation
		if err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&generations).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to fetch generations",
			})
		}

		responses := make([]models.GenerationResponse, len(generations))
		for i, g := range generations {
			responses[i] = g.ToResponse()
		}

		result := fiber.Map{
			"generations": responses,
			"pagination": fiber.Map{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": (total + int64(limit) - 1) / int64(limit),
			},
		}

		// Cache for 30 seconds
		if cache.Cache != nil {
			cache.Cache.Set(cacheKey, result, 30*time.Second)
			log.Println("[Cache SET] GetGenerations for user:", userID)
		}

		return c.JSON(result)
	}
}


func GetGeneration(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)
		id, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid generation ID",
			})
		}

		var generation models.Generation
		if err := db.Where("id = ? AND user_id = ?", id, userID).First(&generation).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Not Found",
				"message": "Generation not found",
			})
		}

		return c.JSON(fiber.Map{
			"generation": generation.ToResponse(),
		})
	}
}

func DeleteGeneration(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)
		id, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid generation ID",
			})
		}

		var generation models.Generation
		if err := db.Where("id = ? AND user_id = ?", id, userID).First(&generation).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Not Found",
				"message": "Generation not found",
			})
		}

		if err := db.Delete(&generation).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to delete generation",
			})
		}

		return c.JSON(fiber.Map{
			"message": "Generation deleted",
		})
	}
}

func ToggleFavorite(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)
		id, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid generation ID",
			})
		}

		var generation models.Generation
		if err := db.Where("id = ? AND user_id = ?", id, userID).First(&generation).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Not Found",
				"message": "Generation not found",
			})
		}

		generation.IsFavorite = !generation.IsFavorite
		db.Save(&generation)
			// Invalidate cache
			if cache.Cache != nil {
				cache.Cache.DeletePattern(fmt.Sprintf("generations:%d:*", userID))
			}

		return c.JSON(fiber.Map{
			"message":    "Favorite toggled",
			"generation": generation.ToResponse(),
		})
	}
}

// TogglePublic toggles the public/private status of a generation
func TogglePublic(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)
		id, err := strconv.ParseUint(c.Params("id"), 10, 32)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid generation ID",
			})
		}

		var generation models.Generation
		if err := db.Where("id = ? AND user_id = ?", id, userID).First(&generation).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Not Found",
				"message": "Generation not found",
			})
		}

		generation.IsPublic = !generation.IsPublic
		db.Save(&generation)

		return c.JSON(fiber.Map{
			"message":    "Public status toggled",
			"is_public":  generation.IsPublic,
			"generation": generation.ToResponse(),
		})
	}
}

// GetPublicGenerations returns all public generations (for explore page)
func GetPublicGenerations(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, _ := strconv.Atoi(c.Query("page", "1"))
		limit, _ := strconv.Atoi(c.Query("limit", "20"))
		genType := c.Query("type")

		if page < 1 {
			page = 1
		}
		if limit < 1 || limit > 100 {
			limit = 20
		}

		offset := (page - 1) * limit

		query := db.Where("is_public = ? AND status = ?", true, models.StatusCompleted)

		if genType != "" {
			query = query.Where("type = ?", genType)
		}

		var total int64
		query.Model(&models.Generation{}).Count(&total)

		var generations []models.Generation
		if err := query.Preload("User").Order("created_at DESC").Offset(offset).Limit(limit).Find(&generations).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to fetch public generations",
			})
		}

		// Build response with user name
		responses := make([]fiber.Map, len(generations))
		for i, g := range generations {
			responses[i] = fiber.Map{
				"id":            g.ID,
				"type":          g.Type,
				"title":         g.Title,
				"style":         g.Style,
				"duration":      g.Duration,
				"output_url":    g.OutputURL,
				"thumbnail_url": g.ThumbnailURL,
				"created_at":    g.CreatedAt,
				"creator_name":  g.User.Name,
				"lyrics":        g.Lyrics,
			}
		}

		return c.JSON(fiber.Map{
			"generations": responses,
			"pagination": fiber.Map{
				"page":        page,
				"limit":       limit,
				"total":       total,
				"total_pages": (total + int64(limit) - 1) / int64(limit),
			},
		})
	}
}
