package pool

import (
	"sync"
)

type Domain struct {
	resolvers map[uint16]*Resolver
	mutex     sync.Mutex
}

func (domain *Domain) Get(typeId uint16) *Resolver {
	return domain.resolvers[typeId]
}

func (domain *Domain) Add(typeId uint16, resolver chan<- *MessageResult) (resolve *Resolver) {
	domain.mutex.Lock()
	subResolver := make(chan *MessageResult)

	resolve = &Resolver{
		resolver: subResolver,
		resolves: make([]chan<- *MessageResult, 2)[:0],
		mutex:    sync.Mutex{},
	}
	resolve.Add(resolver)
	domain.resolvers[typeId] = resolve

	go func(poolResolver *Resolver, subResolver <-chan *MessageResult) {
		result := <-subResolver
		resolve.resolve(result)
	}(resolve, subResolver)
	domain.mutex.Unlock()

	return
}

func (domain *Domain) Delete(typeId uint16) bool {
	domain.mutex.Lock()
	delete(domain.resolvers, typeId)
	domain.mutex.Unlock()

	return len(domain.resolvers) == 0
}
