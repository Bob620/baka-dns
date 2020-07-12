package upstream

import (
	"fmt"
	"github.com/Bob620/baka-dns/upstream/pool"
	"github.com/miekg/dns"
	"net"
	"sort"
	"sync"
	"time"
)

func MakeUpstreamPool(size int, knownServers *[]pool.Server) *pool.Pool {
	// Check for well-known DNS resolvers to know which ones work on the current host
	// Common issue for CSE-Lab machines is blocking UDP to 1.1.1.1
	serverCheckChan := make(chan pool.Server)

	// Spawn goroutines for all the known resolvers
	for _, server := range *knownServers {
		go func(server pool.Server, returnChan chan<- pool.Server) {
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
				returnChan <- pool.Server{}
			}

		}(server, serverCheckChan)
	}

	serverOrder := make([]pool.Server, len(*knownServers))[:0]

	// Wait for all the well known checks and resolve them
	for range *knownServers {
		server := <-serverCheckChan
		if server.Address != "" {
			fmt.Printf("Resolved [%s]:%s with priority %d\n", server.Address, server.Port, server.Priority)
			serverOrder = append(serverOrder, server)
		}
	}

	sort.Sort(pool.ByPriority(serverOrder))

	var wg sync.WaitGroup
	dnsPool := pool.MakePool(knownServers, &serverOrder, &wg, 500*time.Millisecond)

	// Set up upstream dns clients
	wg.Add(size)
	for i := 0; i < size; i++ {
		go pool.Worker(dnsPool)
	}

	return dnsPool
}
