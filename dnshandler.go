package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type DnsHandler struct {
	logFile    *os.File
	redisPool  *RedisPool
	dnsPool    *UpstreamDNSPool
	localCache *Cache
}

func MakeDNSHandler(logFile *os.File, redisPool *RedisPool, dnsPool *UpstreamDNSPool, localCache *Cache) *DnsHandler {
	return &DnsHandler{logFile, redisPool, dnsPool, localCache}
}

func sanitizeFQDN(urlString string) (string, error, string) {
	requestedUrl, err := url.Parse(urlString)

	if err != nil {
		fmt.Println("Unable to parse requested url")
		return "", errors.New("unable to parse requested url"), ""
	}

	// Try to make the hostname the hostname (doesn't always work)
	host := requestedUrl.Hostname()
	if host == "" {
		host = strings.Trim(strings.Split(requestedUrl.EscapedPath(), "/")[0], " ")
	}

	if host == "" {
		fmt.Println("Unable to parse requested url")
		return "", errors.New("unable to parse requested url"), ""
	}

	// Normalize the hostname and make it a fqdn
	// Keep the original hostname for logging
	host = strings.ToLower(host)
	fqdn := host

	// Make sure it's actually fully resolved to root
	if !strings.HasSuffix(host, ".") {
		fqdn = host + "."
	}

	return fqdn, nil, host
}

func (handler DnsHandler) Do(urlString string) string {
	// Parse requested string
	fqdn, err, host := sanitizeFQDN(urlString)
	if err != nil {
		return "unable to parse requested url"
	}

	fmt.Println("Searching for", fqdn)
	resolveWith := make(chan string)

	// Spawn a goroutine to handle async tasks
	go func(resolveWith chan string) {
		// Start the redis query
		var redisResponse <-chan RedisResponse
		if handler.redisPool != nil {
			redisResponse = handler.redisPool.Get(fqdn)
		}

		resolved := handler.localCache.Get(fqdn)
		source := "local" // Default to local source
		var ttl uint32

		if resolved == "" {

			// Setup the dns message pre-maturely while we wait for redis to resolve
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn(fqdn), dns.TypeA)
			var dnsRes *dns.Msg
			source = "redis"
			ttl = 300 // Set redis grabs to 5 min ttl in local cache

			// Resolve redis
			var response RedisResponse
			if redisResponse != nil {
				response = <-redisResponse
			}
			resolved = response.data

			// If we can't resolve via cache look in upstream
			if resolved == "" {
				fmt.Println(host, "not found in local, querying remote...")
				dnsRes, source, _ = handler.dnsPool.Do(*m)

				// Catch when all of the upstream resolvers fail
				if dnsRes == nil {
					dnsRes = &dns.Msg{}
				}

				// Process the response if we have it
				// else reject the request
				switch len(dnsRes.Answer) {
				case 0:
					// No entries found
					fmt.Println(host, "not found in remote")
					break
				case 1:
					// Only one entry was given by upstream
					fmt.Println("Found one entry for", fqdn)
					switch aRecord := dnsRes.Answer[0].(type) {
					case *dns.A:
						resolved = aRecord.A.String()
						ttl = aRecord.Hdr.Ttl
					}
					break
				default:
					// Multiple entries were given by upstream
					fmt.Println("Found multiple entries for", fqdn, "Pinging...")
					optimalPing := 1000.0 // 1 sec will throw an error for latency reasons

					// Create a goroutine wait group and map for pings
					// The map should be operated over in a thread-safe manner due to only having unique ips
					var wg sync.WaitGroup
					var firstARecord *dns.A
					pings := make(map[string]float64)

					// Iterate through each answer and ping the host to check for best live server
					// We should receive an A record if it didn't error because we requested one
					for _, a := range dnsRes.Answer {
						switch a.(type) {
						case *dns.A:
							if firstARecord == nil {
								firstARecord = a.(*dns.A)
							}

							// Start goroutines for each ping command
							wg.Add(1)
							go AsyncPing(&wg, &pings, a.(*dns.A).A.String())
						}
					}

					// Wait for all pings to resolve (max 1+- sec)
					if firstARecord != nil {
						ttl = firstARecord.Hdr.Ttl
					}
					wg.Wait()

					// Use the optimal ping
					for ip, ping := range pings {
						if optimalPing > ping {
							optimalPing = ping
							resolved = ip
						}
					}

					// log the optimal or use the best-hope if pinging failed/was slow
					if optimalPing < 1000 {
						fmt.Println("Finished all pings for", fqdn)
						fmt.Println("Winner is", resolved, "at", optimalPing)
					} else {
						fmt.Println("All pings failed for", fqdn)
						fmt.Println("Returning first entry as best-hope")

						// Use first A record as the best-hope backup
						if firstARecord != nil {
							ttl = firstARecord.Hdr.Ttl
							resolved = firstARecord.A.String()
						}
					}
					break
				}
			}
		}

		// If we got a non-local resolve, cache it until ttl
		if resolved != "" {
			// Resolve the request before we try to contact redis (redis is slower than acceptable)
			fmt.Println(fqdn, "found in", source, "as", resolved)
			resolveWith <- fmt.Sprintf("%s:%s:%s", source, host, resolved)

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
			}
		} else {
			fmt.Println("Unable to resolve", fqdn)
			resolveWith <- fmt.Sprintf("Unable to resolve %s", host)
			resolved = "Unable to resolve"
		}

		// Write to log file if possible
		if handler.logFile != nil {
			if _, err := handler.logFile.Write([]byte(fmt.Sprintf("%s,%s\n", host, resolved))); err != nil {
				fmt.Println("Error while writing log, closing file")
				_ = handler.logFile.Close()
			}
		}
	}(resolveWith)

	// Resolve the goroutine
	return <-resolveWith
}
