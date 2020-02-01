# Baka-DNS
Noah Kraft,Zhang CSCI4211,01/02/20

### General
Baka-DNS is a simple DNS written in Go. This was made for CSCI4211 but I may expand it in the future (and remove ws).

DNS queries can be made via a POST to :9888 or through websockets on :9889.

### Compilation
Go may not be in the path in some systems (CSE), make sure you add it:

`module add soft/go`

To compile and run use:

`go run main.go`

To just compile use:

`go build main.go`

Run pre-compiled:

`./main`

`Ctrl + c` to kill the server.

If you want to also have a local cache, run `./run.sh` in order to start the Redis server.
This is not required but it will have to query the remote dns for each query to it.

If redis-server is already present or you have your own install, run it with:

`./redis-server redis.conf`

### Description
Upon a connection with text, (websocket needs a newline) it will spawn a goroutine that parses the string into a valid
hostname. This hostname will be searched for in the redis cache, if found it will return the cached ip. If not in redis
it will send a DNS request to (by default) 1.1.1.1 for authoritative resolution. If no ip is found it returns an error, if
one ip is found it will return that ip, if multiple ip are found it spawns one goroutine for each and pings once via
spawning the ping function in another goroutine. If all pings fail(or take > 1 sec) it will return the first ip as
best-hope for the user to resolve, otherwise it will return the optimal ping-base ip. The resolved ip will then be cached
in redis with a timeout set to the ttl as given by the authoritative resolution.

The exact same DNS function is used for both POST and WS connections.

Detailed output is written to console during operation and each dns query response is logged into the `dns-server-log.csv`.
