package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type MessageResult struct {
	message *dns.Msg
	error   error
	server  string
}

type DNSQuery struct {
	message     dns.Msg
	resolveChan chan<- MessageResult
}

type UpstreamDNSPool struct {
	knownServers       *[]string
	operationalServers *[]string
	messagesToResolve  chan DNSQuery
	wg                 *sync.WaitGroup
}

func DNSWorker(pool *UpstreamDNSPool) {
	defer pool.wg.Done()
	dnsClient := new(dns.Client)
	dnsClient.Timeout = 100 * time.Millisecond

	for {
		query := <-pool.messagesToResolve
		var err error
		var dnsRes *dns.Msg

		for _, server := range *pool.operationalServers {
			dnsRes, _, err = dnsClient.Exchange(&query.message, net.JoinHostPort(server, "53"))
			if err == nil {
				// If we get a response we can return
				query.resolveChan <- MessageResult{dnsRes, nil, server}
				break
			}
		}

		if err != nil {
			query.resolveChan <- MessageResult{nil, err, ""}
		}
	}
}

func createUpstreamPool(size int, knownServers *[]string) *UpstreamDNSPool {
	// Check for well-known DNS resolvers to know which ones work on the current host
	// Common issue for CSE-Lab machines is blocking UDP to 1.1.1.1
	serverCheckChan := make(chan string)

	// Spawn goroutines for all the known resolvers
	for _, server := range *knownServers {
		go func(server string, returnChan chan<- string) {
			dnsClient := new(dns.Client)
			dnsClient.Timeout = 100 * time.Millisecond

			// Setup the dns message for well-known "google.com"
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn("google.com"), dns.TypeA)

			fmt.Printf("Checking %s...\n", server)

			// Make the dns request
			_, _, err := dnsClient.Exchange(m, net.JoinHostPort(server, "53"))
			if err == nil {
				// If we get a response we can assume the server is live
				returnChan <- server
			} else {
				// Server does not work, resolve as empty string
				returnChan <- ""
			}

		}(server, serverCheckChan)
	}

	operationalServers := []string{}

	// Wait for all the well known checks and resolve them
	for range *knownServers {
		server := <-serverCheckChan
		if server != "" {
			fmt.Printf("Resolved %s\n", server)
			operationalServers = append(operationalServers, server)
		}
	}

	var wg sync.WaitGroup
	pool := UpstreamDNSPool{knownServers, &operationalServers, make(chan DNSQuery), &wg}

	// Set up upstream dns clients
	wg.Add(size)
	for i := 0; i < size; i++ {
		go DNSWorker(&pool)
	}

	return &pool
}

func (pool UpstreamDNSPool) Do(m dns.Msg) (*dns.Msg, string, error) {
	resolver := make(chan MessageResult)
	pool.messagesToResolve <- DNSQuery{m, resolver}
	result := <-resolver
	return result.message, result.server, result.error
}
