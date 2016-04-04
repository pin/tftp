package tftp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/pin/tftp/netascii"
)

type receiver struct {
	send     []byte
	receive  []byte
	addr     *net.UDPAddr
	conn     *net.UDPConn
	block    uint16
	retry    Retry
	timeout  time.Duration
	l        int
	autoTerm bool
	dally    bool
	mode     string
}

func (r *receiver) WriteTo(w io.Writer) (n int64, err error) {
	if r.mode == "netascii" {
		w = netascii.FromWriter(w)
	}
	for {
		if r.l > 0 {
			l, err := w.Write(r.receive[4:r.l])
			n += int64(l)
			if err != nil {
				r.abort(err)
				return n, err
			}
			if r.l-4 < blockLength {
				if r.autoTerm {
					r.terminate()
				}
				return n, nil
			}
		}
		binary.BigEndian.PutUint16(r.send[2:4], r.block)
		r.block++
		ll, _, err := r.receiveWithRetry(4)
		if err != nil {
			r.abort(err)
			return n, err
		}
		r.l = ll
	}
}

func (s *receiver) receiveWithRetry(l int) (int, *net.UDPAddr, error) {
	s.retry.Reset()
	for {
		n, addr, err := s.receiveDatagram(l)
		if _, ok := err.(net.Error); ok && s.retry.Count() < 3 {
			s.retry.Backoff()
			continue
		}
		return n, addr, err
	}
}

func (r *receiver) receiveDatagram(l int) (int, *net.UDPAddr, error) {
	err := r.conn.SetReadDeadline(time.Now().Add(r.timeout))
	if err != nil {
		return 0, nil, err
	}
	// TODO: check the case when we constantly get something bad (incorect block number and loop instead of failing.
	_, err = r.conn.WriteToUDP(r.send[:l], r.addr)
	if err != nil {
		return 0, nil, err //TODO wrap error
	}
	for {
		c, addr, err := r.conn.ReadFromUDP(r.receive)
		if err != nil {
			return 0, nil, err
		}
		// TODO: compare addr here?
		p, err := parsePacket(r.receive[:c])
		if err != nil {
			return 0, addr, err
		}
		switch p := p.(type) {
		case pDATA:
			if p.block() == r.block {
				return c, addr, nil
			}
		case pERROR:
			return 0, addr, fmt.Errorf("code: %d, message: %s",
				p.code(), p.message())
		}
	}
}

func (r *receiver) terminate() error {
	binary.BigEndian.PutUint16(r.send[2:4], r.block)
	if r.dally {
		for i := 0; i < 3; i++ {
			_, _, err := r.receiveDatagram(4)
			if err != nil {
				return nil
			}
		}
		return fmt.Errorf("dallying termination failed")
	} else {
		_, err := r.conn.WriteToUDP(r.send[:4], r.addr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *receiver) abort(err error) error {
	n := packERROR(r.send, 1, err.Error())
	_, err = r.conn.WriteToUDP(r.send[:n], r.addr)
	if err != nil {
		return err
	}
	return nil
}
