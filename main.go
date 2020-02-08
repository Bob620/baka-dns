/**
 * main.go -- Noah Kraft
 *
 * This file is the entire DNS server. It runs the http servers for POST and WS connections, maintains connection pools
 * for redis, external dns, and writing to the log file. From there it spawns and runs goroutines for each connection
 * that query redis for cache, external dns for upstream dns resolution, and the built-in ping command on unix systems.
 */

package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
)

func main() {
	var dnsPool *UpstreamDNSPool
	var redisPool *RedisPool

	// Try to open the log file
	f, err := os.OpenFile("dns-server-log.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Unable to open log file for appending\nResolve this issue then restart if logging desired")
	} else {
		fmt.Println("Log file opened")
		defer f.Close()
	}

	redisPool, err = makeRedisPool("127.0.0.1:64444", "baka-dns:urls")
	if err != nil {
		fmt.Println("Unable to connect to redis")
	} else {
		fmt.Println("Connected to redis")
	}

	// Use cloudflare if possible, fall back to CSE-Lab local dns resolver
	dnsPool = createUpstreamPool(10, &[]string{"1.1.1.1", "1.0.0.1", "127.0.0.53"})

	fmt.Printf("Found %d operatonal authorative dns servers\n", len(*dnsPool.operationalServers))

	if len(*dnsPool.operationalServers) < 1 {
		fmt.Println("Unable to find upstream dns, unable to start server")
		return
	}

	// Create dns handling function
	dnsHandler := makeDNSHandler(f, redisPool, dnsPool)

	// Handle POST requests to the server
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		// Read in and process the request body
		data := make([]byte, request.ContentLength)
		if _, err := io.ReadFull(request.Body, data); err != nil {
			fmt.Println("error:", err)
		}

		_, _ = io.WriteString(writer, dnsHandler.Do(fmt.Sprintf("%s", data)))
	})

	// Spawn goroutine to handle websockets
	go func() {
		ln, _ := net.Listen("tcp", ":9889")

		for {
			conn, _ := ln.Accept()

			// Spawn goroutine for each connection made to the websocket
			go func() {
				for {
					data := make([]byte, 1024)

					length, err := conn.Read(data)
					if err != nil {
						// Kill a dead connection or restart an existing one from an EOF error loop
						// It should work for the provided python3 client
						_ = conn.Close()
						break
					}

					// If we have a request
					if length > 0 {
						_, _ = conn.Write([]byte(dnsHandler.Do(string(data[:length]))))
					}
				}
			}()
		}
	}()

	fmt.Println("Listening on :9889 (WS)\nListening on :9888 (POST)")

	// Start and hold on normal requests made to :9889
	_ = http.ListenAndServe(":9888", nil)
}
