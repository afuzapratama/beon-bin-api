package cache

import (
	"testing"
	"time"
)

func TestCacheEvictsOldestEntryAtCapacity(t *testing.T) {
	c := New(time.Minute, 2)
	t.Cleanup(c.Close)
	c.Set("first", 1)
	time.Sleep(time.Millisecond)
	c.Set("second", 2)
	c.Set("third", 3)

	if _, ok := c.Get("first"); ok {
		t.Fatal("oldest entry was not evicted")
	}
	if _, ok := c.Get("second"); !ok {
		t.Fatal("second entry was unexpectedly evicted")
	}
	if _, ok := c.Get("third"); !ok {
		t.Fatal("new entry was not cached")
	}
}

func TestCacheExpiresEntry(t *testing.T) {
	c := New(time.Millisecond, 2)
	t.Cleanup(c.Close)
	c.Set("key", "value")
	time.Sleep(5 * time.Millisecond)
	if _, ok := c.Get("key"); ok {
		t.Fatal("expired entry was returned")
	}
}

func TestCacheMissAndUpdateExistingEntry(t *testing.T) {
	c := New(time.Minute, 1)
	t.Cleanup(c.Close)
	if value, ok := c.Get("missing"); ok || value != nil {
		t.Fatalf("missing value = %v, ok=%v", value, ok)
	}

	c.Set("key", "old")
	c.Set("key", "new")
	value, ok := c.Get("key")
	if !ok || value != "new" {
		t.Fatalf("updated value = %v, ok=%v", value, ok)
	}
}

func TestCacheUsesMinimumCapacityAndPrefersExpiredEviction(t *testing.T) {
	c := New(time.Millisecond, 0)
	t.Cleanup(c.Close)
	c.Set("expired", 1)
	time.Sleep(5 * time.Millisecond)
	c.Set("fresh", 2)

	if _, ok := c.Get("expired"); ok {
		t.Fatal("expired entry survived capacity eviction")
	}
	if value, ok := c.Get("fresh"); !ok || value != 2 {
		t.Fatalf("fresh value = %v, ok=%v", value, ok)
	}
}

func TestCacheCloseIsIdempotent(t *testing.T) {
	c := New(time.Minute, 1)
	c.Close()
	c.Close()
}
