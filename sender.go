package tftp

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

type sender struct {
	remoteAddr *net.UDPAddr
	conn       *net.UDPConn
	reader     *io.PipeReader
	filename   string
	mode       string
	log        *log.Logger
}

var ErrSendTimeout = errors.New("send timeout")

func (s *sender) run(serverMode bool) {
	var buffer, tmp []byte
	buffer = make([]byte, BLOCK_SIZE)
	tmp = make([]byte, MAX_DATAGRAM_SIZE)
	if !serverMode {
		err := s.sendRequest(tmp)
		if err != nil {
			s.log.Printf("Error starting transmission: %v", err)
			s.reader.CloseWithError(err)
			return
		}
	}
	var blockNumber uint16
	blockNumber = 1
	for {
		c, readError := io.ReadFull(s.reader, buffer)
		// ErrUnexpectedEOF is used by ReadFull to signal an EOF in the middle
		// of the pack. It's not really an errore in our case, so we just
		// process it as a normal packet (the last one)
		if readError != nil && readError != io.ErrUnexpectedEOF {
			if readError == io.EOF {
				// exactly 0 bytes were read; this means that the file is
				// an exact multiple of the block size, and we need to send
				// a 0-sized block to signal termination
				sendError := s.sendBlock(buffer, 0, blockNumber, tmp)
				if sendError != nil && s.log != nil {
					s.log.Printf("Error sending last zero-size block: %v", sendError)
				}
			} else {
				if s.log != nil {
					s.log.Printf("Handler error: %v", readError)
				}
				errorPacket := ERROR{1, readError.Error()}
				s.conn.WriteToUDP(errorPacket.Pack(), s.remoteAddr)
				s.log.Printf("sent ERROR (code=%d): %s", 1, readError.Error())
			}
			return
		}
		sendError := s.sendBlock(buffer, c, blockNumber, tmp)
		if sendError != nil {
			if s.log != nil {
				s.log.Printf("Error sending block %d: %v", blockNumber, sendError)
			}
			s.reader.CloseWithError(sendError)
			return
		}
		blockNumber++
		// If we read a smaller block, it means we've finished reading from the pipe,
		// and thus we can exit. The reader already knows the file is finished
		// because the block was smaller.
		if c < len(buffer) {
			return
		}
	}
}

func (s *sender) sendRequest(tmp []byte) (e error) {
	for i := 0; i < 3; i++ {
		wrqPacket := WRQ{s.filename, s.mode}
		s.conn.WriteToUDP(wrqPacket.Pack(), s.remoteAddr)
		s.log.Printf("sent WRQ (filename=%s, mode=%s)", s.filename, s.mode)
		setDeadlineError := s.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if setDeadlineError != nil {
			return fmt.Errorf("Could not set UDP timeout: %v", setDeadlineError)
		}
		for {
			c, remoteAddr, readError := s.conn.ReadFromUDP(tmp)
			if networkError, ok := readError.(net.Error); ok && networkError.Timeout() {
				break
			} else if readError != nil {
				return fmt.Errorf("Error reading UDP packet: %v", readError)
			}
			packet, e := ParsePacket(tmp[:c])
			if e != nil {
				continue
			}
			switch p := packet.(type) {
			case *ACK:
				if p.BlockNumber == 0 {
					s.log.Printf("got ACK #0")
					s.remoteAddr = remoteAddr
					return nil
				}
			case *ERROR:
				return fmt.Errorf("Transmission error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}
	}
	return ErrSendTimeout
}

func (s *sender) sendBlock(b []byte, c int, n uint16, tmp []byte) (e error) {
	for i := 0; i < 3; i++ {
		setDeadlineError := s.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if setDeadlineError != nil {
			return fmt.Errorf("Could not set UDP timeout: %v", setDeadlineError)
		}
		dataPacket := DATA{n, b[:c]}
		s.conn.WriteToUDP(dataPacket.Pack(), s.remoteAddr)
		s.log.Printf("sent DATA #%d (%d bytes)", n, c)
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
			switch p := packet.(type) {
			case *ACK:
				s.log.Printf("got ACK #%d", p.BlockNumber)
				if n == p.BlockNumber {
					return nil
				}
			case *ERROR:
				return fmt.Errorf("Transmission error %d: %s", p.ErrorCode, p.ErrorMessage)
			}
		}
	}
	return ErrSendTimeout
}
