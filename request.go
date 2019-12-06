package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

type reqPayload struct {
	bytes     []byte
	keepAlive bool
}

var keepAliveHeaderRegex = regexp.MustCompile("(?i:\r\nconnection: *keep-alive\r\n)")

func newHttpReq(reqBytes []byte) (req *reqPayload, err error) {
	headerEndPos := bytes.Index(reqBytes, []byte("\r\n\r\n"))
	if headerEndPos == -1 {
		err = fmt.Errorf("Could not find end of headers (\\r\\n\\r\\n) in request input\n")
		return
	}

	origHeaderLen := headerEndPos + 4
	origHeader := reqBytes[:origHeaderLen]
	body := reqBytes[origHeaderLen:]

	bodyLen := len(reqBytes) - origHeaderLen
	newHeaders := bytes.Replace(origHeader, []byte("{{bodylength}}"), []byte(strconv.Itoa(bodyLen)), -1)

	if bodyLen > 0 {
		body = append(body, []byte("\r\n\r\n")...)
	} else {
		body = nil
	}

	keepAlive := keepAliveHeaderRegex.Match(newHeaders)

	req = &reqPayload{
		bytes:     append(newHeaders, body...),
		keepAlive: keepAlive,
	}

	return
}

var defaultReqBytes = bytes.Replace(bytes.Replace([]byte(`GET / HTTP/1.1
Host: 127.0.0.1
User-Agent: hlg/0.0.0
Accept: */*
Connection: Keep-Alive

`), []byte("\r"), []byte(""), -1), []byte("\n"), []byte("\r\n"), -1) // HTTP craves CRLF
