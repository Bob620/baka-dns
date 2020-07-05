package cache

import (
	"github.com/miekg/dns"
	"time"
)

type Domain struct {
	Records map[dns.Type]*RecordSet
	Expires time.Time
}

func (domain *Domain) clean() {
	now := time.Now()

	for i, record := range domain.Records {
		if record.Expires.Before(now) {
			delete(domain.Records, i)
		}
	}
}

func (domain *Domain) delete(dnsType dns.Type) {
	delete(domain.Records, dnsType)
}
