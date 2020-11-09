package kocache

import (
	"time"

	"github.com/pkg/errors"
)

// ErrGetCacheTimeout is an error of timeout getting cache.
var ErrGetCacheTimeout = errors.New("get cache timeout")

// Entry is cache entry.
type Entry struct {
	lock     chan struct{} // lock for fetch
	value    interface{}
	err      error
	expireAt time.Time // zero means no-expiration
}

// Get gets cache.
func (ce *Entry) Get(dst interface{}) (interface{}, error) {
	return ce.GetWithTimeout(dst, -1)
}

// GetWithTimeout gets cache indicating timeout.
func (ce *Entry) GetWithTimeout(dst interface{}, timeout time.Duration) (interface{}, error) {
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
func (ce *Entry) Expired(now time.Time) bool {
	return !ce.expireAt.IsZero() && now.After(ce.expireAt)
}
