package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

func asyncPing(wg *sync.WaitGroup, pings *map[string]float64, ip string) {
	defer wg.Done()

	// Ping command requires `root` in order to run, thus we have to `exec ping`
	// Run ping with a timeout of 1 sec, any longer and we don't care and need to resolve
	out, err := exec.Command("ping", "-c", "1", "-w", "1", fmt.Sprintf("%s", ip)).Output()
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
	(*pings)[ip] = ping
}
