package tftp

import (
	"fmt"
	"net"
	"time"
)

type connectionError struct {
	error
	timeout   bool
	temporary bool
}

func (t *connectionError) Timeout() bool {
	return t.timeout
}

func (t *connectionError) Temporary() bool {
	return t.temporary
}

type connection interface {
	sendTo([]byte, *net.UDPAddr) error
	readFrom([]byte) (int, *net.UDPAddr, error)
	setDeadline(time.Duration) error
	close()
}

type connConnection struct {
	conn *net.UDPConn
}

type chanConnection struct {
	s       *Server
	channel chan []byte
	addr    *net.UDPAddr
	timeout time.Duration
}

func (c *chanConnection) sendTo(data []byte, addr *net.UDPAddr) error {
	var err error
	c.s.Lock()
	conn := c.s.conn
	c.s.Unlock()
	_, err = conn.WriteTo(data, addr)
	return err
}

func (c *chanConnection) readFrom(buffer []byte) (int, *net.UDPAddr, error) {
	select {
	case data := <-c.channel:
		for i := range data {
			buffer[i] = data[i]
		}
		return len(data), c.addr, nil
	case <-time.After(c.timeout):
		return 0, nil, makeError(c.addr.String())
	}
}

func (c *chanConnection) setDeadline(deadline time.Duration) error {
	c.timeout = deadline
	return nil
}

func (c *chanConnection) close() {
	c.s.Lock()
	defer c.s.Unlock()
	close(c.channel)
	delete(c.s.handlers, c.addr.String())
}

func (c *connConnection) sendTo(data []byte, addr *net.UDPAddr) error {
	_, err := c.conn.WriteToUDP(data, addr)
	return err
}

func makeError(addr string) net.Error {
	error := connectionError{
		timeout:   true,
		temporary: true,
	}
	error.error = fmt.Errorf("Channel timeout: %v", addr)
	return &error
}

func (c *connConnection) readFrom(buffer []byte) (int, *net.UDPAddr, error) {
	return c.conn.ReadFromUDP(buffer)
}

func (c *connConnection) setDeadline(deadline time.Duration) error {
	return c.conn.SetReadDeadline(time.Now().Add(deadline))
}

func (c *connConnection) close() {
	c.conn.Close()
}
