TFTP server and client library for Golang
=========================================

[![GoDoc](https://godoc.org/github.com/pin/tftp?status.svg)](https://godoc.org/github.com/pin/tftp)
[![Build Status](https://travis-ci.org/pin/tftp.svg?branch=master)](https://travis-ci.org/pin/tftp)

Implements:
 * [RFC 1350](https://tools.ietf.org/html/rfc1350) - The TFTP Protocol (Revision 2)
 * [RFC 2347](https://tools.ietf.org/html/rfc2347) - TFTP Option Extension
 * [RFC 2348](https://tools.ietf.org/html/rfc2348) - TFTP Blocksize Option

Partially implements (tsize server side only):
 * [RFC 2349](https://tools.ietf.org/html/rfc2349) - TFTP Timeout Interval and Transfer Size Options

Set of features is sufficient for PXE boot support.

``` go
import "github.com/pin/tftp"
```

TFTP Server
-----------

```go
func writeHanlder(filename string, w io.WriterTo) error {
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return err
	}
	// In case client provides tsize option.
	if t, ok := wt.(tftp.IncomingTransfer); ok {
		if n, ok := t.Size(); ok {
			fmt.Printf("Transfer size: %d\n", n)
		}
	}
	n, err := w.WriteTo(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return err
	}
	fmt.Printf("%d bytes received\n", n)
	return nil
}

func readHandler(filename string, r io.ReaderFrom) error {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return err
	}
	// Optional tsize support.
	// Set transfer size before calling ReadFrom.
	if t, ok := rf.(tftp.OutgoingTransfer); ok {
		if fi, err := file.Stat(); err == nil {
			t.SetSize(fi.Size())
		}
	}
	n, err := r.ReadFrom(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return err
	}
	fmt.Printf("%d bytes sent\n", n)
	return nil
}

func main() {
	// use nil in place of handler to disable read or write operations
	s := tftp.NewServer(readHandler, writeHanlder)
	s.SetTimeout(5 * time.Second) // optional
	err := s.ListenAndServe(":69") // blocks until s.Shutdown() is called
	if err != nil {
		fmt.Fprintf(os.Stdout, "server: %v\n", err)
		os.Exit(1)
	}
}
```

TFTP Client
-----------
Uploading file to server:

```go
file, err := os.Open(path)
if err != nil {
	...
}
c, err := tftp.NewClient("172.16.4.21:69")
if err != nil {
	...
}
c.SetTimeout(5 * time.Second) // optional
r, err := c.Send("foobar.txt", "octet")
if err != nil {
	...
}
// Optional tsize.
if ot, ok := r.(tftp.OutgoingTransfer); ok {
	ot.SetSize(length)
}
n, err := r.ReadFrom(file)
fmt.Printf("%d bytes sent\n", n)
```

Downloading file from server:

```go
c, err := tftp.NewClient("172.16.4.21:69")
if err != nil {
	...
}
w, err := c.Receive("foobar.txt", "octet")
if err != nil {
	...
}
file, err := os.Create(path)
if err != nil {
	...
}
// Optional tsize.
if it, ok := readTransfer.(IncomingTransfer); ok {
	if n, ok := it.Size(); ok {
		fmt.Printf("Transfer size: %d\n", n)
	}
}
n, err := w.WriteTo(file)
if err != nil {
	...
}
fmt.Printf("%d bytes received\n", n)
```

Legacy API
----------
API has been improved in non-backward-compartible way recently.
Please use `v1` release in legacy code.

Legacy code should import http://gopkg.in/pin/tftp.v1