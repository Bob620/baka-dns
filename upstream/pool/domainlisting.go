package pool

import "sync"

type DomainListing struct {
	domains map[string]*Domain
	mutex   sync.RWMutex
}

func (domainListing *DomainListing) Get(name string) *Domain {
	return domainListing.domains[name]
}

func (domainListing *DomainListing) Add(name string) (domain *Domain) {
	domainListing.mutex.Lock()
	domain = &Domain{
		resolvers: map[uint16]*Resolver{},
		mutex:     sync.Mutex{},
	}
	domainListing.domains[name] = domain
	domainListing.mutex.Unlock()

	return
}

func (domainListing *DomainListing) Delete(name string) {
	domainListing.mutex.Lock()
	delete(domainListing.domains, name)
	domainListing.mutex.Unlock()
}
