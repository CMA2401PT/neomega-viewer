package bytecache

import "container/list"

type cacheItem[K comparable, V any] struct {
	key   K
	value V
	size  int
}

type Cache[K comparable, V any] struct {
	// 缓存由调用方同步，来源 goroutine 和 View 各自管理其生命周期。
	limit, size int
	order       *list.List
	items       map[K]*list.Element
}

func New[K comparable, V any](limit int) *Cache[K, V] {
	return &Cache[K, V]{
		limit: limit,
		order: list.New(),
		items: map[K]*list.Element{},
	}
}
func (c *Cache[K, V]) Get(key K) (V, bool) {
	if e, ok := c.items[key]; ok {
		c.order.MoveToFront(e)
		return e.Value.(cacheItem[K, V]).value, true
	}
	var zero V
	return zero, false
}
func (c *Cache[K, V]) Put(key K, value V, size int) {
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
	c.items[key] = c.order.PushFront(cacheItem[K, V]{
		key:   key,
		value: value,
		size:  size,
	})
	c.size += size
}
