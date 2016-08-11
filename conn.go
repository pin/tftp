// +build !darwin

package tftp

import "net"
import "fmt"
import "strconv"

const udp = "udp"

var localhost string = determineLocalhost()

func determineLocalhost() string {
	l, err := net.ListenTCP("tcp", nil)
	if err != nil {
		panic(fmt.Sprintf("ListenTCP error: %s", err))
	}
	_, lport, _ := net.SplitHostPort(l.Addr().String())
	defer l.Close()

	lo := make(chan string)

	go func() {
		conn, err := l.Accept()
		if err != nil {
			panic(fmt.Sprintf("Accept error: %s", err))
		}
		defer conn.Close()
	}()

	go func() {
		port, _ := strconv.Atoi(lport)
		conn, err := net.DialTCP("tcp6", &net.TCPAddr{}, &net.TCPAddr{Port: port})
		if err == nil {
			conn.Close()
			lo <- "::1"
			return
		} else {
			conn, err = net.DialTCP("tcp4", &net.TCPAddr{}, &net.TCPAddr{Port: port})
			if err == nil {
				conn.Close()
				lo <- "127.0.0.1"
				return
			}
		}

		panic("could not determine address family")
	}()

	return <-lo
}

func localSystem(c *net.UDPConn) string {
	_, port, _ := net.SplitHostPort(c.LocalAddr().String())
	return net.JoinHostPort(localhost, port)
}
