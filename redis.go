package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/mediocregopher/radix"
)

type RedisResponse struct {
	data string
	err  error
}

type RedisPool struct {
	pool  *radix.Pool
	basis string
}

// Setup a custom redis client that timesout really quickly (default 1sec)
func quickTimeoutRedis(network, addr string) (radix.Conn, error) {
	return radix.Dial(network, addr,
		radix.DialTimeout(100*time.Millisecond),
	)
}

func makeRedisPool(addr, basis string) (*RedisPool, error) {
	// Try to open redis redisPool
	redisPool, err := radix.NewPool("tcp", addr, 10, radix.PoolConnFunc(quickTimeoutRedis))

	if err != nil {
		return nil, err
	}

	return &RedisPool{redisPool, basis}, err
}

func (pool RedisPool) FlatCmd(command, key string, arguments []string) <-chan RedisResponse {
	redisResponse := make(chan RedisResponse)

	// Check for redis and then query the cache
	if pool.pool != nil {
		// query redis asynchronously and close the redisPool on the change redis can't be properly contacted
		go func(response chan<- RedisResponse, command, key string, arguments []string) {
			redisData := ""
			err := pool.pool.Do(radix.FlatCmd(&redisData, command, key, arguments))
			response <- RedisResponse{redisData, err}

			if err != nil {
				_ = pool.pool.Close()
				// Redis is dead, abandon ship!
				pool.pool = nil
			}
		}(redisResponse, command, key, arguments)
	} else {
		redisResponse <- RedisResponse{"", errors.New("redis is down, unable to query")}
	}

	return redisResponse
}

func (pool RedisPool) Get(key string) <-chan RedisResponse {
	return pool.FlatCmd("GET", fmt.Sprintf("%s:%s", pool.basis, key), []string{})
}

func (pool RedisPool) Set(key, value string) <-chan RedisResponse {
	return pool.FlatCmd("SET", fmt.Sprintf("%s:%s", pool.basis, key), []string{value})
}

func (pool RedisPool) SetEx(key, value string, ttl int) <-chan RedisResponse {
	return pool.FlatCmd("SETEX", fmt.Sprintf("%s:%s", pool.basis, key), []string{fmt.Sprintf("%d", ttl), value})
}
