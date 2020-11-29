# kocache
Single flight cache.

While one request is trying to fetch data to cache, subsequent requests wait for it.

# Usage

## Initialization

```Go
cache, err := kocache.New()
```

## Get

```Go
value, err := cache.Get(key)
```

## Cache

```Go
resolve := cache.Reserve(key)

(fetch data from)

resolve(data, nil)
```

# Documentation

Full docs are available on [Godoc](https://pkg.go.dev/github.com/knightso/kocache)

# Dependencies(Thanks)

* kocache wraps [hashicorp/golang-lru](https://github.com/hashicorp/golang-lru0) inside.
