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
	mu         sync.RWMutex
	items      map[string]*entry
	ttl        time.Duration
	maxEntries int
	stop       chan struct{}
	done       chan struct{}
	closeOnce  sync.Once
}

// New creates an in-memory cache with the given TTL
func New(ttl time.Duration, maxEntries int) *Cache {
	if maxEntries < 1 {
		maxEntries = 1
	}
	c := &Cache{
		items:      make(map[string]*entry),
		ttl:        ttl,
		maxEntries: maxEntries,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go c.cleanup()
	return c
}

// Set stores a value in the cache
func (c *Cache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[key]; !exists && len(c.items) >= c.maxEntries {
		c.evictOne(time.Now())
	}
	c.items[key] = &entry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) evictOne(now time.Time) {
	var oldestKey string
	var oldestExpiry time.Time
	for key, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, key)
			return
		}
		if oldestKey == "" || item.expiresAt.Before(oldestExpiry) {
			oldestKey = key
			oldestExpiry = item.expiresAt
		}
	}
	if oldestKey != "" {
		delete(c.items, oldestKey)
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
	defer close(c.done)
	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for k, e := range c.items {
				if now.After(e.expiresAt) {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		case <-c.stop:
			return
		}
	}
}

// Close stops the background cleanup worker. It is safe to call repeatedly.
func (c *Cache) Close() {
	c.closeOnce.Do(func() { close(c.stop) })
	<-c.done
}
