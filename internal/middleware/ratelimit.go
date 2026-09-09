package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimitConfig configures global and per-client token buckets.
type RateLimitConfig struct {
	GlobalRPS      float64
	GlobalBurst    int
	PerClientRPS   float64
	PerClientBurst int
	MaxClients     int
	ClientTTL      time.Duration
}

// RateLimiter bounds total traffic and traffic from an individual remote IP.
type RateLimiter struct {
	global       *rate.Limiter
	perClientRPS rate.Limit
	clientBurst  int
	maxClients   int
	clientTTL    time.Duration
	overflow     *rate.Limiter

	mu          sync.Mutex
	visitors    map[string]*visitor
	nextCleanup time.Time
}

func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	now := time.Now()
	return &RateLimiter{
		global:       rate.NewLimiter(rate.Limit(cfg.GlobalRPS), cfg.GlobalBurst),
		perClientRPS: rate.Limit(cfg.PerClientRPS),
		clientBurst:  cfg.PerClientBurst,
		maxClients:   cfg.MaxClients,
		clientTTL:    cfg.ClientTTL,
		overflow:     rate.NewLimiter(rate.Limit(cfg.PerClientRPS), cfg.PerClientBurst),
		visitors:     make(map[string]*visitor),
		nextCleanup:  now.Add(time.Minute),
	}
}

func (r *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.global.Allow() || !r.clientLimiter(remoteIP(c.Request)).Allow() {
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "Rate limit exceeded",
				"code":    http.StatusTooManyRequests,
			})
			return
		}
		c.Next()
	}
}

func (r *RateLimiter) clientLimiter(key string) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.After(r.nextCleanup) {
		for ip, item := range r.visitors {
			if now.Sub(item.lastSeen) > r.clientTTL {
				delete(r.visitors, ip)
			}
		}
		r.nextCleanup = now.Add(time.Minute)
	}

	if item, ok := r.visitors[key]; ok {
		item.lastSeen = now
		return item.limiter
	}
	if len(r.visitors) >= r.maxClients {
		return r.overflow
	}

	item := &visitor{
		limiter:  rate.NewLimiter(r.perClientRPS, r.clientBurst),
		lastSeen: now,
	}
	r.visitors[key] = item
	return item.limiter
}

func remoteIP(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err == nil {
		return host
	}
	return req.RemoteAddr
}
