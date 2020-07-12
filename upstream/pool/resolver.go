package pool

import (
	"sync"
)

type Resolver struct {
	resolver chan<- *MessageResult
	resolves []chan<- *MessageResult
	result   *MessageResult
	mutex    sync.Mutex
}

func (poolResolver *Resolver) Add(resolver chan<- *MessageResult) {
	poolResolver.mutex.Lock()
	if poolResolver.result != nil {
		go func(result *MessageResult) {
			resolver <- result
		}(poolResolver.result)
	} else {
		poolResolver.resolves = append(poolResolver.resolves, resolver)
	}
	poolResolver.mutex.Unlock()
}

func (poolResolver *Resolver) resolve(result *MessageResult) {
	poolResolver.mutex.Lock()
	poolResolver.result = result
	for _, resolve := range poolResolver.resolves {
		go func(resolve chan<- *MessageResult, result *MessageResult) {
			resolve <- result
		}(resolve, result)
	}
	poolResolver.mutex.Unlock()
}
