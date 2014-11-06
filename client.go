package tftp

import (
	"net"
	"io"
	"log"
	"fmt"
	"time"
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
	reader, writer := io.Pipe()
	go func() {
		r.WriteTo(writer);
		writer.Close()
	}()
	c.Put(filename, mode, reader)
	if e != nil {
		...
	}
	
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
	reader, writer := io.Pipe()
	go c.Get(filename, mode, writer)
	w.ReadFrom(reader)
	w.Flush()
	file.Close()
*/

type Client struct {
	RemoteAddr *net.UDPAddr
	Log *log.Logger
}

// Method for uploading file to server
func (c Client) Put(filename string, mode string, reader *io.PipeReader) (error) {
	addr, e := net.ResolveUDPAddr("udp", ":0")
	if e != nil {
		return e
	}
	conn, e := net.ListenUDP("udp", addr)
	if e != nil {
		return e
	}
	var buffer []byte
	buffer = make([]byte, 50)
	for i := 0; i < 3; i++ {
		wrqPacket := WRQ{filename, mode}
		conn.WriteToUDP(wrqPacket.Pack(), c.RemoteAddr)
		setDeadlineError := conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if setDeadlineError != nil {
			return fmt.Errorf("Could not set UDP timeout: %v", setDeadlineError)
		}
		for {
			n, remoteAddr, readError := conn.ReadFromUDP(buffer)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				break
			} else if readError != nil {
				return fmt.Errorf("Error reading UDP packet: %v", readError)
			}
			p, e := ParsePacket(buffer[:n])
			if e != nil {
				continue
			}
			switch p := Packet(*p).(type) {
				case *ACK:
					if p.BlockNumber == 0 {
						s := &sender{remoteAddr, conn, reader, c.Log}
						s.Run()
						return nil
					}
				case *ERROR:
					return fmt.Errorf("Transmission error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}
	}
	return nil
}

// Method for downloading file from server
func (c Client) Get(filename string, mode string, writer *io.PipeWriter) (error) {
	addr, e := net.ResolveUDPAddr("udp", ":0")
	if e != nil {
		return e
	}
	conn, e := net.ListenUDP("udp", addr)
	if e != nil {
		return e
	}
	for i := 0; i < 3; i++ {
		rrqPacket := RRQ{filename, mode}
		conn.WriteToUDP(rrqPacket.Pack(), c.RemoteAddr)
		r := &receiver{c.RemoteAddr, conn, writer, c.Log}
		e = r.Run(false)
		if e == nil {
			return nil
		}
	}
	return fmt.Errorf("Send timeout")
}
