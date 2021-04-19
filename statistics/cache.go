package statistics

import (
	"fmt"
	"sync/atomic"
	"time"
)

type Cache struct {
	hit             int64
	miss            int64
	insertion       int64
	eviction        int64
	requests        int64
	tangentRequests int64
	size            int64
	maxSize         int64
}

func MakeCache(maxSize int64) *Cache {
	cache := &Cache{maxSize: maxSize}
	var timed func()

	timed = func() {
		fmt.Printf("\nMax Size:\t%d\nSize:\t\t%d\nHits:\t\t%d\nMisses:\t\t%d\nInserts:\t%d\nEvicts:\t\t%d\n\nRequests:\t%d\nTangets:\t%d\n\n",
			cache.GetMax(), cache.GetSize(), cache.GetHits(), cache.GetMisses(), cache.GetInsertions(), cache.GetEvictions(), cache.GetRequests(), cache.GetTangentRequests(),
		)
		time.AfterFunc(time.Second*5, timed)
	}

	go timed()
	return cache
}

func (cache *Cache) Reset() {
	atomic.StoreInt64(&cache.hit, 0)
	atomic.StoreInt64(&cache.miss, 0)
	atomic.StoreInt64(&cache.insertion, 0)
	atomic.StoreInt64(&cache.eviction, 0)
	atomic.StoreInt64(&cache.size, 0)
	atomic.StoreInt64(&cache.requests, 0)
	atomic.StoreInt64(&cache.tangentRequests, 0)
}

func (cache *Cache) SetMax(newMax int64) {
	atomic.StoreInt64(&cache.maxSize, newMax)
}

func (cache *Cache) GetMax() int64 {
	return atomic.LoadInt64(&cache.maxSize)
}

func (cache *Cache) SetSize(newSize int64) {
	atomic.StoreInt64(&cache.size, newSize)
}

func (cache *Cache) GetSize() int64 {
	return atomic.LoadInt64(&cache.size)
}

func (cache *Cache) Hit() {
	atomic.AddInt64(&cache.hit, 1)
}

func (cache *Cache) GetHits() int64 {
	return atomic.LoadInt64(&cache.hit)
}

func (cache *Cache) Miss() {
	atomic.AddInt64(&cache.miss, 1)
}

func (cache *Cache) GetMisses() int64 {
	return atomic.LoadInt64(&cache.miss)
}

func (cache *Cache) Insert() {
	atomic.AddInt64(&cache.insertion, 1)
}

func (cache *Cache) GetInsertions() int64 {
	return atomic.LoadInt64(&cache.insertion)
}

func (cache *Cache) Evict() {
	atomic.AddInt64(&cache.eviction, 1)
}

func (cache *Cache) GetEvictions() int64 {
	return atomic.LoadInt64(&cache.eviction)
}

func (cache *Cache) Request() {
	atomic.AddInt64(&cache.requests, 1)
}

func (cache *Cache) GetRequests() int64 {
	return atomic.LoadInt64(&cache.requests)
}

func (cache *Cache) TangentRequest() {
	atomic.AddInt64(&cache.tangentRequests, 1)
}

func (cache *Cache) GetTangentRequests() int64 {
	return atomic.LoadInt64(&cache.tangentRequests)
}
