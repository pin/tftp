package tftp

import (
	"net"
	"io"
	"fmt"
	"time"
)

type Sender struct {
	remoteAddr *net.UDPAddr
	conn *net.UDPConn
	reader *io.PipeReader
}

func (s *Sender) Run() {
	var buffer, tmp []byte
	buffer = make([]byte, 512)
	tmp = make([]byte, 512)
	var blockNumber uint16
	blockNumber = 1
	lastBlockSize := -1
	for {
//		fmt.Printf("waiting for data\n")
		c, readError := s.reader.Read(buffer)
		if readError != nil {
			if readError == io.EOF {
				if lastBlockSize == 512 || lastBlockSize == -1 {
					sendError := s.sendBlock(buffer, 0, blockNumber, tmp)
					if sendError != nil {
						fmt.Printf("error sending: %v\n", sendError)
					}
				}
				fmt.Printf("exit\n")
			} else {
				fmt.Printf("CLOSED WITH ERROR: %s\n", readError.Error())
				errorPacket := ERROR{1, readError.Error()}
				s.conn.WriteToUDP(errorPacket.Pack(), s.remoteAddr)
			}
			return
		}
		sendError := s.sendBlock(buffer, c, blockNumber, tmp)
		if sendError != nil {
			fmt.Printf("error sending: %v\n", sendError)
			return
		}
		blockNumber++
		lastBlockSize = c
	}
	return
}

func (s *Sender) sendBlock(b []byte, c int, n uint16, tmp []byte) (e error) {
	for i := 0; i < 3; i++ {
		setDeadlineError := s.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if setDeadlineError != nil {
			return fmt.Errorf("could not set UDP timeout: %v", setDeadlineError)
		}
		dataPacket := DATA{n, b[:c]}
		s.conn.WriteToUDP(dataPacket.Pack(), s.remoteAddr)
		for {
//			fmt.Printf("waiting for packet\n")
			c, _, readError := s.conn.ReadFromUDP(tmp)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				fmt.Printf("timeout!\n")
				break
			} else if readError != nil {
				return fmt.Errorf("error reading UDP packet: %v", readError)
			}
			packet, e := ParsePacket(tmp[:c])
			if e != nil {
				continue
			}
			switch p := Packet(*packet).(type) {
				case *ACK:
//					fmt.Printf("got ACK: %d waiting %d\n", p.BlockNumber, n)
					if n == p.BlockNumber {
//						fmt.Printf("break loop\n")
						return nil
					}
				case *ERROR:
					return fmt.Errorf("error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}	
	}
	return fmt.Errorf("send timeout")
}
