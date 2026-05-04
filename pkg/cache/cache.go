package cache

import (
	"sync"
	"time"
)

type entry struct {
	value     interface{}
	expiresAt time.Time
}

// Cache is a simple thread-safe in-memory TTL cache
type Cache struct {
	mu    sync.RWMutex
	items map[string]*entry
	ttl   time.Duration
}

// New creates an in-memory cache with the given TTL
func New(ttl time.Duration) *Cache {
	c := &Cache{
		items: make(map[string]*entry),
		ttl:   ttl,
	}
	go c.cleanup()
	return c
}

// Set stores a value in the cache
func (c *Cache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = &entry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.items[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.value, true
}

func (c *Cache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, e := range c.items {
			if now.After(e.expiresAt) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}
