package cache

import (
	"github.com/miekg/dns"
	"sync"
	"time"
)

type Record struct {
	RR      dns.RR
	Expires time.Time
}

type RecordSet struct {
	Expires time.Time
	records []*Record
	mutex   sync.RWMutex
}

func createRecordSet() *RecordSet {
	return &RecordSet{
		Expires: time.Now(),
		records: make([]*Record, 1)[:0],
	}
}

func (records *RecordSet) clean() {
	now := time.Now()

	// Filter the Record set
	newPos := 0
	records.mutex.Lock()
	for _, record := range records.records {
		if record.Expires.After(now) {
			records.records[newPos] = record
			newPos++
		}
	}
	records.records = records.records[:newPos]
	records.mutex.Unlock()
}

func (records *RecordSet) Add(record *Record) {
	records.mutex.Lock()
	records.records = append(records.records, record)

	if records.Expires.Before(record.Expires) {
		records.Expires = record.Expires
	}
	records.mutex.Unlock()
}

func (records *RecordSet) GetRecords() []dns.RR {
	records.mutex.RLock()
	rrs := make([]dns.RR, len(records.records))
	for i, record := range records.records {
		record.RR.Header().Ttl = uint32(record.Expires.Sub(time.Now()) / time.Second)
		rrs[i] = record.RR
	}
	records.mutex.RUnlock()

	return rrs
}

func (records *RecordSet) Len() int {
	return len(records.records)
}
