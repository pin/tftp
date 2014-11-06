package tftp

import (
	"net"
	"fmt"
	"io"
	"time"
	"log"
)

type receiver struct {
	remoteAddr *net.UDPAddr
	conn *net.UDPConn
	writer *io.PipeWriter
	Log *log.Logger
}

func (r *receiver) Run() {
	var blockNumber uint16
	blockNumber = 1
	var buffer []byte
	buffer = make([]byte, 50000)
	
	for {
		last, e := r.receiveBlock(buffer, blockNumber)
		if e != nil {
			if r.Log != nil {
				r.Log.Printf("Error receiving block %d: %v", blockNumber, e)
			}
			r.writer.CloseWithError(e)
			return
		}
		if last {
			break
		}
		blockNumber++
	}
	r.writer.Close()
	r.terminate(buffer, blockNumber, false)
	return
}

func (r *receiver) receiveBlock(b []byte, n uint16) (last bool, e error) {
	for i := 0; i < 3; i++ {
		ackPacket := ACK{n - 1}
		r.conn.WriteToUDP(ackPacket.Pack(), r.remoteAddr)
		setDeadlineError := r.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if setDeadlineError != nil {
			return false, fmt.Errorf("Could not set UDP timeout: %v", setDeadlineError)
		}
		for {
			c, _, readError := r.conn.ReadFromUDP(b)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				break
			} else if e != nil {
				return false, fmt.Errorf("Error reading UDP packet: %v", e)
			}
			packet, e := ParsePacket(b[:c])
			if e != nil {
				continue
			}
			switch p := Packet(*packet).(type) {
				case *DATA:
					if n == p.BlockNumber {
						_, e := r.writer.Write(p.Data)
						if e == nil {
							return len(p.Data) < 512, nil
						} else {
							errorPacket := ERROR{1, e.Error()}
							r.conn.WriteToUDP(errorPacket.Pack(), r.remoteAddr)
							return false, fmt.Errorf("Handler error: %v", e)
						}
					}
				case *ERROR:
					return false, fmt.Errorf("Transmission error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}
	}
	return false, fmt.Errorf("Receive timeout")
}

func (r *receiver) terminate(b []byte, n uint16, dallying bool) (e error) {
	for i := 0; i < 3; i++ {
		ackPacket := ACK{n}
		_, e := r.conn.WriteToUDP(ackPacket.Pack(), r.remoteAddr)
		if !dallying {
			return e
		}
		setDeadlineError := r.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if setDeadlineError != nil {
			return fmt.Errorf("Could not set UDP timeout: %v", setDeadlineError)
		}
		l1:
		for {
			c, _, readError := r.conn.ReadFromUDP(b)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				return nil
			} else if e != nil {
				return fmt.Errorf("Error reading UDP packet: %v", e)
			}
			packet, e := ParsePacket(b[:c])
			if e != nil {
				continue
			}
			switch p := Packet(*packet).(type) {
				case *DATA:
					if n == p.BlockNumber {
						break l1
					}
				case *ERROR:
					fmt.Errorf("Transmission error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}	
	}
	return fmt.Errorf("Termination error")
}