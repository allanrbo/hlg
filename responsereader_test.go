package main

import (
	"bytes"
	"testing"
)

func TestSimpleResponse(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: 2

Hi

`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != true {
		t.Fatalf("Unexpected done: %v", done)
	}

	if r.ResponseCode != 200 {
		t.Fatalf("Unexpected responseCode: %d", r.ResponseCode)
	}

	if r.BodyBytesRead != 2 {
		t.Fatalf("Unexpected bodyBytesRead: %v", r.BodyBytesRead)
	}
}

// Splits the response at all possible positions and feeds into the read method in two parts.
func TestResponseSplit(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: 2

Hi`))
	for i := 0; i < len(respBytes); i++ {
		r := ResponseReader{}
		bb1 := respBytes[:i]
		bb2 := respBytes[i:]

		// Act
		done1, err1 := r.Read(bb1)
		done2, err2 := r.Read(bb2)

		// Assert
		if err1 != nil {
			t.Fatalf("Unexpected error1 on iteration %d %T: %v", i, err1, err1)
		}

		if done1 != false {
			t.Fatalf("Unexpected done1 on iteration %d: %v", i, done1)
		}

		if err2 != nil {
			t.Fatalf("Unexpected error2 on iteration %d %T: %v", i, err2, err2)
		}

		if done2 != true {
			t.Fatalf("Unexpected done2 on iteration %d: %v", i, done2)
		}

		if r.ResponseCode != 200 {
			t.Fatalf("Unexpected responseCode on iteration %d: %d", i, r.ResponseCode)
		}

		if r.BodyBytesRead != 2 {
			t.Fatalf("Unexpected bodyBytesRead on iteration %d: %v", i, r.BodyBytesRead)
		}
	}
}

func TestInvalidResponseLine1(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1200OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: 2

Hi

`))
	r := ResponseReader{}

	// Act
	_, err := r.Read(respBytes)

	// Assert
	if err.Error() != "invalid respose line" {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}
}

func TestInvalidResponseLine2(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: 2

Hi

`))
	r := ResponseReader{}

	// Act
	_, err := r.Read(respBytes)

	// Assert
	if err.Error() != "no response code in respose line" {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}
}

func TestInvalidResponseLine3(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200x OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: 2

Hi

`))
	r := ResponseReader{}

	// Act
	_, err := r.Read(respBytes)

	// Assert
	if err.Error() != "no response code in respose line" {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}
}

func TestExtraRead(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: 2

Hi

`))
	r := ResponseReader{}

	// Act
	done1, err1 := r.Read(respBytes)
	done2, err2 := r.Read([]byte{})

	// Assert
	if err1 != nil {
		t.Fatalf("Unexpected error1 %T: %v", err1, err1)
	}

	if done1 != true {
		t.Fatalf("Unexpected done1: %v", done1)
	}

	if err2 != nil {
		t.Fatalf("Unexpected error2 %T: %v", err2, err2)
	}

	if done2 != true {
		t.Fatalf("Unexpected done2: %v", done2)
	}

	if r.ResponseCode != 200 {
		t.Fatalf("Unexpected responseCode: %d", r.ResponseCode)
	}

	if r.BodyBytesRead != 2 {
		t.Fatalf("Unexpected bodyBytesRead: %v", r.BodyBytesRead)
	}
}

func TestMaxCarryResponseLine(t *testing.T) {
	// Arrange
	buf1 := bytes.Buffer{}
	buf1.WriteString(`HTTP/1.1 200 OK xxxxxxxxxx`)
	for i := 0; i < (1024*60)/10; i++ {
		buf1.WriteString(`xxxxxxxxxx`)
	}

	bb1 := forceCRLF(buf1.Bytes())
	r := ResponseReader{}

	// Act
	_, err1 := r.Read(bb1)

	// Assert
	if err1.Error() != "response line spanning multiple packets too long" {
		t.Fatalf("Unexpected error1 %T: %v", err1, err1)
	}
}

func TestMaxCarryHeader(t *testing.T) {
	// Arrange
	buf1 := bytes.Buffer{}
	buf1.WriteString(`HTTP/1.1 200 OK
Server: xxxxxxxxx`)
	for i := 0; i < (1024*60)/10; i++ {
		buf1.WriteString(`xxxxxxxxxx`)
	}

	bb1 := forceCRLF(buf1.Bytes())
	r := ResponseReader{}

	// Act
	_, err1 := r.Read(bb1)

	// Assert
	if err1.Error() != "response header spanning multiple packets too long" {
		t.Fatalf("Unexpected error1 %T: %v", err1, err1)
	}
}

func TestNoContentLength(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html

this should be ignored

`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != true {
		t.Fatalf("Unexpected done: %v", done)
	}

	if r.ResponseCode != 200 {
		t.Fatalf("Unexpected responseCode: %d", r.ResponseCode)
	}

	if r.BodyBytesRead != 0 {
		t.Fatalf("Unexpected bodyBytesRead: %v", r.BodyBytesRead)
	}
}

func TestNoHeaders(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK

`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != true {
		t.Fatalf("Unexpected done: %v", done)
	}

	if r.ResponseCode != 200 {
		t.Fatalf("Unexpected responseCode: %d", r.ResponseCode)
	}

	if r.BodyBytesRead != 0 {
		t.Fatalf("Unexpected bodyBytesRead: %v", r.BodyBytesRead)
	}
}

func TestIncompleteHeaders(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != false {
		t.Fatalf("Unexpected done: %v", done)
	}
}

func TestMissingHeaderDelimeter(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
someStrangeHeaderWithoutDelimiter
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: 2

Hi

`))
	r := ResponseReader{}

	// Act
	_, err := r.Read(respBytes)

	// Assert
	if err.Error() != "invalid header" {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}
}

func TestContentLengthNonNumeric(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: abc

Hi

`))
	r := ResponseReader{}

	// Act
	_, err := r.Read(respBytes)

	// Assert
	if err.Error() != "invalid content-length header" {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}
}

func TestIncompleteBody(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Content-Length: 3

Hi`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != false {
		t.Fatalf("Unexpected done: %v", done)
	}
}

func TestChunked(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Transfer-Encoding: chunked

1A
<hello><hello><hello></hel
13
lo></hello></hello>
0

`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != true {
		t.Fatalf("Unexpected done: %v", done)
	}

	if r.BodyBytesRead != 45 {
		t.Fatalf("Unexpected bodyBytesRead: %v", r.BodyBytesRead)
	}
}

// Splits the chunked response (chunked at HTTP level) at all possible positions (split at TCP level) and feeds into the read method in two parts.
func TestChunkedSplit(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Transfer-Encoding: chunked

1A
<hello><hello><hello></hel
13
lo></hello></hello>
0
`))
	for i := 0; i < len(respBytes); i++ {
		r := ResponseReader{}
		bb1 := respBytes[:i]
		bb2 := respBytes[i:]

		// Act
		done1, err1 := r.Read(bb1)
		done2, err2 := r.Read(bb2)

		// Assert
		if err1 != nil {
			t.Fatalf("Unexpected error1 on iteration %d %T: %v", i, err1, err1)
		}

		if done1 == true {
			t.Fatalf("Unexpected done1 on iteration %d: %v", i, done1)
		}

		if err2 != nil {
			t.Fatalf("Unexpected error2 on iteration %d %T: %v", i, err2, err2)
		}

		if done2 != true {
			t.Fatalf("Unexpected done2 on iteration %d: %v", i, done2)
		}

		if r.ResponseCode != 200 {
			t.Fatalf("Unexpected responseCode on iteration %d: %d", i, r.ResponseCode)
		}

		if r.BodyBytesRead != 45 {
			t.Fatalf("Unexpected bodyBytesRead on iteration %d: %v", i, r.BodyBytesRead)
		}
	}
}

func TestChunkLengthSuddenEnd(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Transfer-Encoding: chunked

1A`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != false {
		t.Fatalf("Unexpected done: %v", done)
	}
}

func TestChunkLengthSuddenEnd2(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Transfer-Encoding: chunked

1A1A1A1A1A1A1A1A1A1A1A1A1A1A1A1A`))
	r := ResponseReader{}

	// Act
	_, err := r.Read(respBytes)

	// Assert
	if err.Error() != "chunk length line too long" {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}
}

func TestChunkSuddenEnd(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Transfer-Encoding: chunked

1A
`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != false {
		t.Fatalf("Unexpected done: %v", done)
	}
}

func TestChunkSuddenEnd2(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Transfer-Encoding: chunked

1A
<hello><hello><hello></hel
13
lo></hello></hello>`))
	r := ResponseReader{}

	// Act
	done, err := r.Read(respBytes)

	// Assert
	if err != nil {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}

	if done != false {
		t.Fatalf("Unexpected done: %v", done)
	}
}

func TestChunkedInvalidLength(t *testing.T) {
	// Arrange
	respBytes := forceCRLF([]byte(`HTTP/1.1 200 OK
Server: nginx/1.10.3 (Ubuntu)
Date: Tue, 12 Jun 2018 18:09:49 GMT
Content-Type: text/html
Transfer-Encoding: chunked

1G
<hello><hello><hello></hel
13
lo></hello></hello>
0

`))
	r := ResponseReader{}

	// Act
	_, err := r.Read(respBytes)

	// Assert
	if err.Error() != "invalid chunk length" {
		t.Fatalf("Unexpected error %T: %v", err, err)
	}
}

func forceCRLF(bb []byte) []byte {
	bb = bytes.Replace(bb, []byte("\r"), []byte(""), -1)
	bb = bytes.Replace(bb, []byte("\n"), []byte("\r\n"), -1)
	return bb
}
