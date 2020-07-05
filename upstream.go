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
	server  *UpstreamServer
}

type DNSQuery struct {
	message     dns.Msg
	resolveChan chan<- MessageResult
}

type UpstreamServer struct {
	Address string
	Port    string
}

type UpstreamDNSPool struct {
	knownServers       *[]UpstreamServer
	operationalServers *[]UpstreamServer
	messagesToResolve  chan DNSQuery
	wg                 *sync.WaitGroup
}

func DNSWorker(pool *UpstreamDNSPool) {
	defer pool.wg.Done()
	dnsClient := new(dns.Client)
	dnsClient.Timeout = 300 * time.Millisecond

	for {
		query := <-pool.messagesToResolve
		var err error
		var dnsRes *dns.Msg

		for _, server := range *pool.operationalServers {
			dnsRes, _, err = dnsClient.Exchange(&query.message, net.JoinHostPort(server.Address, server.Port))
			if err == nil {
				// If we get a response we can return
				query.resolveChan <- MessageResult{dnsRes, nil, &server}
				break
			}
		}

		if err != nil {
			query.resolveChan <- MessageResult{nil, err, nil}
		}
	}
}

func MakeUpstreamPool(size int, knownServers *[]UpstreamServer) *UpstreamDNSPool {
	// Check for well-known DNS resolvers to know which ones work on the current host
	// Common issue for CSE-Lab machines is blocking UDP to 1.1.1.1
	serverCheckChan := make(chan UpstreamServer)

	// Spawn goroutines for all the known resolvers
	for _, server := range *knownServers {
		go func(server UpstreamServer, returnChan chan<- UpstreamServer) {
			dnsClient := new(dns.Client)
			dnsClient.Timeout = 500 * time.Millisecond

			// Setup the dns message for well-known "google.com"
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn("google.com"), dns.TypeA)

			fmt.Printf("Checking %s:%s...\n", server.Address, server.Port)

			// Make the dns request
			_, _, err := dnsClient.Exchange(m, net.JoinHostPort(server.Address, server.Port))
			if err == nil {
				// If we get a response we can assume the address is live
				returnChan <- server
			} else {
				// Server does not work, resolve as nil
				returnChan <- UpstreamServer{}
			}

		}(server, serverCheckChan)
	}

	operationalServers := []UpstreamServer{}

	// Wait for all the well known checks and resolve them
	for range *knownServers {
		server := <-serverCheckChan
		if server.Address != "" {
			fmt.Printf("Resolved %s:%s\n", server.Address, server.Port)
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

func (pool UpstreamDNSPool) Do(m dns.Msg) (*dns.Msg, *UpstreamServer, error) {
	resolver := make(chan MessageResult)
	pool.messagesToResolve <- DNSQuery{m, resolver}
	result := <-resolver
	return result.message, result.server, result.error
}
