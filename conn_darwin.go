// +build darwin

package tftp

import "net"

// use IPv4 only for Darwin: conflict with syslogd UDP port
const udp = "udp4"

// special case for darwin: c.LocalAddr().String() fails with "no route to host"
func localSystem(c *net.UDPConn) string {
	_, port, err := net.SplitHostPort(c.LocalAddr().String())
	if err != nil {
		panic(err)
	}
	return "localhost:" + port
}
