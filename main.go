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
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mediocregopher/radix"
	"github.com/miekg/dns"
)

func main() {
	const redisBasis = "baka-dns:urls"

	// Try to open the log file
	f, err := os.OpenFile("dns-server-log.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Unable to open log file for appending\nResolve this issue then restart if logging desired")
	} else {
		fmt.Println("Log file opened")
		defer f.Close()
	}

	// Setup a custom redis client that timesout really quickly (default 1sec)
	quickTimeoutRedis := func(network, addr string) (radix.Conn, error) {
		return radix.Dial(network, addr,
			radix.DialTimeout(100*time.Millisecond),
		)
	}

	// Try to open redis pool
	// If we can't connect to redis we just can't cache, not a bad tradeoff
	pool, err := radix.NewPool("tcp", "127.0.0.1:64444", 10, radix.PoolConnFunc(quickTimeoutRedis))
	if err != nil {
		fmt.Println("Redis is down! Directing all operations to remote")
		pool = nil
	} else {
		fmt.Println("Redis connected")
	}

	// Set up upstream dns client
	dnsClient := new(dns.Client)
	knownServers := []string{"1.1.1.1", "1.0.0.1", "127.0.0.53"}
	var operationalServers []string

	// Check for well-known DNS resolvers to know which ones work on the current host
	// Common issue for CSE-Lab machines is blocking UDP to 1.1.1.1
	for i := range knownServers {
		server := knownServers[i]

		// Setup the dns message for well-known "google.com"
		m := new(dns.Msg)
		m.SetQuestion(dns.Fqdn("google.com"), dns.TypeA)

		// Make the dns request
		_, _, err := dnsClient.Exchange(m, net.JoinHostPort(server, "53"))
		if err == nil {
			// If we get a response we can assume the server is live
			operationalServers = append(operationalServers, server)
		}
	}

	fmt.Printf("Found %d operatonal authorative dns servers\n", len(operationalServers))

	if len(operationalServers) < 1 {
		fmt.Println("Unable to find upstream dns, unable to start server")
		return
	}

	// Create the dns config for live use
	dnsConfig := dns.ClientConfig{Servers: operationalServers, Port: "53"}
	dnsClient.Timeout = 100 * time.Millisecond

	dnsHandler := func(data string) string {
		// Parse requested string
		requestedUrl, err := url.Parse(data)

		if err != nil {
			fmt.Println("Unable to parse requested url")
			return "Unable to parse requested url"
		}

		// Try to make the hostname the hostname (doesn't always work)
		host := requestedUrl.Hostname()
		if host == "" {
			host = strings.Trim(strings.Split(requestedUrl.EscapedPath(), "/")[0], " ")
		}

		if host == "" {
			fmt.Println("Unable to parse requested url")
			return "Unable to parse requested url"
		}

		// Normalize the hostname and make it a fqdn
		// Keep the original hostname for logging
		host = strings.ToLower(host)
		fqdn := host

		if !strings.HasSuffix(host, ".") {
			fqdn = host + "."
		}

		fmt.Println("Searching for", fqdn)
		resolveWith := make(chan string)

		// Spawn a goroutine to handle async tasks
		go func(resolveWith chan string) {
			resolved := ""
			source := "local" // Default to local source
			ttl := 300        // Default to common 5min ttl unless overridden later

			var redisResponse chan string

			// Check for redis and then query the cache
			if pool != nil {
				redisResponse = make(chan string)

				go func(response chan<- string) {
					redisData := ""
					err = pool.Do(radix.Cmd(&redisData, "GET", fmt.Sprintf("%s:%s", redisBasis, fqdn)))
					response <- redisData

					if err != nil {
						pool.Close()
						// Redis is dead, abandon ship!
						pool = nil
					}
				}(redisResponse)
			}

			// Setup the dns message pre-maturely while we wait for redis to resolve
			m := new(dns.Msg)
			m.SetQuestion(dns.Fqdn(fqdn), dns.TypeA)

			if redisResponse != nil {
				// Wait for redis to resolve
				resolved = <-redisResponse
			}

			// If we can't resolve via cache look in upstream
			if resolved == "" {
				fmt.Println(host, "not found in local, querying remote...")
				var dnsRes *dns.Msg

				// Make the dns request to as many working servers as we need until we get a non-err response
				for i := range dnsConfig.Servers {
					source = dnsConfig.Servers[i]
					dnsRes, _, err = dnsClient.Exchange(m, net.JoinHostPort(source, dnsConfig.Port))

					if err == nil {
						break
					}
				}

				// Catch when all of the upstream resolvers fail
				if dnsRes == nil {
					dnsRes = &dns.Msg{}
				}

				// Process the response if we have it
				// else reject the request
				switch len(dnsRes.Answer) {
				case 0:
					// No entries found
					fmt.Println(host, "not found in remote")
					break
				case 1:
					// Only one entry was given by upstream
					fmt.Println("Found one entry for", fqdn)
					switch aRecord := dnsRes.Answer[0].(type) {
					case *dns.A:
						resolved = aRecord.A.String()
						ttl = int(aRecord.Hdr.Ttl)
					}
					break
				default:
					// Multiple entries were given by upstream
					fmt.Println("Found multiple entries for", fqdn, "Pinging...")
					optimalPing := 1000.0 // 1 sec will throw an error for latency reasons

					// Create a goroutine wait group and map for pings
					// The map should be operated over in a thread-safe manner due to only having unique ips
					var wg sync.WaitGroup
					var firstARecord *dns.A
					pings := make(map[string]float64)

					// Iterate through each answer and ping the host to check for best live server
					// We should receive an A record if it didn't error because we requested one
					for _, a := range dnsRes.Answer {
						switch a.(type) {
						case *dns.A:
							if firstARecord == nil {
								firstARecord = a.(*dns.A)
							}

							aRecordIp := a.(*dns.A).A.String()

							// Start goroutines for each ping command
							wg.Add(1)
							go func(wg *sync.WaitGroup, pings *map[string]float64) {
								defer wg.Done()

								// Ping command requires `root` in order to run, thus we have to `exec ping`
								// Run ping with a timeout of 1 sec, any longer and we don't care and need to resolve
								out, err := exec.Command("ping", "-c", "1", "-w", "1", fmt.Sprintf("%s", aRecordIp)).Output()
								if err != nil {
									return
								}

								// Process returned stdout
								lines := strings.Split(fmt.Sprintf("%s", out), "\n")
								if len(lines) <= 4 {
									return
								}

								// Parse stdout
								ping, err := strconv.ParseFloat(strings.Split(strings.Split(lines[1], "time=")[1], " ms")[0], 64)
								if err != nil {
									return
								}

								// Store ip -> ping map
								(*pings)[aRecordIp] = ping
							}(&wg, &pings)
						}
					}

					// Wait for all pings to resolve (max 1+- sec)
					wg.Wait()

					// Use the optimal ping
					for ip, ping := range pings {
						if optimalPing > ping {
							optimalPing = ping
							resolved = ip
						}
					}

					// log the optimal or use the best-hope if pinging failed/was slow
					if optimalPing < 1000 {
						fmt.Println("Finished all pings for", fqdn)
						fmt.Println("Winner is", resolved, "at", optimalPing)
					} else {
						fmt.Println("All pings failed for", fqdn)
						fmt.Println("Returning first entry as best-hope")

						// Use first A record as the best-hope backup
						if firstARecord != nil {
							ttl = int(firstARecord.Hdr.Ttl)
							resolved = firstARecord.A.String()
						}
					}
					break
				}
			}

			// If we got a non-local resolve, cache it until ttl
			if resolved != "" {
				// Resolve the request before we try to contact redis (redis is slower than acceptable)
				fmt.Println(fqdn, "found in", source, "as", resolved)
				resolveWith <- fmt.Sprintf("%s:%s:%s", source, host, resolved)

				if pool != nil && source != "local" {
					// Set the dns answer with the ttl as it's expiration
					err = pool.Do(radix.FlatCmd(nil, "SETEX", fmt.Sprintf("%s:%s", redisBasis, fqdn), ttl, resolved))
					if err != nil {
						fmt.Println("Unable to cache")
					} else {
						fmt.Println(host, "cached")
					}
				}
			} else {
				fmt.Println("Unable to resolve", fqdn)
				resolveWith <- fmt.Sprintf("Unable to resolve %s", host)
				resolved = "Unable to resolve"
			}

			// Write to log file if possible
			if f != nil {
				if _, err := f.Write([]byte(fmt.Sprintf("%s,%s\n", host, resolved))); err != nil {
					fmt.Println("Error while writing log, closing file")
					f.Close()
				}
			}
		}(resolveWith)

		// Resolve the goroutine
		return <-resolveWith
	}

	// Handle POST requests to the server
	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		// Read in and process the request body
		data := make([]byte, request.ContentLength)
		if _, err := io.ReadFull(request.Body, data); err != nil {
			fmt.Println("error:", err)
		}

		io.WriteString(writer, dnsHandler(fmt.Sprintf("%s", data)))
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
						conn.Close()
						break
					}

					// If we have a request
					if length > 0 {
						conn.Write([]byte(dnsHandler(string(data[:length]))))
					}
				}
			}()
		}
	}()

	fmt.Println("Listening on :9889 (WS)\nListening on :9888 (POST)")

	// Start and hold on normal requests made to :9889
	http.ListenAndServe(":9888", nil)
}
