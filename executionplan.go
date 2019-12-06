package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

type request struct {
	when           time.Duration // How many microseconds since the beginning of the benchmark until this request shall be sent
	responseTime   time.Duration // How long time elapsed since "when" until the response was received
	writtenBytes   int
	writtenDone    bool
	completed      bool
	error          bool
	httpCode       int
	responseReader ResponseReader
	workerID       int
	socketfd       int
}

type executionPlan struct {
	reqs             []request
	workerPos        []int      // Element i in this slice represents the position in reqs of the next request the i'th worker must send.
	workerPosTimeout []int      // Element i in this slice represents the position in reqs of the next request the i'th worker should time out if it was not yet finished.
	latestTimeoutReq []*request // Element i in this slice is the current req that the i'th worker should time out if it was not yet finished.
}

func newExecutionPlan(rps int, seconds int, workerCount int) (e *executionPlan) {
	e = &executionPlan{}

	secondsPerRequest := time.Duration(float64(time.Second) / float64(rps))
	e.reqs = make([]request, rps*seconds)
	for i := 1; i < len(e.reqs); i++ {
		e.reqs[i].when = e.reqs[i-1].when + secondsPerRequest
		e.reqs[i].workerID = i % workerCount
	}

	// Initialize worker positions to point to the first request each worker should send.
	e.workerPos = make([]int, workerCount, workerCount)
	e.workerPosTimeout = make([]int, workerCount, workerCount)
	e.latestTimeoutReq = make([]*request, workerCount, workerCount)
	for workerID := 0; workerID < workerCount; workerID++ {
		nextPos := 0
		for nextPos < len(e.reqs) && e.reqs[nextPos].workerID != workerID {
			nextPos++
		}
		e.workerPos[workerID] = nextPos
		e.workerPosTimeout[workerID] = nextPos
	}

	return
}

func (e *executionPlan) getNext(workerID int) (r *request) {
	if e.done(workerID) {
		return
	}

	r = &e.reqs[e.workerPos[workerID]]

	// Set worker position to point to the next request this worker should send.
	nextPos := e.workerPos[workerID]
	nextPos++
	for nextPos < len(e.reqs) && e.reqs[nextPos].workerID != workerID {
		nextPos++
	}
	e.workerPos[workerID] = nextPos

	return
}

// Gets the next request that could potentially time out.
func (e *executionPlan) getNextPotentialTimeoutReq(workerID int) (r *request) {
	e.latestTimeoutReq[workerID] = nil

	if e.workerPosTimeout[workerID] == len(e.reqs) {
		return
	}

	if e.workerPosTimeout[workerID] == e.workerPos[workerID] {
		// There are no requests currently in flight that we haven't already monitored for time out.
		return
	}

	r = &e.reqs[e.workerPosTimeout[workerID]]
	e.latestTimeoutReq[workerID] = r

	// Set worker timeout position to point to the next request this worker should time out if it was not yet finished.
	nextPos := e.workerPosTimeout[workerID]
	nextPos++
	for nextPos < len(e.reqs) && e.reqs[nextPos].workerID != workerID && e.workerPosTimeout[workerID] < e.workerPos[workerID] && !e.reqs[nextPos].completed && !e.reqs[nextPos].error {
		nextPos++
	}
	e.workerPosTimeout[workerID] = nextPos

	return
}

func (e *executionPlan) peekNext(workerID int) (r *request) {
	if e.done(workerID) {
		return
	}

	r = &e.reqs[e.workerPos[workerID]]
	return
}

func (e *executionPlan) done(workerID int) bool {
	return e.workerPos[workerID] == len(e.reqs)
}

func (e *executionPlan) writeResultsFile(filename string) {
	resultsFile, err := os.Create(filename)
	defer resultsFile.Close()
	if err != nil {
		fmt.Printf("%v\n", err)
		return
	}
	resultsFileWriter := bufio.NewWriter(resultsFile)
	fmt.Fprintf(resultsFileWriter, "whenNs")
	fmt.Fprintf(resultsFileWriter, ",written")
	fmt.Fprintf(resultsFileWriter, ",completed")
	fmt.Fprintf(resultsFileWriter, ",error")
	fmt.Fprintf(resultsFileWriter, ",httpCode")
	fmt.Fprintf(resultsFileWriter, ",latencyMs")
	fmt.Fprintf(resultsFileWriter, "\n")
	for _, r := range e.reqs {
		fmt.Fprintf(resultsFileWriter, "%d", r.when)

		w := 0
		if r.writtenDone {
			w = 1
		}
		fmt.Fprintf(resultsFileWriter, ",%d", w)

		c := 0
		if r.completed {
			c = 1
		}
		fmt.Fprintf(resultsFileWriter, ",%d", c)

		e := 0
		if r.error {
			e = 1
		}
		fmt.Fprintf(resultsFileWriter, ",%d", e)

		fmt.Fprintf(resultsFileWriter, ",%d", r.httpCode)

		if r.responseTime != 0 {
			v := float64(r.responseTime) / float64(time.Millisecond)
			fmt.Fprintf(resultsFileWriter, ",%7f", v)
		} else {
			fmt.Fprintf(resultsFileWriter, ",")
		}

		fmt.Fprintf(resultsFileWriter, "\n")
	}
	resultsFileWriter.Flush()
	resultsFile.Close()
}
