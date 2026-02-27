package cache_test

import (
	"testing"
	"time"

	"github.com/boddenberg/pj-assistant-bfa-go/internal/infra/cache"
)

func TestCache_SetAndGet(t *testing.T) {
	c := cache.New[string](5 * time.Minute)

	c.Set("key1", "value1")
	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if val != "value1" {
		t.Errorf("expected 'value1', got '%s'", val)
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := cache.New[string](5 * time.Minute)

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected cache miss for nonexistent key")
	}
}

func TestCache_Expiration(t *testing.T) {
	c := cache.New[string](50 * time.Millisecond)

	c.Set("key1", "value1")
	time.Sleep(100 * time.Millisecond)

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected cache entry to be expired")
	}
}

func TestCache_Delete(t *testing.T) {
	c := cache.New[string](5 * time.Minute)

	c.Set("key1", "value1")
	c.Delete("key1")

	_, ok := c.Get("key1")
	if ok {
		t.Fatal("expected key to be deleted")
	}
}
