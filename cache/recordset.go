package cache

import (
	"github.com/miekg/dns"
	"time"
)

type Record struct {
	RR      dns.RR
	Expires time.Time
}

type RecordSet struct {
	Records []*Record
	Expires time.Time
}

func (records *RecordSet) clean() {
	now := time.Now()

	// Filter the Record set
	newPos := 0
	for _, record := range records.Records {
		if record.Expires.After(now) {
			records.Records[newPos] = record
			newPos++
		}
	}
	records.Records = records.Records[:newPos]
}

func (records *RecordSet) Add(record *Record) {
	records.Records = append(records.Records, record)

	if records.Expires.Before(record.Expires) {
		records.Expires = record.Expires
	}
}
