package pool

import (
	"github.com/miekg/dns"
	"sync"
	"time"
)

type Message struct {
	Name string
	Type uint16
}

type MessageResult struct {
	Message *dns.Msg
	Error   error
	Server  *Server
}

type Query struct {
	Message     dns.Msg
	ResolveChan chan<- *MessageResult
}

type Pool struct {
	knownServers      []Server
	serverOrder       []Server
	resolvers         *DomainListing
	messagesToResolve chan Query
	wg                *sync.WaitGroup
	dnsClientTimeout  time.Duration
}

type ServerSuccess struct {
	Succeeded bool
	Index     int
}

type ServerIter struct {
	Server *Server
	Index  int
}

func MakePool(knownServers *[]Server, serverOrder *[]Server, wg *sync.WaitGroup, timeout time.Duration) *Pool {
	return &Pool{
		*knownServers,
		*serverOrder,
		&DomainListing{
			map[string]*Domain{},
			sync.RWMutex{},
		},
		make(chan Query),
		wg,
		timeout,
	}
}

func (pool Pool) Do(message Message) (*dns.Msg, *Server, error) {
	resolveChan := make(chan *MessageResult)
	var resolver *Resolver
	var result *MessageResult

	pool.resolvers.mutex.RLock()
	domainResolvers := pool.resolvers.domains[message.Name]
	if domainResolvers != nil {
		resolver = domainResolvers.Get(message.Type)
	}
	pool.resolvers.mutex.RUnlock()

	if resolver != nil {
		resolver.Add(resolveChan)
		result = <-resolveChan
	} else {
		if domainResolvers == nil {
			domainResolvers = pool.resolvers.Add(message.Name)
		}

		resolver = domainResolvers.Add(message.Type, resolveChan)

		msg := new(dns.Msg)
		msg.Compress = true
		msg.RecursionDesired = true
		msg.AuthenticatedData = true

		msg.SetQuestion(message.Name, message.Type)

		pool.messagesToResolve <- Query{*msg, resolveChan}
		result = <-resolveChan

		empty := domainResolvers.Delete(message.Type)
		if empty {
			pool.resolvers.Delete(message.Name)
		}
	}

	return result.Message, result.Server, result.Error
}

func (pool *Pool) ServerIter() (serverChan chan ServerIter, successChan chan ServerSuccess) {
	serverChan = make(chan ServerIter)
	successChan = make(chan ServerSuccess)
	results := make([]bool, pool.NumUpstreams())

	var iter func(int)
	var outcomes func()
	finished := false

	outcomes = func() {
		outcome := <-successChan

		results[outcome.Index] = outcome.Succeeded
		if outcome.Succeeded {
			finished = true
		} else {
			outcomes()
		}
	}

	iter = func(index int) {
		time.AfterFunc(pool.dnsClientTimeout/2, func() {
			if finished || index >= pool.NumUpstreams() {
				serverChan <- ServerIter{nil, -1}
				return
			}

			serverChan <- ServerIter{&pool.serverOrder[index], index}
			go iter(index + 1)
		})
	}

	go outcomes()
	go func() {
		serverChan <- ServerIter{&pool.serverOrder[0], 0}
		iter(1)
	}()
	return
}

func (pool *Pool) GetClientTimeout() time.Duration {
	return pool.dnsClientTimeout
}

func (pool *Pool) NumUpstreams() int {
	return len(pool.serverOrder)
}
