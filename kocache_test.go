// +build faketime

package kocache

import (
	"sync"
	"testing"
	"time"
)

func TestGetAndReserve(t *testing.T) {
	start := time.Now()

	cache, err := New()
	if err != nil {
		t.Fatal(err)
	}

	// create utility method to synchronize testing report.
	var mux sync.Mutex
	lock := func(f func()) {
		mux.Lock()
		defer mux.Unlock()
		f()
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		if _, err = cache.Get("key1"); err != ErrEntryNotFound {
			lock(func() { t.Errorf("ErrEntryNotFound expected, but was:%v", err) })
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(time.Nanosecond)

		resolve := cache.Reserve("key1")

		time.Sleep(5 * time.Nanosecond)

		resolve("value1", nil)
	}()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			time.Sleep(2 * time.Nanosecond)

			value, err := cache.Get("key1")
			if err != nil {
				lock(func() { t.Error(err) })
			}

			// assert waiting for cache
			if actual, expected := time.Now().Sub(start), 6*time.Nanosecond; actual != expected {
				lock(func() { t.Errorf("expected:%v, but was:%v", expected, actual) })
			}

			if actual, expected := value.(string), "value1"; actual != expected {
				lock(func() { t.Errorf("expected:%v, but was:%v", expected, actual) })
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		if _, err = cache.Get("key2"); err != ErrEntryNotFound {
			lock(func() { t.Fatalf("ErrEntryNotFound expected, but was:%v", err) })
		}
	}()

	time.Sleep(time.Second)

	wg.Wait()
}

func TestLifetime(t *testing.T) {
	var duration time.Duration
	sleepBy := func(d time.Duration) {
		time.Sleep(d - duration)
		duration = d
	}

	cache, err := New(WithDefaultLifetime(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	cache.Reserve("default")("default_value", nil)
	cache.ReserveWithLifetime("30sec", 30*time.Second)("30sec_value", nil)

	{
		value, err := cache.Get("default")
		if err != nil {
			t.Error(err)
		}

		if actual, expected := value.(string), "default_value"; actual != expected {
			t.Fatalf("value - expected:%s, but was:%s", expected, actual)
		}
	}

	{
		value, err := cache.Get("30sec")
		if err != nil {
			t.Error(err)
		}

		if actual, expected := value.(string), "30sec_value"; actual != expected {
			t.Fatalf("value - expected:%s, but was:%s", expected, actual)
		}
	}

	sleepBy(30 * time.Second)

	{
		value, err := cache.Get("default")
		if err != nil {
			t.Error(err)
		}

		if actual, expected := value.(string), "default_value"; actual != expected {
			t.Fatalf("value - expected:%s, but was:%s", expected, actual)
		}
	}

	{
		value, err := cache.Get("30sec")
		if err != nil {
			t.Error(err)
		}

		if actual, expected := value.(string), "30sec_value"; actual != expected {
			t.Fatalf("value - expected:%s, but was:%s", expected, actual)
		}
	}

	sleepBy(30*time.Second + time.Millisecond)

	{
		value, err := cache.Get("default")
		if err != nil {
			t.Error(err)
		}

		if actual, expected := value.(string), "default_value"; actual != expected {
			t.Fatalf("value - expected:%s, but was:%s", expected, actual)
		}
	}

	{
		_, err := cache.Get("30sec")
		if actual, expected := err, ErrExpired; actual != expected {
			t.Fatalf("err - expected:%s, but was:%s", expected, actual)
		}
	}

	sleepBy(time.Minute)

	{
		value, err := cache.Get("default")
		if err != nil {
			t.Error(err)
		}

		if actual, expected := value.(string), "default_value"; actual != expected {
			t.Fatalf("value - expected:%s, but was:%s", expected, actual)
		}
	}

	sleepBy(time.Minute + time.Millisecond)

	{
		_, err := cache.Get("default")
		if actual, expected := err, ErrExpired; actual != expected {
			t.Fatalf("err - expected:%s, but was:%s", expected, actual)
		}
	}
}

func TestTimeout(t *testing.T) {
	start := time.Now()

	var duration time.Duration
	sleepBy := func(d time.Duration) {
		time.Sleep(d - duration)
		duration = d
	}

	cache, err := New()
	if err != nil {
		t.Fatal(err)
	}

	key := "testkey"

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		resolve := cache.Reserve(key)

		time.Sleep(time.Second)

		resolve("testvalue", nil)
	}()

	sleepBy(time.Millisecond)

	_, err = cache.GetWithTimeout(key, 0)
	if actual, expected := err, ErrGetCacheTimeout; actual != expected {
		t.Fatalf("expected:%v, but was:%v", expected, actual)
	}

	_, err = cache.GetWithTimeout(key, 999*time.Millisecond)
	if actual, expected := err, ErrGetCacheTimeout; actual != expected {
		t.Fatalf("expected:%v, but was:%v", expected, actual)
	}

	if actual, expected := time.Now().Sub(start), time.Second; actual != expected {
		t.Fatalf("expected:%v, but was:%v", expected, actual)
	}

	sleepBy(time.Millisecond + 1)

	value, err := cache.GetWithTimeout(key, 0)
	if err != nil {
		t.Fatal(err)
	}
	if actual, expected := value.(string), "testvalue"; actual != expected {
		t.Fatalf("expected:%v, but was:%v", expected, actual)
	}
}

func TestWithSize(t *testing.T) {
	cache, err := New(WithSize(5))
	if err != nil {
		t.Fatal(err)
	}

	cache.Reserve("1")("value1", nil)
	cache.Reserve("2")("value2", nil)
	cache.Reserve("3")("value3", nil)
	cache.Reserve("4")("value4", nil)
	cache.Reserve("5")("value5", nil)

	if actual, expected := cache.Len(), 5; actual != expected {
		t.Fatalf("expected:%v, but was:%v", expected, actual)
	}

	// #### assert eviction
	cache.Reserve("6")("value5", nil)

	if actual, expected := cache.Len(), 5; actual != expected {
		t.Fatalf("expected:%v, but was:%v", expected, actual)
	}

	// assert entry 1 is evicted
	_, err = cache.Get("1")
	if err != ErrEntryNotFound {
		t.Fatalf("ErrEntryNotFound expected, but was:%v", err)
	}

	// assert LRU eviction
	{
		value, err := cache.Get("2")
		if err != nil {
			t.Fatal(err)
		}
		if actual, expected := value.(string), "value2"; actual != expected {
			t.Fatalf("expected:%v, but was:%v", expected, actual)
		}
	}

	cache.Reserve("7")("value5", nil)

	// 3 must be evicted instead of 2.
	_, err = cache.Get("3")
	if err != ErrEntryNotFound {
		t.Fatalf("ErrEntryNotFound expected, but was:%v", err)
	}

	{
		// 2 is still alive!
		value, err := cache.Get("2")
		if err != nil {
			t.Fatal(err)
		}
		if actual, expected := value.(string), "value2"; actual != expected {
			t.Fatalf("expected:%v, but was:%v", expected, actual)
		}
	}
}
