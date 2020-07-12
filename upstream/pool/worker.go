package pool

import (
	"github.com/miekg/dns"
	"net"
)

func Worker(pool *Pool) {
	defer pool.wg.Done()
	dnsClient := new(dns.Client)
	dnsClient.Timeout = pool.GetClientTimeout()

	for {
		query := <-pool.messagesToResolve
		var err error
		var dnsRes *dns.Msg

		serverChan, successChan := pool.ServerIter()
		for {
			serverIter := <-serverChan

			if serverIter.Index == -1 {
				break
			}

			dnsRes, _, err = dnsClient.Exchange(&query.Message, net.JoinHostPort(serverIter.Server.Address, serverIter.Server.Port))
			if err == nil {
				// If we get a response we can return
				query.ResolveChan <- &MessageResult{Message: dnsRes, Server: serverIter.Server}
				successChan <- ServerSuccess{true, serverIter.Index}
				break
			} else {
				successChan <- ServerSuccess{false, serverIter.Index}
			}
		}

		if err != nil {
			query.ResolveChan <- &MessageResult{Error: err}
		}
	}
}
