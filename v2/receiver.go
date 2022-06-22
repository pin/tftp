package tftp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/pin/tftp/netascii"
)

// IncomingTransfer provides methods that expose information associated with
// an incoming transfer.
type IncomingTransfer interface {
	// Size returns the size of an incoming file if the request included the
	// tsize option (see RFC2349).  To differentiate a zero-sized file transfer
	// from a request without tsize use the second boolean "ok" return value.
	Size() (n int64, ok bool)

	// RemoteAddr returns the remote peer's IP address and port.
	RemoteAddr() net.UDPAddr
}

func (r *receiver) RemoteAddr() net.UDPAddr { return *r.addr }
func (r *receiver) LocalIP() net.IP         { return r.localIP }

func (r *receiver) Size() (n int64, ok bool) {
	if r.opts != nil {
		if s, ok := r.opts["tsize"]; ok {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return 0, false
			}
			return n, true
		}
	}
	return 0, false
}

type receiver struct {
	send           []byte
	receive        []byte
	addr           *net.UDPAddr
	filename       string
	localIP        net.IP
	tid            int
	conn           connection
	block          uint16
	retry          *backoff
	timeout        time.Duration
	retries        int
	l              int
	autoTerm       bool
	dally          bool
	mode           string
	opts           options
	singlePort     bool
	maxBlockLen    int
	hook           Hook
	startTime      time.Time
	datagramsSent  int
	datagramsAcked int
}

func (r *receiver) WriteTo(w io.Writer) (n int64, err error) {
	if r.mode == "netascii" {
		w = netascii.FromWriter(w)
	}
	if r.opts != nil {
		err := r.sendOptions()
		if err != nil {
			r.abort(err)
			return 0, err
		}
	}
	binary.BigEndian.PutUint16(r.send[0:2], opACK)
	for {
		if r.l > 0 {
			l, err := w.Write(r.receive[4:r.l])
			n += int64(l)
			if err != nil {
				r.abort(err)
				return n, err
			}
			if r.l < len(r.receive) {
				if r.autoTerm {
					r.terminate()
				}
				return n, nil
			}
		}
		binary.BigEndian.PutUint16(r.send[2:4], r.block)
		r.block++ // send ACK for current block and expect next one
		ll, _, err := r.receiveWithRetry(4)
		if err != nil {
			r.abort(err)
			return n, err
		}
		r.l = ll
	}
}

func (r *receiver) sendOptions() error {
	for name, value := range r.opts {
		if name == "blksize" {
			err := r.setBlockSize(value)
			if err != nil {
				delete(r.opts, name)
				continue
			}
		} else {
			delete(r.opts, name)
		}
	}
	if len(r.opts) > 0 {
		m := packOACK(r.send, r.opts)
		r.block = 1 // expect data block number 1
		ll, _, err := r.receiveWithRetry(m)
		if err != nil {
			r.abort(err)
			return err
		}
		r.l = ll
	}
	return nil
}

func (r *receiver) setBlockSize(blksize string) error {
	n, err := strconv.Atoi(blksize)
	if err != nil {
		return err
	}
	if n < 512 {
		return fmt.Errorf("blksize too small: %d", n)
	}
	if n > 65464 {
		return fmt.Errorf("blksize too large: %d", n)
	}
	if r.maxBlockLen > 0 && n > r.maxBlockLen {
		n = r.maxBlockLen
		r.opts["blksize"] = strconv.Itoa(n)
	}
	r.receive = make([]byte, n+4)
	return nil
}

func (r *receiver) receiveWithRetry(l int) (int, *net.UDPAddr, error) {
	r.retry.reset()
	for {
		n, addr, err := r.receiveDatagram(l)
		if _, ok := err.(net.Error); ok && r.retry.count() < r.retries {
			r.retry.backoff()
			continue
		}
		return n, addr, err
	}
}

func (r *receiver) receiveDatagram(l int) (int, *net.UDPAddr, error) {
	err := r.conn.setDeadline(r.timeout)
	if err != nil {
		return 0, nil, err
	}
	err = r.conn.sendTo(r.send[:l], r.addr)
	if err != nil {
		return 0, nil, err
	}
	r.datagramsSent++
	for {
		c, addr, err := r.conn.readFrom(r.receive)
		if err != nil {
			return 0, nil, err
		}
		if !addr.IP.Equal(r.addr.IP) || (r.tid != 0 && addr.Port != r.tid) {
			continue
		}
		p, err := parsePacket(r.receive[:c])
		if err != nil {
			return 0, addr, err
		}
		r.tid = addr.Port
		switch p := p.(type) {
		case pDATA:
			if p.block() == r.block {
				r.datagramsAcked++
				return c, addr, nil
			}
		case pOACK:
			opts, err := unpackOACK(p)
			if r.block != 1 {
				continue
			}
			if err != nil {
				r.abort(err)
				return 0, addr, err
			}
			for name, value := range opts {
				if name == "blksize" {
					err := r.setBlockSize(value)
					if err != nil {
						continue
					}
				}
			}
			r.block = 0 // ACK with block number 0
			r.opts = opts
			return 0, addr, nil
		case pERROR:
			return 0, addr, fmt.Errorf("code: %d, message: %s",
				p.code(), p.message())
		}
	}
}

func (r *receiver) terminate() error {
	if r.conn == nil {
		return nil
	}
	defer func() {
		if r.hook != nil {
			r.hook.OnSuccess(r.buildTransferStats())
		}
		r.conn.close()
	}()
	binary.BigEndian.PutUint16(r.send[2:4], r.block)
	if r.dally {
		for i := 0; i < 3; i++ {
			_, _, err := r.receiveDatagram(4)
			if err != nil {
				return nil
			}
		}
		return fmt.Errorf("dallying termination failed")
	}
	err := r.conn.sendTo(r.send[:4], r.addr)
	if err != nil {
		return err
	}
	return nil
}

func (r *receiver) buildTransferStats() TransferStats {
	return TransferStats{
		RemoteAddr:     r.addr.IP,
		Filename:       r.filename,
		Tid:            r.tid,
		Mode:           r.mode,
		Opts:           r.opts,
		Duration:       time.Now().Sub(r.startTime),
		DatagramsSent:  r.datagramsSent,
		DatagramsAcked: r.datagramsAcked,
	}
}

func (r *receiver) abort(err error) error {
	if r.conn == nil {
		return nil
	}
	if r.hook != nil {
		r.hook.OnFailure(r.buildTransferStats(), err)
	}
	n := packERROR(r.send, 1, err.Error())
	err = r.conn.sendTo(r.send[:n], r.addr)
	if err != nil {
		return err
	}
	r.conn.close()
	r.conn = nil
	return nil
}
