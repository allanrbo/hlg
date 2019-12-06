package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

import (
	_ "net/http/pprof"
)

func main() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()

	ipv4, port, reqBytes, seconds, maxp99d99ms, maxp99d999ms, maxp100ms, rps, timeout, maxConcurrent := processCmdLine()

	req, err := newHttpReq(reqBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}

	// Disable garbage collection for less chance of random variation, and trigger it manually going forward.
	//debug.SetGCPercent(-1)

	if rps != 0 {
		fmt.Printf("Running with %v requests/sec\n", rps)
		b := NewBenchmark(req, ipv4, port, seconds, rps, timeout, maxConcurrent, true)
		_, err = b.Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return
		}
	} else {
		fmt.Printf("Starting...\n")
		ioutil.WriteFile("hillclimb.csv", []byte("rps,errors,p99d99\n"), 0644)
		rps = 1000
		a := 0.5
		for {
			b := NewBenchmark(req, ipv4, port, seconds, rps, timeout, maxConcurrent, false)
			r, err := b.Start()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				return
			}

			p99d99ms := float64(r.p99d99) / float64(time.Millisecond)
			p99d999ms := float64(r.p99d999) / float64(time.Millisecond)
			p100ms := float64(r.max) / float64(time.Millisecond)
			fmt.Printf("rps: %6d, errors: %6d, p99d99ms: %9.2f, p99d999ms: %9.2f, p100ms: %9.2f\n", rps, r.errors, p99d99ms, p99d999ms, p100ms)

			// Write progress to a file.
			f, err := os.OpenFile("hillclimb.csv", os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				return
			}
			_, err = fmt.Fprintf(f, "%d,%d,%f\n", rps, r.errors, p99d99ms)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				return
			}
			err = f.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				return
			}

			if r.errors == 0 && r.p99d99 <= maxp99d99ms && r.p99d999 <= maxp99d999ms && r.max <= maxp100ms {
				rps = int(float64(rps) + float64(rps)*a)
			} else {
				rps = int(float64(rps) - float64(rps)*a)
				a = a * 0.9
			}

			if r.errors > 0 {
				time.Sleep(10 * time.Second)
			}
		}
	}
}

func processCmdLine() (ipv4 net.IP, port int, reqBytes []byte, seconds int, maxp99d99ms time.Duration, maxp99d999ms time.Duration, maxp100ms time.Duration, rps int, timeout time.Duration, maxConcurrent int) {
	hostArg := flag.String("host", "127.0.0.1", "Target host and optionally port. Example: 127.0.0.1:8080")
	requestFileArg := flag.String("requestfile", "", "Path to a file containing a full HTTP request in raw form, that will be used for the benchmark.")
	maxp99d99msArg := flag.Int("maxp99d99ms", 100, "Vary rps until the 99.99th percentile reaches this number of milliseconds.")
	maxp99d999msArg := flag.Int("maxp99d999ms", 200, "Vary rps until the 99.999th percentile reaches this number of milliseconds.")
	maxp100msArg := flag.Int("maxp100ms", 500, "Vary rps until the 100th percentile reaches this number of milliseconds.")
	rpsArg := flag.Int("rps", 0, "Run at a single constant rate of requests per second instead of varying the rps.")
	secondsArg := flag.Int("seconds", 60, "Duration of each test in seconds.")
	timeoutArg := flag.Int("timeoutms", 8000, "Max time in miliseconds to wait for each request to finish before marking it as error and recording the timeout as the time it took.")
	maxConcurrentArg := flag.Int("maxconcurrent", 45000, "Max number of concurrent requests to allow. If this number of concurrent requests is reached and a new request is supposed to run, the new request will just immediately be marked as error.")
	flag.Parse()

	// Default to port 80 if no port was given.
	port = 80
	host := *hostArg
	if strings.Contains(host, ":") {
		h := strings.Split(host, ":")
		if len(h) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid host and port: %v\n", host)
			os.Exit(1)
		}

		host = h[0]
		var err error
		port, err = strconv.Atoi(h[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid port: %v\n", host)
			os.Exit(1)
		}
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to look up IPv4 address for %v: %v\n", host, err)
		os.Exit(1)
	}
	ipv4 = ips[0].To4()

	// If file arg given, then load req from file. Else use default req.
	reqBytes = defaultReqBytes
	if *requestFileArg != "" {
		var err error
		reqBytes, err = ioutil.ReadFile(*requestFileArg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	maxp99d99ms = time.Duration(*maxp99d99msArg) * time.Millisecond

	maxp99d999ms = time.Duration(*maxp99d999msArg) * time.Millisecond

	maxp100ms = time.Duration(*maxp100msArg) * time.Millisecond

	rps = *rpsArg

	seconds = *secondsArg

	timeout = time.Duration(*timeoutArg) * time.Millisecond

	maxConcurrent = *maxConcurrentArg

	return
}
