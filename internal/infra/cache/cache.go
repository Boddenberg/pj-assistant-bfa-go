// Package cache provides a simple in-memory TTL cache.
// In production, this could be backed by Redis.
package cache

import (
	"sync"
	"time"
)

type entry[T any] struct {
	value     T
	expiresAt time.Time
}

// InMemory is a thread-safe in-memory cache with TTL.
type InMemory[T any] struct {
	mu    sync.RWMutex
	items map[string]entry[T]
	ttl   time.Duration
}

// New creates a new in-memory cache with the given TTL.
func New[T any](ttl time.Duration) *InMemory[T] {
	c := &InMemory[T]{
		items: make(map[string]entry[T]),
		ttl:   ttl,
	}
	// Background cleanup goroutine
	go c.cleanup()
	return c
}

// Get retrieves a value from the cache. Returns false if not found or expired.
func (c *InMemory[T]) Get(key string) (T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.items[key]
	if !ok || time.Now().After(e.expiresAt) {
		var zero T
		return zero, false
	}
	return e.value, true
}

// Set stores a value in the cache with the configured TTL.
func (c *InMemory[T]) Set(key string, value T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = entry[T]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes a value from the cache.
func (c *InMemory[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, key)
}

// cleanup periodically removes expired entries.
func (c *InMemory[T]) cleanup() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.items {
			if now.After(v.expiresAt) {
				delete(c.items, k)
			}
		}
		c.mu.Unlock()
	}
}
