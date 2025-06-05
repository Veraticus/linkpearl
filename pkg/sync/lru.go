// lru.go implements a Least Recently Used (LRU) cache for message deduplication.
// This cache is critical for preventing sync loops and avoiding redundant
// processing of clipboard updates.
//
// Purpose:
//
// In a mesh network, messages can arrive via multiple paths, and our own
// changes can echo back through the network. The LRU cache tracks recently
// seen clipboard checksums to identify and discard duplicates.
//
// Implementation:
//
// The cache uses a doubly-linked list for O(1) access to both ends and a
// hash map for O(1) lookups. When the cache reaches capacity, the least
// recently used entries are evicted automatically.
//
// Thread Safety:
//
// All operations are protected by a mutex, making the cache safe for
// concurrent access from multiple goroutines. This is essential as the
// sync engine processes events from multiple sources simultaneously.
//
// Capacity Management:
//
// The cache size is configurable but defaults to 1000 entries. This provides
// a good balance between memory usage and deduplication effectiveness. With
// average clipboard content, this can track several hours of activity.
//
// Usage in Sync Engine:
//
// When a clipboard change is detected (local or remote), its checksum is
// added to the cache. Incoming messages are checked against the cache to
// determine if they're duplicates. The cache also stores timestamps to
// help detect rapid sync loops.

package sync

import (
	"container/list"
	"sync"
)

// lruCache implements a thread-safe LRU cache for deduplication.
// It maintains a fixed-size cache of recently seen items, automatically
// evicting the least recently used entries when capacity is reached.
type lruCache struct {
	size      int
	evictList *list.List
	items     map[string]*list.Element
	mu        sync.Mutex
}

// entry holds the key and value for an LRU entry.
// The key is stored redundantly to enable O(1) removal when evicting
// entries from the tail of the LRU list.
type entry struct {
	key   string
	value interface{}
}

// newLRUCache creates a new LRU cache with the given size.
// If size <= 0, it defaults to 1000 entries which provides a good balance
// between memory usage and deduplication effectiveness.
//
// The cache uses:
//   - A doubly-linked list to track access order (most recent at front)
//   - A hash map for O(1) lookups by key
//   - A mutex for thread-safe concurrent access
func newLRUCache(size int) (*lruCache, error) {
	if size <= 0 {
		size = 1000 // Default size
	}

	return &lruCache{
		size:      size,
		evictList: list.New(),
		items:     make(map[string]*list.Element),
	}, nil
}

// Add adds a key-value pair to the cache or updates an existing entry.
// If the key already exists, it's moved to the front (most recently used)
// and its value is updated.
//
// When the cache is at capacity, the least recently used entry is
// automatically evicted to make room for the new entry.
//
// This method is safe for concurrent use.
func (c *lruCache) Add(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	if elem, exists := c.items[key]; exists {
		// Move to front and update value
		c.evictList.MoveToFront(elem)
		elem.Value.(*entry).value = value
		return
	}

	// Add new entry
	ent := &entry{key: key, value: value}
	elem := c.evictList.PushFront(ent)
	c.items[key] = elem

	// Evict oldest if over capacity
	if c.evictList.Len() > c.size {
		c.removeOldest()
	}
}

// Get retrieves a value from the cache and marks it as recently used.
// If the key exists, it's moved to the front of the LRU list.
//
// Returns:
//   - value: The cached value (interface{} to support any type)
//   - exists: true if the key was found, false otherwise
//
// This method is safe for concurrent use.
func (c *lruCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		// Move to front (mark as recently used)
		c.evictList.MoveToFront(elem)
		return elem.Value.(*entry).value, true
	}

	return nil, false
}

// Contains checks if a key exists in the cache without updating its
// position in the LRU list. This is useful for existence checks when
// you don't want to affect the eviction order.
//
// This method is safe for concurrent use.
func (c *lruCache) Contains(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, exists := c.items[key]
	return exists
}

// Remove removes a key from the cache
func (c *lruCache) Remove(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.removeElement(elem)
		return true
	}

	return false
}

// Clear removes all entries from the cache
func (c *lruCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.evictList.Init()
}

// Len returns the number of items in the cache
func (c *lruCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.evictList.Len()
}

// removeOldest removes the oldest (least recently used) entry from the cache.
// This is called internally when the cache exceeds its capacity.
// The entry at the back of the list is the least recently accessed.
func (c *lruCache) removeOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}

// removeElement removes an element from both the linked list and hash map.
// This is a helper method used by both Remove() and removeOldest().
// It ensures the cache's internal data structures remain consistent.
func (c *lruCache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	ent := elem.Value.(*entry)
	delete(c.items, ent.key)
}
