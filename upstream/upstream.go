package upstream

import (
	"fmt"
	"github.com/miekg/dns"
	"net"
	"sync"
	"time"
)

type MessageResult struct {
	message *dns.Msg
	error   error
	server  *Server
}

type Query struct {
	message     dns.Msg
	resolveChan chan<- MessageResult
}

type Server struct {
	Name     string
	Address  string
	Port     string
	Priority uint
}

func DNSWorker(pool *Pool) {
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

func MakeUpstreamPool(size int, knownServers *[]Server) *Pool {
	// Check for well-known DNS resolvers to know which ones work on the current host
	// Common issue for CSE-Lab machines is blocking UDP to 1.1.1.1
	serverCheckChan := make(chan Server)

	// Spawn goroutines for all the known resolvers
	for _, server := range *knownServers {
		go func(server Server, returnChan chan<- Server) {
			dnsClient := new(dns.Client)
			dnsClient.Timeout = 500 * time.Millisecond

			// Setup the dns message for well-known "google.com"
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn("google.com"), dns.TypeA)

			fmt.Printf("Checking [%s]:%s...\n", server.Address, server.Port)

			// Make the dns request
			_, _, err := dnsClient.Exchange(m, net.JoinHostPort(server.Address, server.Port))
			if err == nil {
				// If we get a response we can assume the address is live
				returnChan <- server
			} else {
				// Server does not work, resolve as nil
				returnChan <- Server{}
			}

		}(server, serverCheckChan)
	}

	operationalServers := make([]Server, len(*knownServers))[:0]

	// Wait for all the well known checks and resolve them
	for range *knownServers {
		server := <-serverCheckChan
		if server.Address != "" {
			fmt.Printf("Resolved [%s]:%s\n", server.Address, server.Port)
			operationalServers = append(operationalServers, server)
		}
	}

	var wg sync.WaitGroup
	pool := Pool{knownServers, &operationalServers, make(chan Query), &wg}

	// Set up upstream dns clients
	wg.Add(size)
	for i := 0; i < size; i++ {
		go DNSWorker(&pool)
	}

	return &pool
}
