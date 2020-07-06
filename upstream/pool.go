package upstream

import (
	"github.com/miekg/dns"
	"sync"
)

type Pool struct {
	knownServers       *[]Server
	operationalServers *[]Server
	messagesToResolve  chan Query
	wg                 *sync.WaitGroup
}

func (pool Pool) Do(m dns.Msg) (*dns.Msg, *Server, error) {
	resolver := make(chan MessageResult)
	pool.messagesToResolve <- Query{m, resolver}
	result := <-resolver
	return result.message, result.server, result.error
}

func (pool *Pool) NumUpstreams() int {
	return len(*pool.operationalServers)
}
