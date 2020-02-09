package main

import (
	"time"
)

type Cache struct {
	keyArray  []string
	keyMap    map[string]string
	expireMap map[string]time.Time
	size      int
}

func MakeCache(size int) *Cache {
	return &Cache{[]string{}, map[string]string{}, map[string]time.Time{}, size}
}

func (cache *Cache) Get(key string) string {
	now := time.Now()
	expires := cache.expireMap[key]
	if expires.After(now) || expires.IsZero() {
		return cache.keyMap[key]
	} else if len(cache.keyArray) > 0 {
		expires := cache.expireMap[cache.keyArray[0]]
		for len(cache.keyArray) > 1 && expires.After(now) {
			cache.DeleteFirst()
			expires = cache.expireMap[cache.keyArray[0]]
		}
	}

	return ""
}

func (cache *Cache) DeleteFirst() {
	key := cache.keyArray[0]
	cache.keyArray = cache.keyArray[1:]

	deadKey := cache.keyMap[key]
	delete(cache.keyMap, deadKey)
	delete(cache.expireMap, deadKey)
}

func (cache *Cache) Set(key, value string, expires time.Duration) {
	length := len(cache.keyArray)

	if length+1 >= cache.size {
		numToRemove := cache.size / 10
		for i := 0; i < numToRemove; i++ {
			deadKey := cache.keyArray[i]
			delete(cache.keyMap, deadKey)
			delete(cache.expireMap, deadKey)
		}

		cache.keyArray = cache.keyArray[numToRemove:]
	}

	cache.keyArray = append(cache.keyArray, key)
	cache.keyMap[key] = value
	cache.expireMap[key] = time.Now().Add(expires)
}
