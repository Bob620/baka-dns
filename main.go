package main

import (
	"fmt"
	"github.com/mediocregopher/radix"
	"github.com/miekg/dns"
	"github.com/tatsushid/go-fastping"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func main() {

	pool, err := radix.NewPool("tcp", "127.0.0.1:64444", 10)
	if err != nil {
		fmt.Println("Redis is down!")
	}

	dnsConfig := dns.ClientConfig{Servers: []string{"1.1.1.1", "1.0.0.1"}, Port: "53"}
	dnsClient := new(dns.Client)

	h1 := func(res http.ResponseWriter, req *http.Request) {
		data := make([]byte, req.ContentLength)
		if _, err := io.ReadFull(req.Body, data); err != nil {
			fmt.Println("error:", err)
		}

		resolved := ""
		source := "local"
		pinging := false

		requestedUrl, err := url.Parse(fmt.Sprintf("%s", data))

		if err == nil {
			host := requestedUrl.Hostname()
			if host == "" {
				host = strings.Trim(strings.Split(requestedUrl.EscapedPath(), "/")[0], " ")
			}

			if host != "" {
				host = strings.ToLower(host)

				fqdn := host

				if !strings.HasSuffix(host, ".") {
					fqdn = host + "."
				}

				fmt.Println("Searching for", fqdn)

				err = pool.Do(radix.Cmd(&resolved, "GET", fmt.Sprintf("baka-dns:urls:%s", fqdn)))
				if err != nil || resolved == "" {
					fmt.Println(host, "not found in local, querying remote...")

					m := new(dns.Msg)
					m.SetQuestion(dns.Fqdn(fqdn), dns.TypeA)
					m.RecursionDesired = true

					dnsRes, _, err := dnsClient.Exchange(m, net.JoinHostPort(dnsConfig.Servers[0], dnsConfig.Port))
					if err == nil && len(dnsRes.Answer) != 0 {
						source = dnsConfig.Servers[0]

						if len(dnsRes.Answer) > 1 {
							fmt.Println("Found multiple entries for", fqdn, "Pinging...")
							pinging = true
							Ttl := 300
							var backupResolved string

							p := fastping.NewPinger()

							p.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
								p.Stop()

								if pinging {
									pinging = false

									io.WriteString(res, fmt.Sprintf("%s:%s:%s", source, host, resolved))
									fmt.Println("finished all pings for", fqdn)
									fmt.Println("Winner is", addr.IP.String(), "at", rtt.String())

									err = pool.Do(radix.FlatCmd(nil, "SETEX", fmt.Sprintf("baka-dns:urls:%s", fqdn), Ttl, resolved))
									if err != nil {
										fmt.Println("Unable to cache")
									} else {
										fmt.Println(host, "cached")
									}
								}
							}

							for _, a := range dnsRes.Answer {
								switch aRecord := a.(type) {
								case *dns.A:
									Ttl = int(aRecord.Hdr.Ttl)
									if backupResolved == "" {
										backupResolved = aRecord.A.String()
									}

									err := p.AddIP(aRecord.A.String())
									if err != nil {
										fmt.Println(err)
									}
								}
							}

							err = p.Run()
							if err != nil {
								fmt.Println("Unable to ping for optimal route, please run as root or allow 'unprivileged' ping via UDP.")
								io.WriteString(res, fmt.Sprintf("%s:%s:%s", source, host, backupResolved))
								fmt.Println(fqdn, "found in", source, "as", backupResolved)

								err = pool.Do(radix.FlatCmd(nil, "SETEX", fmt.Sprintf("baka-dns:urls:%s", fqdn), Ttl, backupResolved))
								if err != nil {
									fmt.Println("Unable to cache")
								} else {
									fmt.Println(host, "cached")
								}
							}
						} else {
							fmt.Println("Found one entry for", fqdn)
							switch aRecord := dnsRes.Answer[0].(type) {
							case *dns.A:
								resolved = aRecord.A.String()

								err = pool.Do(radix.FlatCmd(nil, "SETEX", fmt.Sprintf("baka-dns:urls:%s", fqdn), int(aRecord.Hdr.Ttl), resolved))
								if err != nil {
									fmt.Println("Unable to cache")
								} else {
									fmt.Println(host, "cached")
								}
							}
						}
					} else {
						fmt.Println(host, "not found in remote")
					}
				}

				if !pinging {
					if resolved != "" {
						io.WriteString(res, fmt.Sprintf("%s:%s:%s", source, host, resolved))
						fmt.Println(fqdn, "found in", source, "as", resolved)
					} else {
						io.WriteString(res, fmt.Sprintf("Unable to resolve %s", host))
						fmt.Println("Unable to resolve", fqdn)
					}
				}

				return
			}
		}

		io.WriteString(res, fmt.Sprintf("Unable to parse requested url"))
		fmt.Println("Unable to parse requested url")
	}

	http.HandleFunc("/", h1)

	fmt.Println(http.ListenAndServe(":9889", nil))
}
