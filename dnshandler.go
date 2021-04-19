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

func (handler DnsHandler) cache(name string, typ uint16) {
	result, onlyCname := handler.localCache.Get(dns.Name(name), dns.Type(typ))
	if result == nil || onlyCname {
		dnsRes, source, _ := handler.dnsPool.Do(pool.Message{Name: name, Type: typ})

		if dnsRes != nil {
			go fmt.Printf("%s (T:%s) found in %s (P:%d) with %d answers\n", name, dns.TypeToString[typ], source.Name, source.Priority, len(dnsRes.Answer))

			if dnsRes.Rcode == dns.RcodeSuccess {
				go func() {
					if len(dnsRes.Answer) > 0 {
						handler.localCache.Set(dns.Name(name), dnsRes.Answer, true)
					}
				}()
			}
		}
	}
}

func (handler DnsHandler) Do(question *dns.Question) (*dns.Msg, error) {
	go fmt.Printf("Searching for %s with record type %s\n", question.Name, dns.TypeToString[question.Qtype])

	result, onlyCname := handler.localCache.Get(dns.Name(question.Name), dns.Type(question.Qtype))

	if result != nil && !onlyCname {
		go fmt.Printf("%s found in local cache with %d answers\n", question.Name, len(result))
		return &dns.Msg{Answer: result}, nil
	}

	var dnsRes *dns.Msg
	dnsRes, source, _ := handler.dnsPool.Do(pool.Message{Name: question.Name, Type: question.Qtype})

	go func(name string, qType uint16) {
		switch qType {
		case dns.TypeA:
			fallthrough
		case dns.TypeAAAA:
			fallthrough
		case dns.TypeCNAME:
			fallthrough
		case dns.TypeMX:
			fallthrough
		case dns.TypeNS:
			fallthrough
		case dns.TypeTXT:
			go handler.cache(name, qType)
		}
	}(question.Name, question.Qtype)

	// Catch when all of the upstream resolvers fail
	if dnsRes != nil {
		go fmt.Printf("%s (T:%s) found in %s (P:%d) with %d answers\n", question.Name, dns.TypeToString[question.Qtype], source.Name, source.Priority, len(dnsRes.Answer))

		if dnsRes.Rcode == dns.RcodeSuccess {
			go func() {
				if len(dnsRes.Answer) > 0 {
					handler.localCache.Set(dns.Name(question.Name), dnsRes.Answer, false)
				}
			}()

			return dnsRes, nil
		}
	}

	return nil, errors.New("nxdomain")
}
