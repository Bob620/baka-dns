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
	"github.com/Bob620/baka-dns/upstream"
	"github.com/Bob620/baka-dns/upstream/pool"
	"github.com/miekg/dns"
)

const port = ":53"

func main() {
	var dnsPool *pool.Pool
	var redisPool *RedisPool
	var localCache *cache.Cache

	redisPool, err := MakeRedisPool("127.0.0.1:64444", "baka-dns:urls")
	if err != nil {
		fmt.Println("Unable to connect to redis")
	} else {
		fmt.Println("Connected to redis")
	}

	dnsPool = upstream.MakeUpstreamPool(10, &[]pool.Server{
		{"cloudflared", "192.168.2.1", "5353", 0},
		{"1.1.1.1", "1.1.1.1", "53", 1},
		{"1.0.0.1", "1.0.0.1", "53", 2},
	})

	fmt.Printf("Found %d operatonal authorative dns servers\n", dnsPool.NumUpstreams())

	if dnsPool.NumUpstreams() < 1 {
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

		if msg.Opcode != dns.OpcodeQuery {
			msg.Rcode = dns.RcodeNotImplemented
			_ = writer.WriteMsg(msg)

			return
		}

		res, err := dnsHandler.Do(&msg.Question[0])
		msg.RecursionAvailable = true

		if res != nil {
			msg.Authoritative = res.Authoritative
			msg.AuthenticatedData = res.AuthenticatedData
			msg.Ns = res.Ns
			msg.Extra = res.Extra
			msg.Rcode = dns.RcodeSuccess
		}

		if err == nil {
			msg.Response = true
			msg.Answer = res.Answer
		} else {
			msg.Response = false
			msg.Rcode = dns.RcodeNameError
		}

		_ = writer.WriteMsg(msg)
	})

	fmt.Println("Listening on", port)
	err = server.ListenAndServe()

	if err != nil {
		fmt.Println(err)
	}
}
