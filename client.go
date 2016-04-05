package tftp

import (
	"fmt"
	"io"
	"net"
	"strconv"
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
	blksize int
	tsize   bool
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
	if c.blksize != 0 {
		s.opts = make(options)
		s.opts["blksize"] = strconv.Itoa(c.blksize)
	}
	n := packRQ(s.send, opWRQ, filename, mode, s.opts)
	addr, err := s.sendWithRetry(n)
	if err != nil {
		return nil, err // wrap error
	}
	s.block++
	s.addr = addr
	s.opts = nil
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
	if c.blksize != 0 || c.tsize {
		r.opts = make(options)
	}
	if c.blksize != 0 {
		r.opts["blksize"] = strconv.Itoa(c.blksize)
	}
	if c.tsize {
		r.opts["tsize"] = "0"
	}
	n := packRQ(r.send, opRRQ, filename, mode, r.opts)
	r.block++
	l, addr, err := r.receiveWithRetry(n)
	if err != nil {
		return nil, err
	}
	r.l = l
	r.addr = addr
	return r, nil
}
