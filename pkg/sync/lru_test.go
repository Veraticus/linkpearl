package sync

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLRUCache(t *testing.T) {
	t.Run("with positive size", func(t *testing.T) {
		cache, err := newLRUCache(100)
		require.NoError(t, err)
		require.NotNil(t, cache)
		assert.Equal(t, 100, cache.size)
		assert.Equal(t, 0, cache.Len())
	})

	t.Run("with zero size uses default", func(t *testing.T) {
		cache, err := newLRUCache(0)
		require.NoError(t, err)
		require.NotNil(t, cache)
		assert.Equal(t, 1000, cache.size)
	})

	t.Run("with negative size uses default", func(t *testing.T) {
		cache, err := newLRUCache(-1)
		require.NoError(t, err)
		require.NotNil(t, cache)
		assert.Equal(t, 1000, cache.size)
	})
}

func TestLRUCacheAddGet(t *testing.T) {
	cache, err := newLRUCache(3)
	require.NoError(t, err)

	// Add items
	cache.Add("key1", "value1")
	cache.Add("key2", "value2")
	cache.Add("key3", "value3")

	// Get items
	val, ok := cache.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)

	val, ok = cache.Get("key2")
	assert.True(t, ok)
	assert.Equal(t, "value2", val)

	val, ok = cache.Get("key3")
	assert.True(t, ok)
	assert.Equal(t, "value3", val)

	// Non-existent key
	val, ok = cache.Get("key4")
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestLRUCacheEviction(t *testing.T) {
	cache, err := newLRUCache(3)
	require.NoError(t, err)

	// Fill cache
	cache.Add("key1", "value1")
	cache.Add("key2", "value2")
	cache.Add("key3", "value3")

	assert.Equal(t, 3, cache.Len())

	// Add another item - should evict key1 (oldest)
	cache.Add("key4", "value4")

	assert.Equal(t, 3, cache.Len())

	// key1 should be evicted
	_, ok := cache.Get("key1")
	assert.False(t, ok)

	// Others should still exist
	_, ok = cache.Get("key2")
	assert.True(t, ok)
	_, ok = cache.Get("key3")
	assert.True(t, ok)
	_, ok = cache.Get("key4")
	assert.True(t, ok)
}

func TestLRUCacheLRUOrder(t *testing.T) {
	cache, err := newLRUCache(3)
	require.NoError(t, err)

	// Add items
	cache.Add("key1", "value1")
	cache.Add("key2", "value2")
	cache.Add("key3", "value3")

	// Access key1 - moves it to front
	cache.Get("key1")

	// Add key4 - should evict key2 (now oldest)
	cache.Add("key4", "value4")

	// key2 should be evicted
	_, ok := cache.Get("key2")
	assert.False(t, ok)

	// key1 should still exist (was accessed)
	_, ok = cache.Get("key1")
	assert.True(t, ok)
}

func TestLRUCacheUpdate(t *testing.T) {
	cache, err := newLRUCache(3)
	require.NoError(t, err)

	// Add item
	cache.Add("key1", "value1")

	// Update with new value
	cache.Add("key1", "value2")

	// Should have new value
	val, ok := cache.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value2", val)

	// Should still have only one item
	assert.Equal(t, 1, cache.Len())
}

func TestLRUCacheContains(t *testing.T) {
	cache, err := newLRUCache(3)
	require.NoError(t, err)

	cache.Add("key1", "value1")

	assert.True(t, cache.Contains("key1"))
	assert.False(t, cache.Contains("key2"))
}

func TestLRUCacheRemove(t *testing.T) {
	cache, err := newLRUCache(3)
	require.NoError(t, err)

	cache.Add("key1", "value1")
	cache.Add("key2", "value2")

	// Remove existing key
	removed := cache.Remove("key1")
	assert.True(t, removed)
	assert.Equal(t, 1, cache.Len())
	assert.False(t, cache.Contains("key1"))

	// Remove non-existent key
	removed = cache.Remove("key3")
	assert.False(t, removed)
	assert.Equal(t, 1, cache.Len())
}

func TestLRUCacheClear(t *testing.T) {
	cache, err := newLRUCache(3)
	require.NoError(t, err)

	cache.Add("key1", "value1")
	cache.Add("key2", "value2")
	cache.Add("key3", "value3")

	assert.Equal(t, 3, cache.Len())

	cache.Clear()

	assert.Equal(t, 0, cache.Len())
	assert.False(t, cache.Contains("key1"))
	assert.False(t, cache.Contains("key2"))
	assert.False(t, cache.Contains("key3"))
}

func TestLRUCacheConcurrency(t *testing.T) {
	cache, err := newLRUCache(100)
	require.NoError(t, err)

	var wg sync.WaitGroup

	// Concurrent adds
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", i, j)
				cache.Add(key, j)
			}
		}(i)
	}

	// Concurrent gets
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key-%d-%d", i, j)
				cache.Get(key)
			}
		}(i)
	}

	// Concurrent removes
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				key := fmt.Sprintf("key-%d-%d", i, j)
				cache.Remove(key)
			}
		}(i)
	}

	wg.Wait()

	// Cache should be in valid state
	assert.LessOrEqual(t, cache.Len(), cache.size)
}

func TestLRUCacheTypes(t *testing.T) {
	cache, err := newLRUCache(10)
	require.NoError(t, err)

	// Test with different value types
	cache.Add("string", "value")
	cache.Add("int", 42)
	cache.Add("bool", true)
	cache.Add("struct", struct{ Name string }{Name: "test"})

	val, ok := cache.Get("string")
	assert.True(t, ok)
	assert.Equal(t, "value", val)

	val, ok = cache.Get("int")
	assert.True(t, ok)
	assert.Equal(t, 42, val)

	val, ok = cache.Get("bool")
	assert.True(t, ok)
	assert.Equal(t, true, val)

	val, ok = cache.Get("struct")
	assert.True(t, ok)
	assert.Equal(t, struct{ Name string }{Name: "test"}, val)
}
