package main

import "container/list"

type cacheItem[K comparable, V any] struct {
	key   K
	value V
	size  int
}

// byteCache is owned by its caller. HTTP caches use the server mutex; source
// caches are accessed exclusively by the file-owner goroutine.
type byteCache[K comparable, V any] struct {
	limit, size int
	order       *list.List
	items       map[K]*list.Element
}

func newCache[K comparable, V any](limit int) *byteCache[K, V] {
	return &byteCache[K, V]{limit: limit, order: list.New(), items: map[K]*list.Element{}}
}
func (c *byteCache[K, V]) get(key K) (V, bool) {
	if e, ok := c.items[key]; ok {
		c.order.MoveToFront(e)
		return e.Value.(cacheItem[K, V]).value, true
	}
	var zero V
	return zero, false
}
func (c *byteCache[K, V]) put(key K, value V, size int) {
	size += 128
	if old, ok := c.items[key]; ok {
		c.size -= old.Value.(cacheItem[K, V]).size
		c.order.Remove(old)
		delete(c.items, key)
	}
	if size > c.limit {
		return
	}
	for c.size+size > c.limit {
		e := c.order.Back()
		item := e.Value.(cacheItem[K, V])
		c.size -= item.size
		delete(c.items, item.key)
		c.order.Remove(e)
	}
	c.items[key] = c.order.PushFront(cacheItem[K, V]{key: key, value: value, size: size})
	c.size += size
}
