package kocache

import (
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/pkg/errors"
)

const (
	// DefaultSize is default cache size
	DefaultSize = 1024
)

var (
	// ErrEntryNotFound is an error that describes entry not found.
	ErrEntryNotFound = errors.New("entry not found")

	// ErrExpired is an error that describes cache expired
	ErrExpired = errors.New("expired")

	// ErrGetCacheTimeout is an error of timeout getting cache.
	ErrGetCacheTimeout = errors.New("get cache timeout")
)

// Cache is single flight cache
type Cache struct {
	cache           *lru.Cache
	size            int
	withStats       bool
	stats           Stats
	defaultLifetime time.Duration
}

// Stats describes cache hits&misses statistics.
type Stats struct {
	Hits   uint32
	Misses uint32
}

// An Option is an option for a kocache
type Option interface {
	Apply(cache *Cache)
}

// WithStats returns an Option that enables cache statistics.
func WithStats() Option {
	return withStats{}
}

type withStats struct {
}

func (w withStats) Apply(cache *Cache) {
	cache.withStats = true
}

// WithSize returns an Option that defines cache size.
func WithSize(size int) Option {
	return withSize{size}
}

type withSize struct {
	size int
}

func (w withSize) Apply(cache *Cache) {
	cache.size = w.size
}

// WithDefaultLifetime returns an Option that defines cache default lifetime.
func WithDefaultLifetime(defaultLifetime time.Duration) Option {
	return withDefaultLifetime{defaultLifetime}
}

type withDefaultLifetime struct {
	defaultLifetime time.Duration
}

func (w withDefaultLifetime) Apply(cache *Cache) {
	cache.defaultLifetime = w.defaultLifetime
}

// New creates a new Cache.
func New(opts ...Option) (*Cache, error) {
	c := &Cache{
		size:            DefaultSize,
		withStats:       false,
		defaultLifetime: -1, // no expiration
	}

	for _, opt := range opts {
		opt.Apply(c)
	}

	inner, err := lru.New(c.size)
	if err != nil {
		return nil, err
	}

	c.cache = inner

	return c, nil
}

// Get gets a cache value by a key.
// It returns ErrEntryNotFound if entry is not registered.
func (c *Cache) Get(key interface{}) (value interface{}, err error) {
	return c.GetWithTimeout(key, -1)
}

// GetWithTimeout gets a cache value by a key, indicating timeout of other's fetch.
// It returns ErrEntryNotFound if entry is not registered, and ErrGetCacheTimeout on timeout.
func (c *Cache) GetWithTimeout(key interface{}, timeout time.Duration) (value interface{}, err error) {
	entity := c.getEntry(key)
	if entity == nil {
		return nil, ErrEntryNotFound
	}
	if entity.Expired(time.Now()) {
		return nil, ErrExpired
	}
	return entity.getWithTimeout(key, timeout)
}

// Len returns the number of entries in the cache.
func (c *Cache) Len() int {
	return c.cache.Len()
}

// ResolveFunc describes function which resolves cache.
type ResolveFunc func(entity interface{}, err error)

// Reserve reserves cache entry to fetch.
// Caller must try fetch the value and call resolveFunc to set result.
func (c *Cache) Reserve(key interface{}) ResolveFunc {
	return c.ReserveWithLifetime(key, c.defaultLifetime)
}

// ReserveWithLifetime reserves cache entry to fetch indicating its lifetime.
// Caller must try fetch the value and call resolveFunc to set result, otherwise others will wait until timeout.
func (c *Cache) ReserveWithLifetime(key interface{}, lifetime time.Duration) ResolveFunc {
	entry := &entry{lock: make(chan struct{})}

	resolve := func(entity interface{}, err error) {
		entry.value, entry.err = entity, err

		if lifetime >= 0 {
			entry.expireAt = time.Now().Add(lifetime)
		}

		close(entry.lock)
	}

	c.cache.Add(key, entry)

	return resolve
}

func (c *Cache) getEntry(key interface{}) *entry {
	v, ok := c.cache.Get(key)

	if c.withStats {
		if ok {
			atomic.AddUint32(&c.stats.Hits, 1)
		} else {
			atomic.AddUint32(&c.stats.Misses, 1)
		}
	}

	if !ok {
		return nil
	}

	return v.(*entry)
}

// entry is cache entry.
type entry struct {
	lock     chan struct{} // lock for fetch
	value    interface{}
	err      error
	expireAt time.Time // zero means no-expiration
}

// get gets cache.
func (ce *entry) get(dst interface{}) (interface{}, error) {
	return ce.getWithTimeout(dst, -1)
}

// getWithTimeout gets cache indicating timeout.
func (ce *entry) getWithTimeout(dst interface{}, timeout time.Duration) (interface{}, error) {
	if timeout < 0 { // no timeout
		<-ce.lock
	} else {
		select {
		case <-ce.lock:
		case <-time.After(timeout):
			return nil, ErrGetCacheTimeout
		}
	}

	if ce.err != nil {
		return nil, ce.err
	}

	return ce.value, nil
}

// Expired returns true if cache expired.
func (ce *entry) Expired(now time.Time) bool {
	return !ce.expireAt.IsZero() && now.After(ce.expireAt)
}
