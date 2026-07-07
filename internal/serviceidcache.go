/*
Copyright 2026 Petr H

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package internal

import (
	"container/list"
	"sync"
	"time"
)

// Default service ID cache configuration
const (
	defaultServiceIDCacheMaxSize = 100
	defaultServiceIDCacheTTL     = 47 * 24 * time.Hour // 47 days
)

// ServiceIDCacheConfig represents a service ID cache configuration, exported to be accessible from another packages
type ServiceIDCacheConfig struct {
	MaxSize int
	Ttl     time.Duration
}

// serviceIDCacheEntry represents a cached service ID with expiration tracking
type serviceIDCacheEntry struct {
	domain     string
	serviceID  int
	expiryTime time.Time
}

// ServiceIDCache is an LRU cache for service IDs with expiration
type ServiceIDCache struct {
	mu       sync.RWMutex
	maxSize  int
	ttl      time.Duration
	ll       *list.List               // front = most recently used
	elements map[string]*list.Element // domain -> list element
}

// SetDefaultConfig is a simple helper function for setting default configuration (can't handle valid "0" values or boolean types)
func SetDefaultConfig[T comparable](val *T, def T) {
	var zero T
	if *val == zero {
		*val = def
	}
}

// NewServiceIDCache creates a new service ID cache
func NewServiceIDCache(cacheCfg ServiceIDCacheConfig) *ServiceIDCache {

	// Set default cache configuration
	SetDefaultConfig(&cacheCfg.MaxSize, defaultServiceIDCacheMaxSize)
	SetDefaultConfig(&cacheCfg.Ttl, defaultServiceIDCacheTTL)

	return &ServiceIDCache{
		maxSize:  cacheCfg.MaxSize,
		ttl:      cacheCfg.Ttl,
		ll:       list.New(),
		elements: make(map[string]*list.Element),
	}
}

// Get retrieves a service ID from the cache if it exists and hasn't expired
func (c *ServiceIDCache) Get(domain string) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, exists := c.elements[domain]
	if !exists {
		return 0, false
	}

	entry := element.Value.(*serviceIDCacheEntry)
	// Check if expired
	if time.Now().After(entry.expiryTime) {
		c.removeElement(element)
		return 0, false
	}

	// Mark as most recently used
	c.ll.MoveToFront(element)
	return entry.serviceID, true
}

// Set adds or replaces a service ID in the cache
func (c *ServiceIDCache) Set(domain string, serviceID int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// if the entry exists, replace it
	if element, exists := c.elements[domain]; exists {
		entry := element.Value.(*serviceIDCacheEntry)
		entry.serviceID = serviceID
		entry.expiryTime = now.Add(c.ttl)
		c.ll.MoveToFront(element)
		return
	}

	// If cache is full, remove expired or LRU entry
	if c.ll.Len() >= c.maxSize {
		if c.removeExpiredUnlocked(now) <= 0 {
			c.removeLRUUnlocked()
		}
	}

	entry := &serviceIDCacheEntry{
		domain:     domain,
		serviceID:  serviceID,
		expiryTime: now.Add(c.ttl),
	}
	c.elements[domain] = c.ll.PushFront(entry)
}

// removeExpiredUnlocked removes all expired entries and returns the count of removed entries (must be called with lock held)
func (c *ServiceIDCache) removeExpiredUnlocked(now time.Time) int {
	removedCount := 0
	var next *list.Element
	for element := c.ll.Front(); element != nil; element = next {
		next = element.Next()
		entry := element.Value.(*serviceIDCacheEntry)
		if now.After(entry.expiryTime) {
			c.removeElement(element)
			removedCount++
		}
	}
	return removedCount
}

// removeLRUUnlocked removes the least recently used entry (must be called with lock held)
func (c *ServiceIDCache) removeLRUUnlocked() {
	if element := c.ll.Back(); element != nil {
		c.removeElement(element)
	}
}

// removeElement removes an element from both the list and the map (must be called with lock held)
func (c *ServiceIDCache) removeElement(element *list.Element) {
	c.ll.Remove(element)
	entry := element.Value.(*serviceIDCacheEntry)
	delete(c.elements, entry.domain)
}

// Len returns the current number of entries held in the cache (may include expired but not yet removed entries)
func (c *ServiceIDCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}

// serviceIDCache is the global cache instance
var (
	serviceIDCache      *ServiceIDCache
	serviceIDCacheMutex sync.Mutex
)

// InitServiceIDCache initializes the global service ID cache
// This should be called once during application startup before the cache is used
func InitServiceIDCache(cacheCfg ServiceIDCacheConfig) {
	serviceIDCacheMutex.Lock()
	defer serviceIDCacheMutex.Unlock()
	serviceIDCache = NewServiceIDCache(cacheCfg)
}
