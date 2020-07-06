package main

import (
	"fmt"
	"github.com/Bob620/baka-dns/cache"
	"github.com/Bob620/baka-dns/upstream"
	"github.com/miekg/dns"
)

type DnsHandler struct {
	redisPool  *RedisPool
	dnsPool    *upstream.Pool
	localCache *cache.Cache
}

func MakeDNSHandler(redisPool *RedisPool, dnsPool *upstream.Pool, localCache *cache.Cache) *DnsHandler {
	return &DnsHandler{redisPool, dnsPool, localCache}
}

func (handler DnsHandler) Do(question *dns.Question) ([]dns.RR, error) {
	fmt.Println("Searching for", question.Name, "with record type", dns.TypeToString[question.Qtype])

	result := handler.localCache.Get(dns.Name(question.Name), dns.Type(question.Qtype))

	if result != nil {
		fmt.Println(question.Name, "found in local cache with", len(result), "answers")
		return result, nil
	}

	msg := new(dns.Msg)
	msg.SetQuestion(question.Name, question.Qtype)

	var dnsRes *dns.Msg
	dnsRes, source, _ := handler.dnsPool.Do(*msg)

	// Catch when all of the upstream resolvers fail
	if dnsRes == nil {
		dnsRes = &dns.Msg{}
	}

	fmt.Println(question.Name, "found in", source.Address, ":", source.Port, "with", len(dnsRes.Answer), "answers")

	go func() {
		if len(dnsRes.Answer) > 0 {
			handler.localCache.Set(dns.Name(question.Name), dnsRes.Answer)
		}
	}()

	return dnsRes.Answer, nil

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
