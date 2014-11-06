package tftp

import (
	"net"
	"io"
	"fmt"
	"time"
	"log"
)

type sender struct {
	remoteAddr *net.UDPAddr
	conn *net.UDPConn
	reader *io.PipeReader
	Log *log.Logger
}

func (s *sender) Run() {
	var buffer, tmp []byte
	buffer = make([]byte, 512)
	tmp = make([]byte, 512)
	var blockNumber uint16
	blockNumber = 1
	lastBlockSize := -1
	for {
		c, readError := s.reader.Read(buffer)
		if readError != nil {
			if readError == io.EOF {
				if lastBlockSize == 512 || lastBlockSize == -1 {
					sendError := s.sendBlock(buffer, 0, blockNumber, tmp)
					if sendError != nil && s.Log != nil {
						s.Log.Printf("Error sending last block: %v", sendError)
					}
				}
			} else {
				if s.Log != nil {
					s.Log.Printf("Handler error: %v", readError)
				}
				errorPacket := ERROR{1, readError.Error()}
				s.conn.WriteToUDP(errorPacket.Pack(), s.remoteAddr)
			}
			return
		}
		sendError := s.sendBlock(buffer, c, blockNumber, tmp)
		if sendError != nil {
			if s.Log != nil {
				s.Log.Printf("Error sending block %d: %v", blockNumber, sendError)
			}
			return
		}
		blockNumber++
		lastBlockSize = c
	}
	return
}

func (s *sender) sendBlock(b []byte, c int, n uint16, tmp []byte) (e error) {
	for i := 0; i < 3; i++ {
		setDeadlineError := s.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if setDeadlineError != nil {
			return fmt.Errorf("Could not set UDP timeout: %v", setDeadlineError)
		}
		dataPacket := DATA{n, b[:c]}
		s.conn.WriteToUDP(dataPacket.Pack(), s.remoteAddr)
		for {
			c, _, readError := s.conn.ReadFromUDP(tmp)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				break
			} else if readError != nil {
				return fmt.Errorf("Error reading UDP packet: %v", readError)
			}
			packet, e := ParsePacket(tmp[:c])
			if e != nil {
				continue
			}
			switch p := Packet(*packet).(type) {
				case *ACK:
					if n == p.BlockNumber {
						return nil
					}
				case *ERROR:
					return fmt.Errorf("Transmission error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}	
	}
	return fmt.Errorf("Send timeout")
}
