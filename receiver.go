package tftp

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

type receiver struct {
	remoteAddr *net.UDPAddr
	conn       *net.UDPConn
	writer     *io.PipeWriter
	filename   string
	mode       string
	log        *log.Logger
}

var ErrReceiveTimeout = errors.New("receive timeout")

func (r *receiver) run(serverMode bool) error {
	var blockNumber uint16
	blockNumber = 1
	var buffer []byte
	buffer = make([]byte, MAX_DATAGRAM_SIZE)
	firstBlock := true
	for {
		last, e := r.receiveBlock(buffer, blockNumber, firstBlock && !serverMode)
		if e != nil {
			if r.log != nil {
				r.log.Printf("Error receiving block %d: %v", blockNumber, e)
			}
			r.writer.CloseWithError(e)
			return e
		}
		firstBlock = false
		if last {
			break
		}
		blockNumber++
	}
	r.writer.Close()
	r.terminate(buffer, blockNumber, false)
	return nil
}

func (r *receiver) receiveBlock(b []byte, n uint16, firstBlockOnClient bool) (last bool, e error) {
	for i := 0; i < 3; i++ {
		if firstBlockOnClient {
			rrqPacket := RRQ{r.filename, r.mode}
			r.conn.WriteToUDP(rrqPacket.Pack(), r.remoteAddr)
			r.log.Printf("sent RRQ (filename=%s, mode=%s)", r.filename, r.mode)
		} else {
			ackPacket := ACK{n - 1}
			r.conn.WriteToUDP(ackPacket.Pack(), r.remoteAddr)
			r.log.Printf("sent ACK #%d", n-1)
		}
		setDeadlineError := r.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if setDeadlineError != nil {
			return false, fmt.Errorf("Could not set UDP timeout: %v", setDeadlineError)
		}
		for {
			c, remoteAddr, readError := r.conn.ReadFromUDP(b)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				break
			} else if e != nil {
				return false, fmt.Errorf("Error reading UDP packet: %v", e)
			}
			packet, e := ParsePacket(b[:c])
			if e != nil {
				continue
			}
			switch p := packet.(type) {
			case *DATA:
				r.log.Printf("got DATA #%d (%d bytes)", p.BlockNumber, len(p.Data))
				if n == p.BlockNumber {
					if firstBlockOnClient {
						r.remoteAddr = remoteAddr
					}
					_, e := r.writer.Write(p.Data)
					if e == nil {
						return len(p.Data) < BLOCK_SIZE, nil
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
	return false, ErrReceiveTimeout
}

func (r *receiver) terminate(b []byte, n uint16, dallying bool) (e error) {
	for i := 0; i < 3; i++ {
		ackPacket := ACK{n}
		_, e := r.conn.WriteToUDP(ackPacket.Pack(), r.remoteAddr)
		r.log.Printf("sent ACK #%d", n)
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
			switch p := packet.(type) {
			case *DATA:
				r.log.Printf("got DATA #%d (%d bytes)", p.BlockNumber, len(p.Data))
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
