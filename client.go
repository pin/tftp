package tftp

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

/*
Client provides TFTP client functionality. It requires remote address and
optional logger

Uploading file to server example

	addr, e := net.ResolveUDPAddr("udp", "example.org:69")
	if e != nil {
		...
	}
	file, e := os.Open("/etc/passwd")
	if e != nil {
		...
	}
	r := bufio.NewReader(file)
	log := log.New(os.Stderr, "", log.Ldate | log.Ltime)
	c := tftp.Client{addr, log}
	c.Put(filename, mode, func(writer *io.PipeWriter) {
		n, writeError := r.WriteTo(writer)
		if writeError != nil {
			fmt.Fprintf(os.Stderr, "Can't put %s: %v\n", filename, writeError);
		} else {
			fmt.Fprintf(os.Stderr, "Put %s (%d bytes)\n", filename, n);
		}
		writer.Close()
	})

Downloading file from server example

	addr, e := net.ResolveUDPAddr("udp", "example.org:69")
	if e != nil {
		...
	}
	file, e := os.Create("/var/tmp/debian.img")
	if e != nil {
		...
	}
	w := bufio.NewWriter(file)
	log := log.New(os.Stderr, "", log.Ldate | log.Ltime)
	c := tftp.Client{addr, log}
	c.Get(filename, mode, func(reader *io.PipeReader) {
		n, readError := w.ReadFrom(reader)
		if readError != nil {
			fmt.Fprintf(os.Stderr, "Can't get %s: %v\n", filename, readError);
		} else {
			fmt.Fprintf(os.Stderr, "Got %s (%d bytes)\n", filename, n);
		}
		w.Flush()
		file.Close()
	})
*/
type Client struct {
	RemoteAddr *net.UDPAddr
	Log        *log.Logger
}

// Method for uploading file to server
func (c Client) Put(filename string, mode string, handler func(w *io.PipeWriter)) error {
	addr, e := net.ResolveUDPAddr("udp", ":0")
	if e != nil {
		return e
	}
	conn, e := net.ListenUDP("udp", addr)
	if e != nil {
		return e
	}
	reader, writer := io.Pipe()
	s := &sender{c.RemoteAddr, conn, reader, filename, mode, c.Log}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		handler(writer)
		wg.Done()
	}()
	s.run(false)
	wg.Wait()
	return nil
}

// Method for downloading file from server
func (c Client) Get(filename string, mode string, handler func(r *io.PipeReader)) error {
	addr, e := net.ResolveUDPAddr("udp", ":0")
	if e != nil {
		return e
	}
	conn, e := net.ListenUDP("udp", addr)
	if e != nil {
		return e
	}
	reader, writer := io.Pipe()
	r := &receiver{c.RemoteAddr, conn, writer, filename, mode, c.Log}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		handler(reader)
		wg.Done()
	}()
	r.run(false)
	wg.Wait()
	return fmt.Errorf("Send timeout")
}
