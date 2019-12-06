package main

import (
	"time"
)

type stats struct {
	reqsStarted              uint
	reqsWritten              uint
	respRecvd                uint
	errorsTooManyConcurrent  uint
	errorsResponseReader     uint
	errorsNoResponse         uint
	errorsTimeout            uint
	errorsSocketCreate       uint
	errorsSocketSetSockOpt   uint
	errorsSocketConnect      uint
	errorsSocketWrite        uint
	errorsUnexpectedHttpCode uint
	httpCodes                [1000]uint
	max                      time.Duration
}

func newStats() (s *stats) {
	s = &stats{}
	return
}

func (s *stats) recordValue(d time.Duration) {
	if d > s.max {
		s.max = d
	}

	s.respRecvd++
}
