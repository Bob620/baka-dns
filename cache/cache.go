package cache

import (
	"github.com/miekg/dns"
	"sync"
	"time"
)

type Cache struct {
	expireOrder []dns.Name
	domains     map[dns.Name]*Domain
	size        int
	mutex       sync.RWMutex
}

func MakeCache(size int) *Cache {
	return &Cache{
		expireOrder: make([]dns.Name, size)[:0],
		domains:     make(map[dns.Name]*Domain, size),
		size:        size,
	}
}

func (cache *Cache) clean() {
	now := time.Now()

	// Filter the cache
	newPos := 0

	cache.mutex.Lock()
	for _, domainName := range cache.expireOrder {
		if domainName == "" {
			continue
		}

		if !cache.domains[domainName].Expires.After(now) {
			delete(cache.domains, domainName)
		} else {
			cache.expireOrder[newPos] = domainName
			newPos++
		}
	}
	cache.expireOrder = cache.expireOrder[:newPos]
	cache.mutex.Unlock()
}

func (cache *Cache) deleteFirst() {
	var domainName dns.Name
	cache.mutex.Lock()
	domainName, cache.expireOrder = cache.expireOrder[0], cache.expireOrder[1:]
	delete(cache.domains, domainName)
	cache.mutex.Unlock()
}

func (cache *Cache) Get(domainName dns.Name, recordType dns.Type) []dns.RR {
	now := time.Now()

	cache.mutex.RLock()
	domain := cache.domains[domainName]
	cache.mutex.RUnlock()

	if domain == nil {
		return nil
	}

	if domain.Expires.Before(now) {
		cache.clean()
		return nil
	}

	records := domain.Records[recordType]
	if records == nil {
		return nil
	}

	if records.Expires.Before(now) {
		domain.delete(recordType)
		return nil
	}

	records.clean()
	rrs := make([]dns.RR, len(records.Records))
	for i, record := range records.Records {
		record.RR.Header().Ttl = uint32(record.Expires.Sub(time.Now()) / time.Second)
		rrs[i] = record.RR
	}

	return rrs

}

func (cache *Cache) Set(domainName dns.Name, records []dns.RR) {
	now := time.Now()

	cache.mutex.Lock()
	domain := cache.domains[domainName]
	if domain == nil {
		if len(cache.expireOrder) >= cache.size {
			cache.deleteFirst()
		}

		domain = &Domain{map[dns.Type]*RecordSet{}, time.Now()}
	}

	recordSet := domain.Records
	cleanTypes := make(map[dns.Type]bool)

	for _, record := range records {
		header := record.Header()
		recordType := dns.Type(header.Rrtype)
		ttl := now.Add(time.Second * time.Duration(header.Ttl))

		set := recordSet[recordType]
		if set == nil || cleanTypes[recordType] == false {
			cleanTypes[recordType] = true
			set = &RecordSet{
				Records: make([]*Record, 1)[:0],
				Expires: time.Now(),
			}
		}

		set.Add(&Record{
			RR:      record,
			Expires: ttl,
		})

		if domain.Expires.Before(set.Expires) {
			domain.Expires = set.Expires
		}

		recordSet[recordType] = set
	}

	cache.expireOrder = append(cache.expireOrder, domainName)
	cache.domains[domainName] = domain
	cache.mutex.Unlock()
}
