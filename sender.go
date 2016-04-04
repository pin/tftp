package tftp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/pin/tftp/netascii"
)

type sender struct {
	conn    *net.UDPConn
	addr    *net.UDPAddr
	send    []byte
	receive []byte
	retry   Retry
	timeout time.Duration
	block   uint16
	mode    string
}

func (s *sender) ReadFrom(r io.Reader) (n int64, err error) {
	if s.mode == "netascii" {
		r = netascii.ToReader(r)
	}
	for {
		l, err := io.ReadFull(r, s.send[4:])
		n += int64(l)
		if err != nil && err != io.ErrUnexpectedEOF {
			if err == io.EOF {
				binary.BigEndian.PutUint16(s.send[2:4], s.block)
				_, err = s.sendWithRetry(4)
				if err != nil {
					s.abort(err)
					return n, err
				}
				return n, nil
			}
			s.abort(err)
			return n, err
		}
		binary.BigEndian.PutUint16(s.send[2:4], s.block)
		_, err = s.sendWithRetry(4 + l)
		if err != nil {
			s.abort(err)
			return n, err
		}
		if l < blockLength {
			return n, nil
		}
		s.block++
	}
}

func (s *sender) sendWithRetry(l int) (*net.UDPAddr, error) {
	s.retry.Reset()
	for {
		addr, err := s.sendDatagram(l)
		if _, ok := err.(net.Error); ok && s.retry.Count() < 3 {
			s.retry.Backoff()
			continue
		}
		return addr, err
	}
}

func (s *sender) sendDatagram(l int) (*net.UDPAddr, error) {
	err := s.conn.SetReadDeadline(time.Now().Add(s.timeout))
	if err != nil {
		return nil, err
	}
	_, err = s.conn.WriteToUDP(s.send[:l], s.addr)
	if err != nil {
		return nil, err //TODO wrap error
	}
	for {
		n, addr, err := s.conn.ReadFromUDP(s.receive)
		if err != nil {
			return nil, err
		}
		p, err := parsePacket(s.receive[:n])
		if err != nil {
			continue // just ignore
		}
		switch p := p.(type) {
		case pACK:
			block := p.block()
			if s.block == block {
				return addr, nil
			}
		case pERROR:
			return nil, fmt.Errorf("sending block %d: code=%d, error: %s",
				s.block, p.code(), p.message())
		}
	}
}

func (s *sender) abort(err error) error {
	n := packERROR(s.send, 1, err.Error())
	_, err = s.conn.WriteToUDP(s.send[:n], s.addr)
	if err != nil {
		return err //TODO wrap error
	}
	return nil
}
