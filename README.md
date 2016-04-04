TFTP server and client library for Golang
=========================================

[![Build Status](https://travis-ci.org/pin/tftp.svg?branch=master)](https://travis-ci.org/pin/tftp)

``` go
import "github.com/pin/tftp"
```

TFTP Server
-----------

```go
func writeHanlder(filename string, w io.WriterTo) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	n, err := w.WriteTo(file)
	if err != nil {
		return err
	}
	logf("%d bytes received", n)
	return nil
}
func readHandler(filename string, r io.ReaderFrom) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	n, err := r.ReadFrom(file)
	if err != nil {
		return err
	}
	logf("%d bytes sent", n)
	return nil
}
...
s, err := tftp.NewServer(readHandler, writeHanlder)
if err != nil {
	...
}
s.SetTimeout(5 * time.Second) // optional
err := s.ListenAndServe(":69") // blocks until s.Shutdown() called
if err != nil
	...
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
n, err := w.WriteTo(file)
if err != nil {
	...
}
fmt.Printf("%d bytes received\n", n)
```
