package kocache

import (
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
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
type Cache[K comparable, V any] struct {
	cache *lru.Cache[K, *entry[V]]
	opts  options
	stats Stats
}

// options describes option list
type options struct {
	size            int
	withStats       bool
	defaultLifetime time.Duration
}

// Stats describes cache hits&misses statistics.
type Stats struct {
	Hits   uint32
	Misses uint32
}

// An Option is an option for a kocache
type Option interface {
	Apply(opts *options)
}

// WithStats returns an Option that enables cache statistics.
func WithStats() Option {
	return withStats{}
}

type withStats struct {
}

func (w withStats) Apply(opts *options) {
	opts.withStats = true
}

// WithSize returns an Option that defines cache size.
func WithSize(size int) Option {
	return withSize{size}
}

type withSize struct {
	size int
}

func (w withSize) Apply(opts *options) {
	opts.size = w.size
}

// WithDefaultLifetime returns an Option that defines cache default lifetime.
func WithDefaultLifetime(defaultLifetime time.Duration) Option {
	return withDefaultLifetime{defaultLifetime}
}

type withDefaultLifetime struct {
	defaultLifetime time.Duration
}

func (w withDefaultLifetime) Apply(opts *options) {
	opts.defaultLifetime = w.defaultLifetime
}

// New creates a new Cache.
func New[K comparable, V any](opts ...Option) (*Cache[K, V], error) {
	c := &Cache[K, V]{
		opts: options{
			size:            DefaultSize,
			withStats:       false,
			defaultLifetime: -1, // no expiration
		},
	}

	for _, opt := range opts {
		opt.Apply(&c.opts)
	}

	inner, err := lru.New[K, *entry[V]](c.opts.size)
	if err != nil {
		return nil, err
	}

	c.cache = inner

	return c, nil
}

// Get gets a cache value by a key.
// It returns ErrEntryNotFound if entry is not registered.
func (c *Cache[K, V]) Get(key K) (value V, err error) {
	return c.GetWithTimeout(key, -1)
}

// GetWithTimeout gets a cache value by a key, indicating timeout of other's fetch.
// It returns ErrEntryNotFound if entry is not registered, ErrGetCacheTimeout on timeout, and ErrExpired if expired.
func (c *Cache[K, V]) GetWithTimeout(key K, timeout time.Duration) (value V, err error) {
	entity := c.getEntry(key)
	if entity == nil {
		return value, ErrEntryNotFound
	}
	if entity.Expired(time.Now()) {
		return value, ErrExpired
	}
	return entity.getWithTimeout(timeout)
}

// Len returns the number of entries in the cache.
func (c *Cache[K, V]) Len() int {
	return c.cache.Len()
}

// Stats retuns statistics of the cache
func (c *Cache[K, V]) Stats() Stats {
	return c.stats
}

// ResolveFunc describes function which resolves cache.
type ResolveFunc[V any] func(entity V, err error)

// Reserve reserves cache entry to fetch.
// Caller must try fetch the value and call resolveFunc to set result.
// Reserve must be called jsut once. It will panic if called two or more times.
func (c *Cache[K, V]) Reserve(key K) ResolveFunc[V] {
	return c.ReserveWithLifetime(key, c.opts.defaultLifetime)
}

// ReserveWithLifetime reserves cache entry to fetch indicating its lifetime.
// Caller must try fetch the value and call resolveFunc to set result, otherwise others will wait until timeout.
// ReserveWithLifetime  must be called jsut once. It will panic if called two or more times.
func (c *Cache[K, V]) ReserveWithLifetime(key K, lifetime time.Duration) ResolveFunc[V] {
	entry := &entry[V]{lock: make(chan struct{})}

	var mux sync.Mutex
	reserved := false

	resolve := func(entity V, err error) {
		mux.Lock()
		defer mux.Unlock()

		if reserved {
			panic("already reserved")
		}
		reserved = true

		entry.value, entry.err = entity, err

		if lifetime >= 0 {
			entry.expireAt = time.Now().Add(lifetime)
		}

		close(entry.lock)
		entry.lock = nil // set nil to save memory
	}

	c.cache.Add(key, entry)

	return resolve
}

func (c *Cache[K, V]) getEntry(key K) *entry[V] {
	v, ok := c.cache.Get(key)

	if c.opts.withStats {
		if ok {
			atomic.AddUint32(&c.stats.Hits, 1)
		} else {
			atomic.AddUint32(&c.stats.Misses, 1)
		}
	}

	if !ok {
		return nil
	}

	return v
}

// entry is cache entry.
type entry[V any] struct {
	lock     chan struct{} // lock for fetch
	value    V
	err      error
	expireAt time.Time // zero means no-expiration
}

// get gets cache.
func (ce *entry[V]) get() (V, error) {
	return ce.getWithTimeout(-1)
}

// getWithTimeout gets cache indicating timeout.
func (ce *entry[V]) getWithTimeout(timeout time.Duration) (v V, err error) {
	if lock := ce.lock; lock != nil { // nil lock means cache is ready
		if timeout < 0 { // no timeout
			<-lock
		} else {
			select {
			case <-lock:
			case <-time.After(timeout):
				return v, ErrGetCacheTimeout
			}
		}
	}

	if ce.err != nil {
		return v, ce.err
	}

	return ce.value, nil
}

// Expired returns true if cache expired.
func (ce *entry[V]) Expired(now time.Time) bool {
	return !ce.expireAt.IsZero() && now.After(ce.expireAt)
}
