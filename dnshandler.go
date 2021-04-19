package main

import (
	"errors"
	"fmt"
	"github.com/Bob620/baka-dns/cache"
	"github.com/Bob620/baka-dns/upstream/pool"
	"github.com/miekg/dns"
)

type DnsHandler struct {
	redisPool  *RedisPool
	dnsPool    *pool.Pool
	localCache *cache.Cache
}

func MakeDNSHandler(redisPool *RedisPool, dnsPool *pool.Pool, localCache *cache.Cache) *DnsHandler {
	return &DnsHandler{redisPool, dnsPool, localCache}
}

func (handler DnsHandler) cache(name string, typ uint16, tangent bool) *dns.Msg {
	result, onlyCname := handler.localCache.Get(dns.Name(name), dns.Type(typ))
	if result == nil || onlyCname {
		dnsRes, source, _ := handler.dnsPool.Do(pool.Message{Name: name, Type: typ})

		if dnsRes != nil {
			go fmt.Printf("%s (T:%s) found in %s (P:%d) with %d answers\n", name, dns.TypeToString[typ], source.Name, source.Priority, len(dnsRes.Answer))

			if dnsRes.Rcode == dns.RcodeSuccess {
				go func() {
					if len(dnsRes.Answer) > 0 {
						handler.localCache.Set(dns.Name(name), dnsRes.Answer, tangent)
					}
				}()

				return dnsRes
			}
		}
	}

	return nil
}

func (handler DnsHandler) Do(question *dns.Question) (*dns.Msg, error) {
	go fmt.Printf("Searching for %s with record type %s\n", question.Name, dns.TypeToString[question.Qtype])

	// Local cache lookup
	result, onlyCname := handler.localCache.Get(dns.Name(question.Name), dns.Type(question.Qtype))

	if result != nil && !onlyCname {
		go fmt.Printf("%s found in local cache with %d answers\n", question.Name, len(result))
		return &dns.Msg{Answer: result}, nil
	}

	// Tangent queries
	go func(name string, qType uint16) {
		go handler.cache(name, dns.TypeA, true)
		go handler.cache(name, dns.TypeAAAA, true)
		go handler.cache(name, dns.TypeCNAME, true)
		go handler.cache(name, dns.TypeMX, true)
		go handler.cache(name, dns.TypeNS, true)
		go handler.cache(name, dns.TypeTXT, true)
	}(question.Name, question.Qtype)

	// Query upstream DNS
	dnsRes := handler.cache(question.Name, question.Qtype, false)

	// Catch if all of the upstream resolvers fail and return an error
	if dnsRes != nil {
		return dnsRes, nil
	}

	return nil, errors.New("nxdomain")
}
