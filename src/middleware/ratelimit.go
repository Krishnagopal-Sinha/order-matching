package middleware

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

type RateLimiter struct {
	maxRequests   int
	windowDuration time.Duration
	counters       map[string]int
	mu             sync.Mutex
}

func NewRateLimiter(maxRequests int, windowDuration time.Duration) *RateLimiter {
	return &RateLimiter{
		maxRequests:   maxRequests,
		windowDuration: windowDuration,
		counters:      make(map[string]int),
	}
}

func (rl *RateLimiter) getClientID(c *fiber.Ctx) string {
	ip := c.Get("X-Forwarded-For")
	if ip == "" {
		ip = c.Get("X-Real-IP")
	}
	if ip == "" {
		ip = c.IP()
	}
	return ip
}

func (rl *RateLimiter) getWindowKey(clientIP string, now time.Time) string {
	windowNumber := now.Unix() / int64(rl.windowDuration.Seconds())
	return fmt.Sprintf("%s_%d", clientIP, windowNumber)
}

func (rl *RateLimiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	key := rl.getWindowKey(clientIP, now)

	count, exists := rl.counters[key]

	if !exists {
		// edge case: remove old windows when starting new window
		rl.removeOldWindows(clientIP, now)
		rl.counters[key] = 1
		return true
	}

	if count >= rl.maxRequests {
		return false
	}

	rl.counters[key] = count + 1
	return true
}

func (rl *RateLimiter) removeOldWindows(clientIP string, now time.Time) {
	currentWindowKey := rl.getWindowKey(clientIP, now)
	
	for key := range rl.counters {
		if key != currentWindowKey {
			clientPrefix := clientIP + "_"
			if len(key) > len(clientPrefix) && key[:len(clientPrefix)] == clientPrefix {
				delete(rl.counters, key)
			}
		}
	}
}

func (rl *RateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		clientID := rl.getClientID(c)

		if !rl.Allow(clientID) {
			log.Warn().
				Str("client_ip", clientID).
				Str("path", c.Path()).
				Str("method", c.Method()).
				Int("max_requests", rl.maxRequests).
				Msg("Rate limit exceeded")
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "Rate limit exceeded",
				"message": "Too many requests. Please try again later.",
			})
		}

		c.Set("X-RateLimit-Limit", strconv.Itoa(rl.maxRequests))
		c.Set("X-RateLimit-Window", rl.windowDuration.String())

		return c.Next()
	}
}

func DefaultRateLimiter() *RateLimiter {
	return NewRateLimiter(100, time.Second)
}
