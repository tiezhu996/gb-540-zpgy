package middleware

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"cadastral-boundary-topology-resolution/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func AuthMiddleware(auth *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			unauthorized(c, "bearer token is required")
			return
		}
		claims, err := auth.Parse(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")))
		if err != nil {
			unauthorized(c, "access token is invalid or expired")
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

type visitor struct {
	count     int
	expiresAt time.Time
}

type RateLimiter struct {
	mu        sync.Mutex
	limit     int
	window    time.Duration
	visitors  map[string]visitor
	lastSweep time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, visitors: make(map[string]visitor), lastSweep: time.Now()}
}

func (l *RateLimiter) Allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.Sub(l.lastSweep) > l.window {
		for k, value := range l.visitors {
			if now.After(value.expiresAt) {
				delete(l.visitors, k)
			}
		}
		l.lastSweep = now
	}
	current := l.visitors[key]
	if current.expiresAt.IsZero() || now.After(current.expiresAt) {
		current = visitor{count: 0, expiresAt: now.Add(l.window)}
	}
	if current.count >= l.limit {
		return false, time.Until(current.expiresAt)
	}
	current.count++
	l.visitors[key] = current
	return true, time.Until(current.expiresAt)
}

func RateLimitMiddleware(limiter *RateLimiter, scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, retry := limiter.Allow(scope+":"+c.ClientIP(), time.Now())
		if !allowed {
			retryAfter := int(math.Ceil(retry.Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatusJSON(429, gin.H{"error": gin.H{"code": "RATE_LIMITED", "message": "request rate limit exceeded", "request_id": c.GetString("request_id")}})
			return
		}
		c.Next()
	}
}

func unauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(401, gin.H{"error": gin.H{"code": service.CodeUnauthorized, "message": message, "request_id": c.GetString("request_id")}})
}
