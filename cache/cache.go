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

		if cache.domains[domainName] == nil {
			delete(cache.domains, domainName)
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

func (cache *Cache) getDomain(domainName dns.Name) *Domain {
	now := time.Now()

	cache.mutex.RLock()
	domain := cache.domains[domainName]
	cache.mutex.RUnlock()

	if domain != nil && domain.Expires.Before(now) {
		cache.clean()
		return nil
	}

	return domain
}

func (cache *Cache) setDomain(domainName dns.Name, domain *Domain) {
	cache.mutex.Lock()
	cache.expireOrder = append(cache.expireOrder, domainName)
	cache.domains[domainName] = domain
	cache.mutex.Unlock()
}

func (cache *Cache) Get(domainName dns.Name, recordType dns.Type) ([]dns.RR, bool) {
	var cnames *RecordSet
	now := time.Now()
	cnameType := dns.Type(dns.TypeCNAME)
	output := make([]dns.RR, 2)[:0]

	domain := cache.getDomain(domainName)

	if domain == nil {
		return nil, false
	}

	if recordType != cnameType {
		cnames = domain.Get(cnameType)
	}
	records := domain.Get(recordType)

	if cnames != nil {
		if cnames.Expires.Before(now) || cnames.Len() == 0 {
			domain.delete(cnameType)
			cnames = nil
		} else {
			cnames.clean()
			output = append(output, cnames.GetRecords()...)
		}
	}

	if records != nil {
		if records.Expires.Before(now) || records.Len() == 0 {
			domain.delete(recordType)
			records = nil
		} else {
			records.clean()
			output = append(output, records.GetRecords()...)
		}
	}

	if records == nil && cnames == nil {
		return nil, false
	}

	return output, records == nil
}

func (cache *Cache) Set(domainName dns.Name, records []dns.RR) {
	now := time.Now()

	domain := cache.getDomain(domainName)
	if domain == nil {
		cache.clean()
		if len(cache.expireOrder) >= cache.size {
			cache.deleteFirst()
		}

		domain = createDomain()
	}

	cleanTypes := make(map[dns.Type]bool)

	for _, record := range records {
		header := record.Header()
		recordType := dns.Type(header.Rrtype)
		ttl := now.Add(time.Second * time.Duration(header.Ttl))

		set := domain.Get(recordType)
		if set == nil || cleanTypes[recordType] == false {
			cleanTypes[recordType] = true
			set = createRecordSet()
		}

		set.Add(&Record{
			RR:      record,
			Expires: ttl,
		})

		if domain.Expires.Before(set.Expires) {
			domain.Expires = set.Expires
		}

		domain.Set(recordType, set)
	}

	cache.setDomain(domainName, domain)
}
