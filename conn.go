// +build !darwin

package tftp

import "net"

const udp = "udp"

func localSystem(c *net.UDPConn) string {
	return c.LocalAddr().String()
}
