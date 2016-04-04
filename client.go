package tftp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

func NewClient(addr string) (*Client, error) {
	a, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolving address %s: %v", addr, err)
	}
	return &Client{
		addr:    a,
		retry:   &backoff{},
		timeout: defaultTimeout,
	}, nil
}

func (c *Client) SetTimeout(t time.Duration) {
	c.timeout = t
}

type Client struct {
	addr    *net.UDPAddr
	retry   Retry
	timeout time.Duration
}

func (c Client) Send(filename string, mode string) (io.ReaderFrom, error) {
	conn, err := transmissionConn()
	if err != nil {
		return nil, err
	}
	s := &sender{
		send:    make([]byte, datagramLength),
		receive: make([]byte, datagramLength),
		conn:    conn,
		retry:   c.retry,
		timeout: c.timeout,
		addr:    c.addr,
		mode:    mode,
	}
	n := packRQ(s.send, opWRQ, filename, mode)
	addr, err := s.sendWithRetry(n)
	if err != nil {
		return nil, err // wrap error
	}
	s.block++
	s.addr = addr
	binary.BigEndian.PutUint16(s.send[0:2], opDATA)
	return s, nil
}

func (c Client) Receive(filename string, mode string) (io.WriterTo, error) {
	conn, err := transmissionConn()
	if err != nil {
		return nil, err
	}
	if c.timeout == 0 {
		c.timeout = defaultTimeout
	}
	r := &receiver{
		send:     make([]byte, datagramLength),
		receive:  make([]byte, datagramLength),
		conn:     conn,
		retry:    c.retry,
		timeout:  c.timeout,
		addr:     c.addr,
		autoTerm: true,
		mode:     mode,
	}
	n := packRQ(r.send, opRRQ, filename, mode)
	r.block++
	l, addr, err := r.receiveWithRetry(n)
	if err != nil {
		return nil, err
	}
	r.l = l
	r.addr = addr
	binary.BigEndian.PutUint16(r.send[0:2], opACK)
	return r, nil
}
