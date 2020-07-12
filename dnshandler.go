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
			go fmt.Printf("%s (T:%s) found in [%s]:%s (P:%d) with %d answers\n", name, dns.TypeToString[typ], source.Address, source.Port, source.Priority, len(dnsRes.Answer))

			if dnsRes.Rcode == dns.RcodeSuccess {
				go func() {
					if len(dnsRes.Answer) > 0 {
						handler.localCache.Set(dns.Name(name), dnsRes.Answer)
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
		if qType != dns.TypeCNAME {
			go handler.cache(name, dns.TypeCNAME)
		}
		if qType != dns.TypeNS {
			go handler.cache(name, dns.TypeNS)
		}
		if qType != dns.TypeA {
			go handler.cache(name, dns.TypeA)
		}
		if qType != dns.TypeAAAA {
			go handler.cache(name, dns.TypeAAAA)
		}
	}(question.Name, question.Qtype)

	// Catch when all of the upstream resolvers fail
	if dnsRes != nil {
		go fmt.Printf("%s (T:%s) found in [%s]:%s (P:%d) with %d answers\n", question.Name, dns.TypeToString[question.Qtype], source.Address, source.Port, source.Priority, len(dnsRes.Answer))

		if dnsRes.Rcode == dns.RcodeSuccess {
			go func() {
				if len(dnsRes.Answer) > 0 {
					handler.localCache.Set(dns.Name(question.Name), dnsRes.Answer)
				}
			}()

			return dnsRes, nil
		}
	}

	return nil, errors.New("nxdomain")

	/*
		// Spawn a goroutine to handle async tasks
		go func(resolveWith chan []dns.RR) {
			// Start the redis query
			//var redisResponse <-chan RedisResponse
			//if handler.redisPool != nil {
			//	redisResponse = handler.redisPool.Get(fqdn)
			//}

			resolved := handler.localCache.Get(fqdn)
			var resolvedd []dns.RR
			source := "local" // Default to local source
			//var ttl uint32

			if resolved == "" {

				// Setup the dns message pre-maturely while we wait for redis to resolve
				m := new(dns.Msg)
				m.SetQuestion(dns.Fqdn(fqdn), dns.TypeA)
				var dnsRes *dns.Msg
				source = "redis"
				//ttl = 300 // Set redis grabs to 5 min ttl in local cache

				/*
					// Resolve redis
					var response RedisResponse
					if redisResponse != nil {
						response = <-redisResponse
					}
					resolved = response.data
	*/
	/*
			// If we can't resolve via cache look in upstream
			if resolved == "" {
				fmt.Println(host, "not found in local, querying remote...")
				dnsRes, source, _ = handler.dnsPool.Do(*m)

				// Catch when all of the upstream resolvers fail
				if dnsRes == nil {
					dnsRes = &dns.Msg{}
				}

				if len(dnsRes.Answer) > 0 {
					resolvedd = dnsRes.Answer
				}
			}
		}

		// If we got a non-local resolve, cache it until ttl
		if resolvedd != nil {
			// Resolve the request before we try to contact redis (redis is slower than acceptable)
			fmt.Println(fqdn, "found in", source, "as", resolved)
			resolveWith <- resolvedd
			/*
				if source != "local" {
					handler.localCache.Set(fqdn, resolved, time.Duration(ttl)*time.Second)
				}

				if handler.redisPool != nil && source != "local" && source != "redis" {
					result := <-handler.redisPool.SetEx(fqdn, resolved, ttl)

					// Set the dns answer with the ttl as it's expiration
					if result.err != nil {
						fmt.Println("Unable to cache")
					} else {
						fmt.Println(host, "cached")
					}
				}*/ /*
			} else {
				fmt.Println("Unable to resolve", fqdn)
				resolveWith <- nil
				resolved = "Unable to resolve"
			}
		}(resolveWith)
		// Resolve the goroutine
		rr := <-resolveWith

		if rr != nil {
			return rr, nil
		}
		return nil, errors.New("unable to resolve")
	*/
}
