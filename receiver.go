package tftp

import (
	"net"
	"fmt"
	"io"
	"time"
)

type Receiver struct {
	remoteAddr *net.UDPAddr
	conn *net.UDPConn
	writer *io.PipeWriter
}

func (r *Receiver) Run() {
	var blockNumber uint16
	blockNumber = 1
	var buffer []byte
	buffer = make([]byte, 50000)
	
	for {
		last, e := r.receiveBlock(buffer, blockNumber)
		if e != nil {
			fmt.Printf("error receiving file: %v", e)
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
	fmt.Printf("done!")
	return
}

func (r *Receiver) receiveBlock(b []byte, n uint16) (last bool, e error) {
	for i := 0; i < 3; i++ {
		ackPacket := ACK{n - 1}
		r.conn.WriteToUDP(ackPacket.Pack(), r.remoteAddr)
//		fmt.Printf("ACK %d sent\n", n - 1)
		setDeadlineError := r.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if setDeadlineError != nil {
			return false, fmt.Errorf("could not set UDP timeout: %v", setDeadlineError)
		}
		for {
			c, _, readError := r.conn.ReadFromUDP(b)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				fmt.Printf("timeout!\n")
				break
			} else if e != nil {
				return false, fmt.Errorf("error reading UDP packet: %v", e)
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
							return false, fmt.Errorf("error writing data: %v", e)
						}
					}
				case *ERROR:
					return false, fmt.Errorf("transmission error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}
	}
	return false, fmt.Errorf("receive timeout")
}

func (r *Receiver) terminate(b []byte, n uint16, dallying bool) (e error) {
	for i := 0; i < 3; i++ {
		ackPacket := ACK{n}
		_, e := r.conn.WriteToUDP(ackPacket.Pack(), r.remoteAddr)
		fmt.Printf("last ACK %d\n", n)
		if !dallying {
			return e
		}
		setDeadlineError := r.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		if setDeadlineError != nil {
			return fmt.Errorf("could not set UDP timeout: %v", setDeadlineError)
		}
		l1:
		for {
			c, _, readError := r.conn.ReadFromUDP(b)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				fmt.Printf("good timeout\n")
				return nil
			} else if e != nil {
				return fmt.Errorf("error reading UDP packet: %v", e)
			}
			packet, e := ParsePacket(b[:c])
			if e != nil {
				continue
			}
			switch p := Packet(*packet).(type) {
				case *DATA:
					fmt.Printf("got DATA %d\n", p.BlockNumber)
					if n == p.BlockNumber {
						break l1
					}
				case *ERROR:
					fmt.Errorf("transmission error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}	
	}
	return fmt.Errorf("termination error")
}