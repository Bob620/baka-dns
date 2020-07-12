package cache

import (
	"github.com/miekg/dns"
	"sync"
	"time"
)

type Domain struct {
	Expires time.Time
	records map[dns.Type]*RecordSet
	mutex   sync.RWMutex
}

func createDomain() *Domain {
	return &Domain{
		Expires: time.Time{},
		records: make(map[dns.Type]*RecordSet, 1),
		mutex:   sync.RWMutex{},
	}
}

func (domain *Domain) clean() {
	now := time.Now()

	domain.mutex.Lock()
	for i, record := range domain.records {
		if record.Expires.Before(now) {
			delete(domain.records, i)
		}
	}
	domain.mutex.Unlock()
}

func (domain *Domain) delete(dnsType dns.Type) {
	domain.mutex.Lock()
	delete(domain.records, dnsType)
	domain.mutex.Unlock()
}

func (domain *Domain) Set(dnsType dns.Type, records *RecordSet) {
	domain.mutex.Lock()
	domain.records[dnsType] = records
	if domain.Expires.Before(records.Expires) {
		domain.Expires = records.Expires
	}
	domain.mutex.Unlock()
}

func (domain *Domain) Get(dnsType dns.Type) *RecordSet {
	domain.mutex.RLock()
	records := domain.records[dnsType]
	domain.mutex.RUnlock()
	return records
}
