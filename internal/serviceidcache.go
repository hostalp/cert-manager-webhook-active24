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
	"sync"
	"time"
)

// ServiceIDCacheConfig represents a service ID cache configuration, exported to be accessible from another packages
type ServiceIDCacheConfig struct {
	MaxSize int
	Ttl     time.Duration
}

// cacheEntry represents a cached service ID with expiration tracking
type cacheEntry struct {
	serviceID    int
	expiryTime   time.Time
	lastUsedTime time.Time
}

// ServiceIDCache is an LRU cache for service IDs with expiration
type ServiceIDCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
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
	SetDefaultConfig(&cacheCfg.MaxSize, 100)
	SetDefaultConfig(&cacheCfg.Ttl, 47*24*time.Hour) // 47 days

	return &ServiceIDCache{
		entries: make(map[string]*cacheEntry),
		maxSize: cacheCfg.MaxSize,
		ttl:     cacheCfg.Ttl,
	}
}

// Get retrieves a service ID from the cache if it exists and hasn't expired
func (c *ServiceIDCache) Get(domain string) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.entries[domain]
	if !exists {
		return 0, false
	}

	// Check if expired
	if time.Now().After(entry.expiryTime) {
		delete(c.entries, domain)
		return 0, false
	}

	// Update last used time
	entry.lastUsedTime = time.Now()
	return entry.serviceID, true
}

// Put stores a service ID in the cache
func (c *ServiceIDCache) Put(domain string, serviceID int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// If cache is full, remove expired or LRU entry
	if len(c.entries) >= c.maxSize {
		c.removeExpiredUnlocked(now)

		// If still full, remove LRU
		if len(c.entries) >= c.maxSize {
			c.removeLRUUnlocked()
		}
	}

	c.entries[domain] = &cacheEntry{
		serviceID:    serviceID,
		expiryTime:   now.Add(c.ttl),
		lastUsedTime: now,
	}
}

// removeExpiredUnlocked removes all expired entries (must be called with lock held)
func (c *ServiceIDCache) removeExpiredUnlocked(now time.Time) {
	for domain, entry := range c.entries {
		if now.After(entry.expiryTime) {
			delete(c.entries, domain)
		}
	}
}

// removeLRUUnlocked removes the least recently used entry (must be called with lock held)
func (c *ServiceIDCache) removeLRUUnlocked() {
	var lruDomain string
	var lruTime time.Time

	for domain, entry := range c.entries {
		if lruTime.IsZero() || entry.lastUsedTime.Before(lruTime) {
			lruDomain = domain
			lruTime = entry.lastUsedTime
		}
	}

	if lruDomain != "" {
		delete(c.entries, lruDomain)
	}
}

// serviceIDCache is the global cache instance
var serviceIDCache *ServiceIDCache
var cacheMutex sync.Mutex

// InitServiceIDCache initializes the global service ID cache
// This should be called once during application startup before the cache is used
func InitServiceIDCache(cacheCfg ServiceIDCacheConfig) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	serviceIDCache = NewServiceIDCache(cacheCfg)
}
