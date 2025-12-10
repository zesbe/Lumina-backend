package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/zesbe/lumina-ai/internal/auth"
)

func JWTAuth(secret string) fiber.Handler {
	jwtService := auth.NewJWTService(secret, 0, 0)

	return func(c *fiber.Ctx) error {
		var tokenString string

		// Check Authorization header first
		authHeader := c.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				tokenString = parts[1]
			}
		}

		// Fallback to query param for WebSocket
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Unauthorized",
				"message": "Missing authorization",
			})
		}

		claims, err := jwtService.ValidateToken(tokenString)
		if err != nil {
			if err == auth.ErrExpiredToken {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"error":   "Unauthorized",
					"message": "Token has expired",
					"code":    "TOKEN_EXPIRED",
				})
			}
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Unauthorized",
				"message": "Invalid token",
			})
		}

		if claims.TokenType != auth.AccessToken {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":   "Unauthorized",
				"message": "Invalid token type",
			})
		}

		c.Locals("userID", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		c.Locals("plan", claims.Plan)
		c.Locals("claims", claims)

		return c.Next()
	}
}

func RequireRole(roles ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userRole := c.Locals("role").(string)
		for _, role := range roles {
			if userRole == role {
				return c.Next()
			}
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":   "Forbidden",
			"message": "Insufficient permissions",
		})
	}
}

func RequirePlan(plans ...string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userPlan := c.Locals("plan").(string)
		for _, plan := range plans {
			if userPlan == plan {
				return c.Next()
			}
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":   "Forbidden",
			"message": "Plan upgrade required",
		})
	}
}
