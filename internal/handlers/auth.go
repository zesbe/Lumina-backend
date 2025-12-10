package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/zesbe/lumina-ai/internal/auth"
	"github.com/zesbe/lumina-ai/internal/config"
	"github.com/zesbe/lumina-ai/internal/crypto"
	"github.com/zesbe/lumina-ai/internal/middleware"
	"github.com/zesbe/lumina-ai/internal/models"
)

func Register(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req models.RegisterRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid request body",
			})
		}

		v := middleware.NewValidator()
		v.Required("email", req.Email).Email("email", req.Email).NoSQLInjection("email", req.Email)
		v.Required("password", req.Password).Password("password", req.Password)
		v.Required("name", req.Name).MinLength("name", req.Name, 2).MaxLength("name", req.Name, 100).NoXSS("name", req.Name)

		if v.HasErrors() {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation Failed",
				"details": v.Errors(),
			})
		}

		var existingUser models.User
		if err := db.Where("email = ?", req.Email).First(&existingUser).Error; err == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error":   "Conflict",
				"message": "Email already registered",
			})
		}

		hashedPassword, err := crypto.HashPassword(req.Password)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to process registration",
			})
		}

		user := models.User{
			Email:        req.Email,
			PasswordHash: hashedPassword,
			Name:         middleware.SanitizeInput(req.Name),
			Role:         "user",
			Plan:         "free",
			Credits:      10,
			IsActive:     true,
		}

		if err := db.Create(&user).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to create user",
			})
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"message": "Registration successful",
			"user":    user.ToResponse(),
		})
	}
}

func Login(db *gorm.DB, cfg *config.Config) fiber.Handler {
	jwtService := auth.NewJWTService(cfg.JWTSecret, cfg.JWTExpiry, cfg.JWTRefreshExpiry)

	return func(c *fiber.Ctx) error {
		var req models.LoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid request body",
			})
		}

		v := middleware.NewValidator()
		v.Required("email", req.Email).Email("email", req.Email)
		v.Required("password", req.Password)

		if v.HasErrors() {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Validation Failed",
				"details": v.Errors(),
			})
		}

		var user models.User
		if err := db.Where("email = ? AND is_active = ?", req.Email, true).First(&user).Error; err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Unauthorized",
				"message": "Invalid credentials",
			})
		}

		valid, err := crypto.VerifyPassword(req.Password, user.PasswordHash)
		if err != nil || !valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Unauthorized",
				"message": "Invalid credentials",
			})
		}

		tokens, err := jwtService.GenerateTokenPair(user.ID, user.Email, user.Role, user.Plan)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to generate tokens",
			})
		}

		now := time.Now()
		db.Model(&user).Update("last_login_at", now)

		return c.JSON(fiber.Map{
			"message": "Login successful",
			"user":    user.ToResponse(),
			"tokens":  tokens,
		})
	}
}

func RefreshToken(cfg *config.Config) fiber.Handler {
	jwtService := auth.NewJWTService(cfg.JWTSecret, cfg.JWTExpiry, cfg.JWTRefreshExpiry)

	return func(c *fiber.Ctx) error {
		var req models.RefreshTokenRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid request body",
			})
		}

		if req.RefreshToken == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Refresh token is required",
			})
		}

		tokens, err := jwtService.RefreshTokens(req.RefreshToken)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Unauthorized",
				"message": "Invalid or expired refresh token",
			})
		}

		return c.JSON(fiber.Map{
			"message": "Token refreshed",
			"tokens":  tokens,
		})
	}
}

func Logout(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"message": "Logged out successfully",
	})
}

func GenerateCSRFToken(c *fiber.Ctx) error {
	token := make([]byte, 32)
	rand.Read(token)
	csrfToken := base64.StdEncoding.EncodeToString(token)

	return c.JSON(fiber.Map{
		"csrf_token": csrfToken,
	})
}

func GetProfile(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)

		var user models.User
		if err := db.First(&user, userID).Error; err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Not Found",
				"message": "User not found",
			})
		}

		return c.JSON(fiber.Map{
			"user": user.ToResponse(),
		})
	}
}

func UpdateProfile(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)

		var req models.UpdateProfileRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid request body",
			})
		}

		v := middleware.NewValidator()
		if req.Name != "" {
			v.MinLength("name", req.Name, 2).MaxLength("name", req.Name, 100).NoXSS("name", req.Name)
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

		updates := make(map[string]interface{})
		if req.Name != "" {
			updates["name"] = middleware.SanitizeInput(req.Name)
		}
		if req.Avatar != "" {
			updates["avatar"] = req.Avatar
		}

		if len(updates) > 0 {
			if err := db.Model(&user).Updates(updates).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error":   "Internal Server Error",
					"message": "Failed to update profile",
				})
			}
		}

		db.First(&user, userID)

		return c.JSON(fiber.Map{
			"message": "Profile updated",
			"user":    user.ToResponse(),
		})
	}
}

func ChangePassword(db *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("userID").(uint)

		var req models.ChangePasswordRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Bad Request",
				"message": "Invalid request body",
			})
		}

		v := middleware.NewValidator()
		v.Required("current_password", req.CurrentPassword)
		v.Required("new_password", req.NewPassword).Password("new_password", req.NewPassword)

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

		valid, _ := crypto.VerifyPassword(req.CurrentPassword, user.PasswordHash)
		if !valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Unauthorized",
				"message": "Current password is incorrect",
			})
		}

		hashedPassword, err := crypto.HashPassword(req.NewPassword)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":   "Internal Server Error",
				"message": "Failed to update password",
			})
		}

		db.Model(&user).Update("password_hash", hashedPassword)

		return c.JSON(fiber.Map{
			"message": "Password changed successfully",
		})
	}
}
