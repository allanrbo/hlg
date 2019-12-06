# hlg: HTTP Load Generator

Hlg is a benchmarking tool that will find a sustainable throughput rate at a given latency requirement.

As input, it takes the maximum latency you are willing to accept. As output it gives you the rps (requests per second rate) able to be sustained while staying below the maximum latency requirement input.

It works by running a hill climbing algorithm with a series of benchmarks with varying rps. It eventually converges on a sustainable rps.

Example:
```
# ./hlg -host 192.168.1.10:80 -maxp99d99ms 100 -maxp99d999ms 150 -maxp100ms 500

Starting...
rps:   1000, errors:      0, p99d99ms:     18.10, p99d999ms:     18.10, p100ms:     18.26
rps:   1500, errors:      0, p99d99ms:     36.47, p99d999ms:     36.47, p100ms:     38.17
rps:   2250, errors:      0, p99d99ms:      4.12, p99d999ms:      4.16, p100ms:      7.16
rps:   3375, errors:      0, p99d99ms:     14.91, p99d999ms:     15.49, p100ms:     15.81
rps:   5062, errors:      0, p99d99ms:      2.87, p99d999ms:      3.25, p100ms:      3.34
rps:   7593, errors:      0, p99d99ms:     45.79, p99d999ms:     46.62, p100ms:     47.97
rps:  11389, errors:      0, p99d99ms:      6.35, p99d999ms:      6.87, p100ms:      6.93
rps:  17083, errors:      0, p99d99ms:     23.83, p99d999ms:     24.79, p100ms:     25.99
rps:  25624, errors:      0, p99d99ms:      8.93, p99d999ms:     15.53, p100ms:     15.75
rps:  38436, errors:      0, p99d99ms:     14.88, p99d999ms:     40.39, p100ms:     40.86
rps:  57654, errors:      0, p99d99ms:   3034.76, p99d999ms:   3040.88, p100ms:   3043.71
rps:  28827, errors:      0, p99d99ms:      9.42, p99d999ms:     11.19, p100ms:     11.41
rps:  28827, errors:      0, p99d99ms:      9.42, p99d999ms:     11.19, p100ms:     11.41
rps:  41799, errors:      0, p99d99ms:     16.12, p99d999ms:     20.66, p100ms:     21.10
rps:  60608, errors:      0, p99d99ms:    203.00, p99d999ms:   1030.29, p100ms:   1031.33
rps:  33334, errors:      0, p99d99ms:     17.96, p99d999ms:     23.01, p100ms:     23.32
rps:  46834, errors:      0, p99d99ms:     13.84, p99d999ms:     15.08, p100ms:     15.49
rps:  65801, errors:      0, p99d99ms:   1048.79, p99d999ms:   1057.68, p100ms:   1059.69
rps:  39151, errors:      0, p99d99ms:     22.05, p99d999ms:     25.94, p100ms:     27.22
rps:  53421, errors:      0, p99d99ms:     19.60, p99d999ms:     23.38, p100ms:     24.75
rps:  72892, errors:      0, p99d99ms:     45.72, p99d999ms:     51.27, p100ms:     52.47
rps:  99461, errors:      0, p99d99ms:     66.86, p99d999ms:     72.16, p100ms:     75.57
rps: 135714, errors:      0, p99d99ms:     70.62, p99d999ms:     79.32, p100ms:   1043.96
rps:  86246, errors:      0, p99d99ms:     71.25, p99d999ms:     76.55, p100ms:     82.51
rps: 114539, errors:      0, p99d99ms:   1237.13, p99d999ms:   3053.40, p100ms:   3061.11
rps:  76964, errors:      0, p99d99ms:     47.47, p99d999ms:     50.89, p100ms:     54.35
rps:  99687, errors:      0, p99d99ms:     74.21, p99d999ms:     78.83, p100ms:   1033.28
rps:  70254, errors:      0, p99d99ms:     67.40, p99d999ms:     72.69, p100ms:     84.24
rps:  88921, errors:      0, p99d99ms:     50.68, p99d999ms:     55.48, p100ms:     57.22
rps: 112549, errors:      0, p99d99ms:   1033.83, p99d999ms:   1226.11, p100ms:   1245.79
rps:  82642, errors:      0, p99d99ms:     61.51, p99d999ms:     64.59, p100ms:   1015.47
rps:  62878, errors:      0, p99d99ms:     35.11, p99d999ms:     39.41, p100ms:     46.24
rps:  76411, errors:      0, p99d99ms:     31.92, p99d999ms:     36.49, p100ms:     40.25
rps:  92857, errors:      0, p99d99ms:     43.35, p99d999ms:     46.03, p100ms:     50.48
rps: 112842, errors:      0, p99d99ms:     49.95, p99d999ms:     53.18, p100ms:   1035.07
rps:  88554, errors:      0, p99d99ms:     50.34, p99d999ms:     53.25, p100ms:     56.75
rps: 105707, errors:      0, p99d99ms:   1032.84, p99d999ms:   1041.10, p100ms:   1043.71
rps:  85230, errors:      0, p99d99ms:     48.82, p99d999ms:     52.36, p100ms:     53.98
rps: 100088, errors:      0, p99d99ms:     53.06, p99d999ms:     57.29, p100ms:     59.79

```

Implementation features:
 * For each benchmark, hlg creates an execution plan up front of when each requests is to be run. This helps avoid the coordinated omission problem as described by [Gil Tene](https://www.youtube.com/watch?v=lJ8ydIuPFeU).
 * Uses Linux's epoll API to run requests concurrently asynchronously.
 * Shards the requests in the execution plan between OS threads to distribute the load amongst all CPU cores.

Command line flags:
```
  -host string
        Target host and optionally port. Example: 127.0.0.1:8080 (default "127.0.0.1")
  -maxconcurrent int
        Max number of concurrent requests to allow. If this number of concurrent requests is reached and a new request is supposed to run, the new request will just immediately be marked as error. (default 45000)
  -maxp100ms int
        Vary rps until the 100th percentile reaches this number of milliseconds. (default 500)
  -maxp99d999ms int
        Vary rps until the 99.999th percentile reaches this number of milliseconds. (default 200)
  -maxp99d99ms int
        Vary rps until the 99.99th percentile reaches this number of milliseconds. (default 100)
  -requestfile string
        Path to a file containing a full HTTP request in raw form, that will be used for the benchmark.
  -rps int
        Run at a single constant rate of requests per second instead of varying the rps.
  -seconds int
        Duration of each test in seconds. (default 60)
  -timeoutms int
        Max time in miliseconds to wait for each request to finish before marking it as error and recording the timeout as the time it took. (default 8000)
```

FAQ
---

**Q**: I get the error `benchmark tool is being hindered by OS limit on number of open files`. What can I do?

**A**: add the following to your `/etc/security/limits.conf`
```
*         hard    nofile      500000
*         soft    nofile      500000
root      hard    nofile      500000
root      soft    nofile      500000
```

Differences and similarities to Wrk2
---

[Wrk2](https://github.com/giltene/wrk2) and [Gil Tene's talk "How NOT to Measure Latency"](https://www.youtube.com/watch?v=lJ8ydIuPFeU) was the main inspiration for this tool.

**Wrk2 and Hlg both take coordinated omission into account**: Coordinated omission means to ignore requests that should have been sent in the timespan of a slow request. Wrk2 corrects for this numerically, but Hlg differs in that it just increases the number of concurrent requests, enabling us to also observe failures occuring in this interval. Hlg follows Gil's point about creating a full execution plan up front, to avoid being susceptible to the coordinated omission problem.

**Hlg does not require you to choose concurrency parameters**: Wrk2 requires you to choose your concurrency parameters up front. Hlg instead creates new HTTP connections when needed. This gives smoother traffic in situations like this example: `wrk2 --threads 1 --connections 10 --rate 2 --duration 10`. Here Wrk2 will at the beginning immediately send 10 requests all at once, then wait 5 seconds, and then send 10 more all at once.


Testing while developing hlg
---

A Docker container for compiling and running hlg inside:

    docker build -t hlg .
    docker run --name hlg1 --rm -it -v `pwd`:/hlg -v $HOME:/home/user1 -v $HOME/dev/gopackagecachelinux:/root/go --entrypoint /bin/bash hlg

A test webserver to test hlg against:

    docker run -p 80:80 -p 443:443 -it --rm allanrbo/minibackend

A quick way to start Jupyter for plotting results:

    docker run --name tf -d --rm -p 8888:8888 -p 9119:9119 -v `pwd`:/home/jovyan jupyter/tensorflow-notebook start.sh jupyter lab
    docker logs tf 2>&1 | head -n20
