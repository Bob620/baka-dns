/**
 * main.go -- Noah Kraft
 *
 * This file is the entire DNS server. It runs the http servers for POST and WS connections, maintains connection pools
 * for redis, external dns, and writing to the log file. From there it spawns and runs goroutines for each connection
 * that query redis for cache, external dns for upstream dns resolution, and the built-in ping command on unix systems.
 */

package main

import (
	"fmt"
	"github.com/Bob620/baka-dns/cache"
	"github.com/miekg/dns"
)

const port = ":53"

func main() {
	var dnsPool *UpstreamDNSPool
	var redisPool *RedisPool
	var localCache *cache.Cache

	redisPool, err := MakeRedisPool("127.0.0.1:64444", "baka-dns:urls")
	if err != nil {
		fmt.Println("Unable to connect to redis")
	} else {
		fmt.Println("Connected to redis")
	}

	// Use cloudflare if possible, fall back to CSE-Lab local dns resolver
	dnsPool = MakeUpstreamPool(10, &[]UpstreamServer{{"192.168.2.1", "5353"}}) //, {"1.1.1.1", "53"}, {"1.0.0.1", "53"}, {"127.0.0.1", "53"}})

	fmt.Printf("Found %d operatonal authorative dns servers\n", len(*dnsPool.operationalServers))

	if len(*dnsPool.operationalServers) < 1 {
		fmt.Println("Unable to find upstream dns, unable to start server")
		return
	}

	localCache = cache.MakeCache(100)

	// Create dns handling function
	dnsHandler := MakeDNSHandler(redisPool, dnsPool, localCache)

	server := dns.Server{
		Addr: port,
		Net:  "udp",
	}

	server.Handler = dns.HandlerFunc(func(writer dns.ResponseWriter, msg *dns.Msg) {
		defer writer.Close()

		res, err := dnsHandler.Do(&msg.Question[0])

		msg.RecursionAvailable = true

		if err == nil {
			msg.Answer = res
			msg.Response = true
			_ = writer.WriteMsg(msg)
		} else {
			msg.Response = false
			_ = writer.WriteMsg(msg)
		}
	})

	fmt.Println("Listening on", port)
	err = server.ListenAndServe()

	if err != nil {
		fmt.Println(err)
	}
}
