package main

import (
	"bytes"
	"fmt"
	"strconv"
)

type reseponseReaderState int

// States for the response reader state machine.
const (
	stateReadResponseLine reseponseReaderState = iota
	stateReadNextHeaderLine
	stateReadBodyWithContentLength
	stateReadBodyChunkedLengthLine
	stateReadBodyChunkedBytes
	stateDone
)

type ResponseReader struct {
	ResponseCode            int
	BodyBytesRead           int
	state                   reseponseReaderState
	carry                   []byte // Header-bytes carried over from previous call to read, in case the previous header ended abruptly.
	contentLength           int
	transferEncodingChunked bool
	curChunkLength          int
	curChunkBytesRead       int
}

var headerKeyTransferEncoding = []byte("Transfer-Encoding")
var headerValChunked = []byte("chunked")
var headerKeyContentLength = []byte("Content-Length")

const maxCarrySizeBytes = 1024 * 50

func (r *ResponseReader) Read(input []byte) (done bool, err error) {
	bb := input
	if len(r.carry) > 0 {
		bb = r.carry
		bb = append(bb, input...)
		r.carry = nil
	}

	for {
		switch r.state {

		case stateReadResponseLine:
			n := bytes.IndexByte(bb, '\n')
			if n == -1 {
				if len(bb) > maxCarrySizeBytes {
					err = fmt.Errorf("response line spanning multiple packets too long")
					return
				}
				r.carry = make([]byte, len(bb))
				copy(r.carry, bb)
				return
			}
			responseLine := bb[:n]
			bb = bb[n+1:]

			// Skip past HTTP version.
			n = bytes.IndexByte(responseLine, ' ')
			if n == -1 {
				err = fmt.Errorf("invalid respose line")
				return
			}
			responseLineRest := responseLine[n+1:]

			// Get response code.
			n = bytes.IndexByte(responseLineRest, ' ')
			if n == -1 {
				n = len(responseLineRest)
			}
			r.ResponseCode, err = strconv.Atoi(string(responseLineRest[:n]))
			if err != nil {
				err = fmt.Errorf("no response code in respose line")
				return
			}

			r.state = stateReadNextHeaderLine

		case stateReadNextHeaderLine:
			n := bytes.IndexByte(bb, '\n')
			if n == -1 {
				if len(bb) > maxCarrySizeBytes {
					err = fmt.Errorf("response header spanning multiple packets too long")
					return
				}
				r.carry = make([]byte, len(bb))
				copy(r.carry, bb)
				return
			}

			headerLine := bb[:n]
			headerLine = bytes.TrimSuffix(headerLine, []byte{'\r'})
			bb = bb[n+1:]

			if len(headerLine) == 0 {
				// Empty header line means end of headers.
				r.state = stateReadBodyWithContentLength
				if r.transferEncodingChunked {
					r.state = stateReadBodyChunkedLengthLine
				}
				continue
			}

			// Get header name.
			n = bytes.IndexByte(headerLine, ':')
			if n == -1 {
				err = fmt.Errorf("invalid header")
				return
			}
			headerName := bytes.TrimSpace(headerLine[:n])

			// Interpret the headers that are of importance to us
			if bytes.EqualFold(headerName, headerKeyContentLength) {
				headerVal := string(bytes.ToLower(bytes.TrimSpace(headerLine[n+1:])))
				r.contentLength, err = strconv.Atoi(headerVal)
				if err != nil {
					err = fmt.Errorf("invalid content-length header")
					return
				}
			} else if bytes.EqualFold(headerName, headerKeyTransferEncoding) {
				headerVal := bytes.ToLower(bytes.TrimSpace(headerLine[n+1:]))
				r.transferEncodingChunked = bytes.EqualFold(headerVal, headerValChunked)
			}

		case stateReadBodyWithContentLength:
			remaining := r.contentLength - r.BodyBytesRead
			n := len(bb)
			if n > remaining {
				n = remaining
			}
			r.BodyBytesRead += n
			if r.BodyBytesRead == r.contentLength {
				r.state = stateDone
				done = true
			}
			return

		case stateReadBodyChunkedLengthLine:
			var chunkLengthLine []byte
			for {
				n := bytes.IndexByte(bb, '\n')
				if n == -1 {
					if len(bb) > 20 {
						err = fmt.Errorf("chunk length line too long")
						return
					}
					r.carry = make([]byte, len(bb))
					copy(r.carry, bb)
					return
				}

				chunkLengthLine = bb[:n]
				bb = bb[n+1:]

				if len(bytes.TrimSpace(chunkLengthLine)) == 0 {
					continue // This looping is to consume line breaks after the chunk bytes right before a chunk length.
				}
				break
			}

			var l int64
			l, err = strconv.ParseInt(string(bytes.TrimSpace(chunkLengthLine)), 16, 64)
			if err != nil {
				err = fmt.Errorf("invalid chunk length")
				return
			}
			r.curChunkLength = int(l)
			r.curChunkBytesRead = 0

			if r.curChunkLength == 0 {
				r.state = stateDone
				done = true
				return
			}

			r.state = stateReadBodyChunkedBytes

		case stateReadBodyChunkedBytes:

			remaining := r.curChunkLength - r.curChunkBytesRead
			n := len(bb)
			if n > remaining {
				n = remaining
			}
			r.curChunkBytesRead += n
			r.BodyBytesRead += n
			bb = bb[n:]
			if r.curChunkBytesRead == r.curChunkLength {
				r.state = stateReadBodyChunkedLengthLine
				if len(bb) > 0 {
					continue
				}
			}
			return

		case stateDone:
			done = true
			return

		}
	}
}
