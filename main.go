/**
 * main.go -- Noah Kraft
 *
 * This file is the entire DNS server. It runs the http servers for POST and WS connections, maintains connection pools
 * for redis, external dns, and writing to the log file. From there it spawns and runs goroutines for each connection
 * that query redis for cache, external dns for authoritative dns, and the built-in ping command on unix systems.
 */

package main

import (
	"fmt"
	"github.com/mediocregopher/radix"
	"github.com/miekg/dns"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

func main() {
	const redisBasis = "baka-dns:urls"

	// Try to open the log file
	f, err := os.OpenFile("dns-server-log.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Unable to open log file for appending\nResolve this issue then restart if logging desired")
	} else {
		fmt.Println("Log file opened")
	}

	// Try to open redis pool
	pool, err := radix.NewPool("tcp", "127.0.0.1:64444", 10)
	if err != nil {
		fmt.Println("Redis is down! Directing all operations to remote")
		pool = nil
	} else {
		fmt.Println("Redis connected")
	}

	// Setup authoritative dns
	dnsConfig := dns.ClientConfig{Servers: []string{"1.1.1.1", "1.0.0.1"}, Port: "53"}
	dnsClient := new(dns.Client)

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
			source := "local"
			ttl := 300

			// Check for redis and then query the cache
			if pool != nil {
				err = pool.Do(radix.Cmd(&resolved, "GET", fmt.Sprintf("%s:%s", redisBasis, fqdn)))
			}

			// If we can't resolve via cache look in authoritative
			if err != nil || resolved == "" {
				fmt.Println(host, "not found in local, querying remote...")

				// Setup the dns message
				m := new(dns.Msg)
				m.SetQuestion(dns.Fqdn(fqdn), dns.TypeA)
				m.RecursionDesired = true

				// Make the dns request
				dnsRes, _, err := dnsClient.Exchange(m, net.JoinHostPort(dnsConfig.Servers[0], dnsConfig.Port))
				if err == nil && len(dnsRes.Answer) != 0 {
					source = dnsConfig.Servers[0]

					if len(dnsRes.Answer) > 1 {
						fmt.Println("Found multiple entries for", fqdn, "Pinging...")
						var backupResolved string
						optimalPing := 1000.0 // 1 sec will throw an error for latency reasons

						var wg sync.WaitGroup
						var pings sync.Map

						// Set up first ip as the best-hope backup
						switch aRecord := dnsRes.Answer[0].(type) {
						case *dns.A:
							aRecordIp := aRecord.A.String()

							if resolved == "" {
								ttl = int(aRecord.Hdr.Ttl)
								backupResolved = aRecordIp
							}
						}

						for _, a := range dnsRes.Answer {
							switch aRecord := a.(type) {
							case *dns.A:
								aRecordIp := aRecord.A.String()

								// Start goroutines for each ping command
								wg.Add(1)
								go func(wg *sync.WaitGroup, pings *sync.Map) {
									defer wg.Done()
									out, err := exec.Command("ping", "-c", "1", "-w", "1", fmt.Sprintf("%s", aRecordIp)).Output()
									if err == nil {
										// Process returned stdout
										lines := strings.Split(fmt.Sprintf("%s", out), "\n")
										if len(lines) > 4 {
											ping, err := strconv.ParseFloat(strings.Split(strings.Split(lines[1], "time=")[1], " ms")[0], 64)
											if err == nil {
												// Store ip -> ping map
												pings.Store(aRecordIp, ping)
											}
										}
									}
								}(&wg, &pings)
							}
						}

						// Wait for all pings to resolve (max 1+- sec)
						wg.Wait()

						// Use the optimal ping
						pings.Range(func(ip interface{}, pingInterface interface{}) bool {
							ping := pingInterface.(float64)
							if optimalPing > ping {
								optimalPing = ping
								resolved = ip.(string)
							}
							return true
						})

						// If no pings were successful use the first as the best-hope
						if resolved == "" {
							resolved = backupResolved
						}

						if optimalPing < 1000 {
							fmt.Println("Finished all pings for", fqdn)
							fmt.Println("Winner is", resolved, "at", optimalPing)
						} else {
							fmt.Println("All pings failed for", fqdn)
							fmt.Println("Returning first entry as best-hope")
						}
					} else {
						// Only one entry was given by the authoritative
						fmt.Println("Found one entry for", fqdn)
						switch aRecord := dnsRes.Answer[0].(type) {
						case *dns.A:
							resolved = aRecord.A.String()
							ttl = int(aRecord.Hdr.Ttl)
						}
					}
				} else {
					fmt.Println(host, "not found in remote")
				}
			}

			// If we got a non-local resolve, cache it until ttl
			if resolved != "" {
				if pool != nil && source != "local" {
					err = pool.Do(radix.FlatCmd(nil, "SETEX", fmt.Sprintf("%s:%s", redisBasis, fqdn), ttl, resolved))
					if err != nil {
						fmt.Println("Unable to cache")
					} else {
						fmt.Println(host, "cached")
					}
				}

				fmt.Println(fqdn, "found in", source, "as", resolved)
				resolveWith <- fmt.Sprintf("%s:%s:%s", source, host, resolved)
			} else {
				fmt.Println("Unable to resolve", fqdn)
				resolveWith <- fmt.Sprintf("Unable to resolve %s", host)
				resolved = "\"Unable to resolve\""
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
						conn.Close()
						break
					}

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

	// Make sure we close the file once the program exits
	if f != nil {
		if err := f.Close(); err != nil {
			fmt.Println("Closed logging file")
		}
	}
}
