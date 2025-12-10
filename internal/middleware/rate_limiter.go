package middleware

import (
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

type rateLimiter struct {
	requests map[string]*clientInfo
	mu       sync.RWMutex
	limit    int
	window   time.Duration
}

type clientInfo struct {
	count     int
	lastReset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		requests: make(map[string]*clientInfo),
		limit:    limit,
		window:   window,
	}

	go func() {
		ticker := time.NewTicker(window)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return rl
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, info := range rl.requests {
		if now.Sub(info.lastReset) > rl.window {
			delete(rl.requests, key)
		}
	}
}

func (rl *rateLimiter) isAllowed(clientID string) (bool, int, time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	info, exists := rl.requests[clientID]

	if !exists {
		rl.requests[clientID] = &clientInfo{
			count:     1,
			lastReset: now,
		}
		return true, rl.limit - 1, now.Add(rl.window)
	}

	if now.Sub(info.lastReset) > rl.window {
		info.count = 1
		info.lastReset = now
		return true, rl.limit - 1, now.Add(rl.window)
	}

	if info.count >= rl.limit {
		resetTime := info.lastReset.Add(rl.window)
		return false, 0, resetTime
	}

	info.count++
	remaining := rl.limit - info.count
	resetTime := info.lastReset.Add(rl.window)

	return true, remaining, resetTime
}

func RateLimiter(limit int, window time.Duration) fiber.Handler {
	limiter := newRateLimiter(limit, window)

	return func(c *fiber.Ctx) error {
		clientID := c.IP()
		if userID := c.Locals("userID"); userID != nil {
			clientID = fmt.Sprintf("user:%d", userID.(uint))
		}

		allowed, remaining, resetTime := limiter.isAllowed(clientID)

		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Set("X-RateLimit-Reset", resetTime.Format(time.RFC3339))

		if !allowed {
			retryAfter := int(time.Until(resetTime).Seconds())
			c.Set("Retry-After", fmt.Sprintf("%d", retryAfter))

			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "Too Many Requests",
				"message":     "Rate limit exceeded. Please try again later.",
				"retry_after": retryAfter,
			})
		}

		return c.Next()
	}
}

func StrictRateLimiter(limit int, window time.Duration) fiber.Handler {
	limiter := newRateLimiter(limit, window)

	return func(c *fiber.Ctx) error {
		clientID := c.IP()

		allowed, remaining, resetTime := limiter.isAllowed(clientID)

		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
		c.Set("X-RateLimit-Reset", resetTime.Format(time.RFC3339))

		if !allowed {
			retryAfter := int(time.Until(resetTime).Seconds())
			c.Set("Retry-After", fmt.Sprintf("%d", retryAfter))

			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":       "Too Many Requests",
				"message":     "Rate limit exceeded. Please try again later.",
				"retry_after": retryAfter,
			})
		}

		return c.Next()
	}
}
