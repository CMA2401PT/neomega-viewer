package bytecache

import "testing"

func TestByteCacheEvictsLeastRecentlyUsed(t *testing.T) {
	cache := New[int, []byte](300)
	cache.Put(1, []byte{1}, 1)
	cache.Put(2, []byte{2}, 1)
	cache.Get(1)
	cache.Put(3, []byte{3}, 1)
	if _, ok := cache.Get(2); ok {
		t.Fatal("oldest entry retained")
	}
	if _, ok := cache.Get(1); !ok {
		t.Fatal("recent entry evicted")
	}
	cache.Put(4, make([]byte, 301), 301)
	if _, ok := cache.Get(4); ok {
		t.Fatal("oversized entry retained")
	}
}
