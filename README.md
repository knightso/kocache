# kocache
Single flight cache.

While one request is trying to fetch data to cache, subsequent requests wait for it.

# Usage

## Initialization

```Go
cache, err := kocache.New[string, string]()
```

## Get

```Go
value, err := cache.Get(key)
```

## Cache

```Go
resolve := cache.Reserve(key)

data, err := fetchSomething();
resolve(data, err)
if err != nil {
    return err
}
```

## Stats

```Go
stats := cache.Stats()
fmt.Printf("%+v", stats) // {Hits:123 Misses:456}
```

# Documentation

Full docs are available on [Godoc](https://pkg.go.dev/github.com/knightso/kocache)

And you can see the design concept at [Qiita](https://qiita.com/hogedigo/items/21283e922b321be90aa4)  (Sorry, but it's Japanese)

# Dependencies(Thanks)

* kocache wraps [hashicorp/golang-lru](https://github.com/hashicorp/golang-lru) inside.
