package main

import (
	"fmt"
	"net"
	"runtime"
	"sort"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// TODO Are we measuring the latency of failed requests correctly, taking coordinated omission into account?

type Benchmark struct {
	payload            *reqPayload
	addr               unix.SockaddrInet4
	seconds            int
	timeout            time.Duration
	rps                int
	maxConcurrent      int
	verbose            bool
	ep                 *executionPlan
	startTime          time.Time
	startTimeMonotonic int64
	endTime            time.Time
	done               bool
	workerCount        int
	workers            []*benchmarkWorker
}

type benchmarkWorker struct {
	benchmark      *Benchmark
	workerID       int
	reqsInProgress map[int]*request // Find a request item from a file descriptor. These are the requests currently in flight.
	//reqsInProgressTimeouts timeoutHeap
	connRb              *ringbuffer // Ring buffer used to store client connections that can be reused if using HTTP keep-alive.
	epollfd             int
	timerfdReqs         int // Timer file descriptor for scheduling requests.
	timerfdTimeout      int // Timer file descriptor for timeouts.
	timerfdTimeoutArmed bool
	stats               *stats
	buf                 []byte
}

type BenchmarkResult struct {
	startedRate float64
	errors      uint
	recvd       uint
	p99d9       time.Duration
	p99d99      time.Duration
	p99d999     time.Duration
	max         time.Duration
}

func NewBenchmark(payload *reqPayload, ipv4 net.IP, port int, seconds int, rps int, timeout time.Duration, maxConcurrent int, verbose bool) *Benchmark {
	workerCount := runtime.NumCPU()
	b := &Benchmark{
		workerCount:   workerCount,
		payload:       payload,
		seconds:       seconds,
		timeout:       timeout,
		rps:           rps,
		maxConcurrent: maxConcurrent,
		verbose:       verbose,
		ep:            newExecutionPlan(rps, seconds, workerCount),
	}

	b.addr = unix.SockaddrInet4{Port: port}
	copy(b.addr.Addr[:], ipv4)

	return b
}

func (b *Benchmark) Start() (r BenchmarkResult, err error) {
	b.startTime = time.Now()

	var t unix.Timespec
	err = unix.ClockGettime(unix.CLOCK_MONOTONIC, &t)
	if err != nil {
		return
	}
	b.startTimeMonotonic = unix.TimespecToNsec(t)

	for i := 0; i < b.workerCount; i++ {
		w := &benchmarkWorker{
			benchmark:      b,
			workerID:       i,
			reqsInProgress: make(map[int]*request),
			connRb:         &ringbuffer{},
			stats:          newStats(),
			buf:            make([]byte, 32*1024),
		}

		b.workers = append(b.workers, w)

		go w.startWorker()
	}

	for {
		time.Sleep(1 * time.Second)
		if b.elapsed() > time.Duration(b.seconds)*time.Second {
			break
		}
		if b.verbose {
			b.printStatus()
		}
	}

	b.endTime = time.Now()
	b.done = true

	// Wait until final requests have timed out
	for i := 0; b.reqsConcurrent() > 0; i++ {
		time.Sleep(100 * time.Millisecond)
		if i%10 == 0 && b.verbose {
			b.printStatus()
		}
	}

	b.ep.writeResultsFile("latencies.csv")

	r = b.calculateResult()
	if b.verbose {
		b.printSummary(r)
	}

	return
}

func (b *benchmarkWorker) startWorker() (err error) {
	defer b.closeAllFds()

	// Open the epoll file descriptor.
	b.epollfd, err = unix.EpollCreate1(0)
	if err != nil {
		err = fmt.Errorf("epoll_create1 failed: %v", err)
		return
	}

	b.timerfdReqs, err = b.createTimerFd()
	if err != nil {
		return
	}

	b.timerfdTimeout, err = b.createTimerFd()
	if err != nil {
		return
	}

	if b.workerID == 0 {
		// Send initial request immediately.
		b.handleReqTimerTriggered()
	} else {
		err = b.scheduleNextRequest()
		if err != nil {
			return
		}
	}

	err = b.eventLoop()
	if err != nil {
		return
	}

	return
}

func (b *benchmarkWorker) eventLoop() (err error) {
	var events [50]unix.EpollEvent
	for {
		// TODO if there are already known overdue requests to send, then do use the timerfd to schedule next event and do not let EpollWait block, but still run it in order to process any ongoing traffic

		// Wait until one or more events occur.
		var nevents int
		nevents, err = unix.EpollWait(b.epollfd, events[:], 100)
		if err != nil {
			err = fmt.Errorf("epoll_wait failed: %v", error(err))
			panic(err)
		}

		for i := 0; i < nevents; i++ {
			fd := int(events[i].Fd)
			curReq := b.reqsInProgress[fd]

			// Handle connections that got closed.
			if events[i].Events&unix.EPOLLHUP != 0 || events[i].Events&unix.EPOLLRDHUP != 0 {
				err = b.handleConnectionClosed(fd, curReq)
				if err != nil {
					panic(err)
				}

				continue
			}

			// Handle connections that are ready to be read from.
			if events[i].Events&unix.EPOLLIN != 0 {
				// Handle timer events for request scheduling.
				if fd == b.timerfdReqs {
					err = b.handleReqTimerTriggered()
					if err != nil {
						panic(err)
					}

					continue
				}

				// Handle timer events for timeouts.
				if fd == b.timerfdTimeout {
					err = b.handleTimeoutTimerTriggered()
					if err != nil {
						panic(err)
					}

					continue
				}

				err = b.handleConnectionReadyToRead(fd, curReq)
				if err != nil {
					panic(err)
				}

				continue
			}

			// Handle connections that are ready to be written to.
			if events[i].Events&unix.EPOLLOUT != 0 {
				if curReq == nil {
					err = fmt.Errorf("got unexpected EPOLLOUT event for %d\n", fd)
					panic(err)
				}

				b.writeRequest(curReq, fd)

				continue
			}

			// Handle connections on which an error happened.
			if events[i].Events&unix.EPOLLERR != 0 {
				b.connRb.remove(fd)
				err = unix.EpollCtl(b.epollfd, unix.EPOLL_CTL_DEL, fd, nil)
				if err != nil {
					err = fmt.Errorf("epoll_ctl failed on EPOLL_CTL_DEL for client socket fd %d: %v", fd, error(err))
					panic(err)
				}

				continue
			}
		}

		if b.benchmark.done && len(b.reqsInProgress) == 0 {
			return
		}
	}

	return
}

func (b *benchmarkWorker) handleReqTimerTriggered() (err error) {
	if b.benchmark.done {
		err = b.scheduleNextRequest()
		return
	}

	// Send current request.
	curReq := b.benchmark.ep.getNext(b.workerID)
	if curReq == nil {
		return // TODO this shouldn't be needed...
	}
	b.stats.reqsStarted++
	/*
		if b.benchmark.reqsConcurrent() >= b.benchmark.maxConcurrent {
			curReq.error = true
			b.stats.errorsTooManyConcurrent++
			return
		}
	*/
	err = b.issueRequest(curReq)
	if err != nil {
		return
	}

	if !b.timerfdTimeoutArmed {
		err = b.scheduleNextTimeout()
		if err != nil {
			return
		}
	}

	err = b.scheduleNextRequest()
	if err != nil {
		return
	}

	return
}

func (b *benchmarkWorker) scheduleNextRequest() (err error) {
	// Schedule next request.
	next := b.benchmark.ep.peekNext(b.workerID)
	if b.benchmark.done || next == nil {
		// No more requests.
		err = timerFdSetTime(0, b.timerfdReqs)
		return
	}

	timeSinceBeginning := time.Now().Sub(b.benchmark.startTime)
	timeUntilNext := next.when - timeSinceBeginning
	if timeUntilNext < 1 {
		timeUntilNext = 1 // 0 means never trigger, so use 1 nanosecond instead.
	}

	err = timerFdSetTime(timeUntilNext, b.timerfdReqs)
	return
}

func (b *benchmarkWorker) scheduleNextTimeout() (err error) {
	timeSinceBeginning := time.Now().Sub(b.benchmark.startTime)

	for {
		r := b.benchmark.ep.getNextPotentialTimeoutReq(b.workerID)
		if r == nil {
			b.timerfdTimeoutArmed = false
			err = timerFdSetTime(0, b.timerfdTimeout)
			return
		}

		if r.completed || r.error {
			continue
		}

		timeUntilTimeout := (r.when + b.benchmark.timeout) - timeSinceBeginning
		if timeUntilTimeout < 1 {
			// Already timed out.
			err = b.timeoutRequest(r)
			if err != nil {
				return
			}
			continue
		}

		b.timerfdTimeoutArmed = true
		err = timerFdSetTime(timeUntilTimeout, b.timerfdTimeout)
		return
	}
}

func (b *benchmarkWorker) handleTimeoutTimerTriggered() (err error) {
	r := b.benchmark.ep.latestTimeoutReq[b.workerID]
	err = b.timeoutRequest(r)
	if err != nil {
		return
	}

	err = b.scheduleNextTimeout()
	return
}

func (b *benchmarkWorker) timeoutRequest(r *request) (err error) {
	if r != nil && !r.completed && !r.error {
		err = unix.EpollCtl(b.epollfd, unix.EPOLL_CTL_DEL, r.socketfd, nil)
		if err != nil {
			err = fmt.Errorf("epoll_ctl failed on EPOLL_CTL_DEL for client socket fd %d: %v", r.socketfd, error(err))
			return
		}
		unix.Close(r.socketfd)
		delete(b.reqsInProgress, r.socketfd)
		r.socketfd = 0
		r.responseReader = ResponseReader{}
		r.error = true
		b.stats.errorsTimeout++
		r.responseTime = time.Since(b.benchmark.startTime) - r.when
		b.stats.recordValue(b.benchmark.timeout) // TODO maybe use a more real value instead...
	}

	return
}

func (b *benchmarkWorker) handleConnectionReadyToRead(fd int, curReq *request) (err error) {
	var n int
	n, err = unix.Read(fd, b.buf)
	if err != nil {
		err = fmt.Errorf("read error: %v", error(err))
		return
	}

	if n == 0 {
		err = b.handleConnectionClosed(fd, curReq)
		return
	}

	if curReq == nil {
		err = fmt.Errorf("got unexpected EPOLLIN event for %d\n", fd)
		return
	}

	curReq.completed, err = curReq.responseReader.Read(b.buf[:n])
	if err != nil {
		curReq.error = true
		b.stats.errorsResponseReader++
		curReq.responseTime = time.Since(b.benchmark.startTime) - curReq.when
	}

	if curReq.completed {
		curReq.httpCode = curReq.responseReader.ResponseCode
		if curReq.httpCode != 200 {
			curReq.error = true
			b.stats.errorsUnexpectedHttpCode++
		}
		if 0 <= curReq.httpCode && curReq.httpCode < 1000 {
			b.stats.httpCodes[curReq.httpCode]++
		}

		curReq.responseTime = time.Since(b.benchmark.startTime) - curReq.when

		delete(b.reqsInProgress, fd)

		b.stats.recordValue(curReq.responseTime)

		err = unix.EpollCtl(b.epollfd, unix.EPOLL_CTL_MOD, fd, &unix.EpollEvent{Events: unix.EPOLLRDHUP, Fd: int32(fd)})
		if err != nil {
			err = fmt.Errorf("epoll_ctl failed on EPOLL_CTL_MOD removing EPOLLIN and EPOLLOUT: %v", err)
			return
		}

		if b.benchmark.payload.keepAlive {
			b.connRb.put(fd)
		} else {
			unix.Close(fd)
		}
	}

	return
}

func (b *benchmarkWorker) handleConnectionClosed(fd int, curReq *request) (err error) {
	// Delete client socket fd from epoll.
	err = unix.EpollCtl(b.epollfd, unix.EPOLL_CTL_DEL, fd, nil)
	if err != nil {
		err = fmt.Errorf("epoll_ctl failed when deleting client socket fd: %v", error(err))
		return
	}

	unix.Close(fd)

	if curReq == nil {
		b.connRb.remove(fd)
	} else {
		// It's OK for an HTTP server to close the socket at any time. So we will reissue the request if this happened.
		// Source: https://www.oreilly.com/library/view/http-the-definitive/1565925092/ch04s07.html,
		delete(b.reqsInProgress, fd)
		b.reissueRequest(curReq)
	}

	return
}

func (b *benchmarkWorker) issueRequest(curReq *request) (err error) {
	socketfd, ok := b.connRb.get()

	// If there was not an existing connection that could be reused we will create one.
	if !ok {
		// Create non-blocking client socket.
		socketfd, err = unix.Socket(unix.AF_INET, unix.O_NONBLOCK|unix.SOCK_STREAM, 0)
		if err != nil {
			if err.Error() == "too many open files" {
				panic("benchmark tool is being hindered by OS limit on number of open files.")
			}
			curReq.error = true
			b.stats.errorsSocketCreate++
			err = nil // Not a fatal error for the benchmark as a whole
			return
		}

		// Connect client socket.
		unix.Connect(socketfd, &b.benchmark.addr)
		if err != nil {
			curReq.error = true
			b.stats.errorsSocketConnect++
			err = nil // Not a fatal error for the benchmark as a whole
			return
		}

		unix.SetsockoptInt(socketfd, unix.IPPROTO_TCP, unix.TCP_NODELAY, 1)
		if err != nil {
			curReq.error = true
			b.stats.errorsSocketSetSockOpt++
			err = nil // Not a fatal error for the benchmark as a whole
			return
		}

		err = unix.EpollCtl(b.epollfd, unix.EPOLL_CTL_ADD, socketfd, &unix.EpollEvent{Events: unix.EPOLLRDHUP, Fd: int32(socketfd)})
		if err != nil {
			err = fmt.Errorf("epoll_ctl failed on EPOLL_CTL_ADD client socket %d: %v", socketfd, err)
			return
		}
	}

	curReq.socketfd = socketfd

	b.writeRequest(curReq, socketfd)
	if err != nil {
		return
	}

	b.reqsInProgress[socketfd] = curReq

	return
}

func (b *benchmarkWorker) writeRequest(curReq *request, socketfd int) (err error) {
	// Write request bytes.
	n, err := unix.Write(socketfd, b.benchmark.payload.bytes[curReq.writtenBytes:])
	if err != nil {
		if err == unix.EAGAIN {
			err = nil // Not a fatal error for the benchmark as a whole
		} else {
			curReq.error = true
			b.stats.errorsSocketWrite++
			return
		}
	} else {
		curReq.writtenBytes += n
		if curReq.writtenBytes == len(b.benchmark.payload.bytes) {
			curReq.writtenDone = true
		}
		b.stats.reqsWritten++
	}

	// Add the socket to epoll.
	if curReq.writtenDone {
		// We are done writing, so we want a notification when there is data to read.
		err = unix.EpollCtl(b.epollfd, unix.EPOLL_CTL_MOD, socketfd, &unix.EpollEvent{Events: unix.EPOLLIN | unix.EPOLLRDHUP, Fd: int32(socketfd)})
		if err != nil {
			err = fmt.Errorf("epoll_ctl failed on EPOLL_CTL_MOD EPOLLIN: %v", err)
			return
		}
	} else {
		// We are not done writing, so we want a notification when we can write more data.
		err = unix.EpollCtl(b.epollfd, unix.EPOLL_CTL_MOD, socketfd, &unix.EpollEvent{Events: unix.EPOLLOUT | unix.EPOLLRDHUP, Fd: int32(socketfd)})
		if err != nil {
			err = fmt.Errorf("epoll_ctl failed on EPOLL_CTL_MOD EPOLLOUT: %v", err)
			return
		}
	}

	return
}

func (b *benchmarkWorker) reissueRequest(curReq *request) (err error) {
	if curReq.completed || curReq.error {
		return
	}

	// Since it's ok for a connection to be closed remotely, we don't consider it an error. Instead, reset and reissue the request.
	curReq.writtenBytes = 0
	if curReq.writtenDone {
		b.stats.reqsWritten--
	}
	curReq.writtenDone = false

	err = b.issueRequest(curReq)
	return
}

func (b *benchmarkWorker) closeAllFds() {
	// Close all fd's from requests still in flight.
	for fd, r := range b.reqsInProgress {
		if r == nil {
			continue
		}

		r.error = true
		b.stats.errorsNoResponse++ // TODO this should not be possible anymore now that timeouts are implemented.
		unix.Close(fd)
	}
	b.reqsInProgress = make(map[int]*request)

	// Close all fd's in ringbuffer.
	for {
		if fd, ok := b.connRb.get(); ok {
			unix.Close(fd)
		} else {
			break
		}
	}

	unix.Close(int(b.timerfdReqs))
	//unix.Close(int(b.timerfdControl))
	unix.Close(b.epollfd)
}

func (b *benchmarkWorker) createTimerFd() (timerfd int, err error) {
	// Create a timer file descriptor that can be used to trigger epoll after a given timespan.
	tfd, _, errno := unix.Syscall(unix.SYS_TIMERFD_CREATE, unix.CLOCK_MONOTONIC, 0, 0)
	if errno != 0 {
		err = fmt.Errorf("timerfd_create failed: %v", error(err))
		return
	}
	timerfd = int(tfd)

	// Add the timerfd to epoll.
	err = unix.EpollCtl(b.epollfd, unix.EPOLL_CTL_ADD, int(timerfd), &unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(timerfd)})
	if err != nil {
		err = fmt.Errorf("epoll_ctl failed when adding timerfd: %v", error(err))
		return
	}

	return
}

// Set up the timerfd to make epoll wake up after the given timespan.
func timerFdSetTime(d time.Duration, timerfd int) (err error) {
	its := itimerspec{it_value: unix.NsecToTimespec(int64(d))}
	_, _, errno := unix.Syscall(unix.SYS_TIMERFD_SETTIME, uintptr(timerfd), 0, uintptr(unsafe.Pointer(&its)))
	if errno != 0 {
		err = fmt.Errorf("timerfd_settime failed: %v", error(err))
		return
	}

	return
}

// From Linux's time.h
type itimerspec struct {
	it_interval unix.Timespec // Interval for periodic timer
	it_value    unix.Timespec // Initial expiration
}

func (b *Benchmark) elapsed() time.Duration {
	et := time.Now()
	if !b.endTime.IsZero() {
		et = b.endTime
	}

	return et.Sub(b.startTime)
}

func (b *Benchmark) printStatus() {
	var max time.Duration
	var reqsConcurrent int
	var connsAlive int
	var reqsStarted uint
	var reqsWritten uint
	var respRecvd uint
	var errors uint

	for _, w := range b.workers {
		if w.stats.max > max {
			max = w.stats.max
		}

		reqsConcurrent += len(w.reqsInProgress)
		connsAlive += len(w.reqsInProgress) + w.connRb.size
		reqsStarted += w.stats.reqsStarted
		reqsWritten += w.stats.reqsWritten
		respRecvd += w.stats.respRecvd
		errors += w.stats.errorsTooManyConcurrent +
			w.stats.errorsResponseReader +
			w.stats.errorsNoResponse +
			w.stats.errorsTimeout +
			w.stats.errorsSocketCreate +
			w.stats.errorsSocketSetSockOpt +
			w.stats.errorsSocketConnect +
			w.stats.errorsSocketWrite +
			w.stats.errorsUnexpectedHttpCode
	}

	elapsed := b.elapsed()

	startedRate := float64(reqsStarted) / float64(float64(elapsed)/float64(time.Second))
	writtenRate := float64(reqsWritten) / float64(float64(elapsed)/float64(time.Second))

	maxResponseTimeMs := float64(max) / float64(time.Millisecond)

	line := "alive: %4d, concurrent: %4d, startedRate: %9.2f , writtenRate: %9.2f ,  started: %6d , recvd:  %6d , errors: %6d, maxMs: %9.2f\n"
	fmt.Printf(line,
		connsAlive,
		reqsConcurrent,
		startedRate,
		writtenRate,
		reqsStarted,
		respRecvd,
		errors,
		maxResponseTimeMs)
}

func (b *Benchmark) reqsConcurrent() (r int) {
	for _, w := range b.workers {
		r += len(w.reqsInProgress)
	}
	return
}

func (b *Benchmark) printSummary(r BenchmarkResult) {
	var errorsTooManyConcurrent uint
	var errorsResponseReader uint
	var errorsNoResponse uint
	var errorsTimeout uint
	var errorsSocketCreate uint
	var errorsSocketConnect uint
	var errorsSocketSetSockOpt uint
	var errorsSocketWrite uint
	var errorsUnexpectedHttpCode uint
	var httpCodes [1000]uint

	for _, w := range b.workers {
		errorsTooManyConcurrent += w.stats.errorsTooManyConcurrent
		errorsResponseReader += w.stats.errorsResponseReader
		errorsNoResponse += w.stats.errorsNoResponse
		errorsTimeout += w.stats.errorsTimeout
		errorsSocketCreate += w.stats.errorsSocketCreate
		errorsSocketConnect += w.stats.errorsSocketConnect
		errorsSocketSetSockOpt += w.stats.errorsSocketSetSockOpt
		errorsSocketWrite += w.stats.errorsSocketWrite
		errorsUnexpectedHttpCode += w.stats.errorsUnexpectedHttpCode
		for i := 0; i < len(w.stats.httpCodes); i++ {
			if w.stats.httpCodes[i] == 0 {
				continue
			}

			httpCodes[i] += w.stats.httpCodes[i]
		}
	}

	fmt.Printf("startedRate rps           %11.2f\n", r.startedRate)
	fmt.Printf("recvd                     %8d\n", r.recvd)
	fmt.Printf("p99d9 ms                  %11.2f\n", float64(r.p99d9)/float64(time.Millisecond))
	fmt.Printf("p99d99 ms                 %11.2f\n", float64(r.p99d99)/float64(time.Millisecond))
	fmt.Printf("p99d999 ms                %11.2f\n", float64(r.p99d999)/float64(time.Millisecond))
	fmt.Printf("max ms                    %11.2f\n", float64(r.max)/float64(time.Millisecond))
	fmt.Printf("errorsTooManyConcurrent   %8d\n", errorsTooManyConcurrent)
	fmt.Printf("errorsTooManyConcurrent   %8d\n", errorsTooManyConcurrent)
	fmt.Printf("errorsResponseReader      %8d\n", errorsResponseReader)
	fmt.Printf("errorsNoResponse          %8d\n", errorsNoResponse)
	fmt.Printf("errorsTimeout             %8d\n", errorsTimeout)
	fmt.Printf("errorsSocketCreate        %8d\n", errorsSocketCreate)
	fmt.Printf("errorsSocketConnect       %8d\n", errorsSocketConnect)
	fmt.Printf("errorsSocketSetSockOpt    %8d\n", errorsSocketSetSockOpt)
	fmt.Printf("errorsSocketWrite         %8d\n", errorsSocketWrite)
	fmt.Printf("errorsUnexpectedHttpCode  %8d\n", errorsUnexpectedHttpCode)
	for i := 0; i < len(httpCodes); i++ {
		if httpCodes[i] == 0 {
			continue
		}

		fmt.Printf("completedWithCode%03d      %8d\n", i, httpCodes[i])
	}
}

func (b *Benchmark) calculateResult() (r BenchmarkResult) {
	latencies := make([]time.Duration, 0, len(b.ep.reqs))
	for _, r := range b.ep.reqs {
		if r.responseTime == 0 {
			continue
		}

		latencies = append(latencies, r.responseTime)
	}
	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		r.p99d9 = latencies[int(float64(len(latencies)-1)*0.999)]
		r.p99d99 = latencies[int(float64(len(latencies)-1)*0.9999)]
		r.p99d999 = latencies[int(float64(len(latencies)-1)*0.99999)]
	}

	var reqsStarted uint
	for _, w := range b.workers {
		if w.stats.max > r.max {
			r.max = w.stats.max
		}

		reqsStarted += w.stats.reqsStarted
		r.recvd += w.stats.respRecvd
		r.errors += w.stats.errorsTooManyConcurrent +
			w.stats.errorsResponseReader +
			w.stats.errorsNoResponse +
			w.stats.errorsTimeout +
			w.stats.errorsSocketCreate +
			w.stats.errorsSocketSetSockOpt +
			w.stats.errorsSocketConnect +
			w.stats.errorsSocketWrite +
			w.stats.errorsUnexpectedHttpCode
	}

	elapsed := b.elapsed()
	r.startedRate = float64(reqsStarted) / float64(float64(elapsed)/float64(time.Second))

	return
}
