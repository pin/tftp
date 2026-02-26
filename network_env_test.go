package tftp

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

const networkEnvVar = "TFTP_RUN_NETWORK_ENV_TESTS"

func requireNetworkEnv(t *testing.T) {
	t.Helper()
	if os.Getenv(networkEnvVar) != "1" {
		t.Skipf("set %s=1 to run environment-dependent network tests", networkEnvVar)
	}
}

// TestRequestPacketInfo checks that request packet destination address
// obtained by server using out-of-band socket info is sane.
// It creates server and tries to do transfers using different local interfaces.
// NB: Test ignores transfer errors and validates RequestPacketInfo only
// if transfer is completed successfully. So it checks that LocalIP returns
// correct result if any result is returned, but does not check if result was
// returned at all when it should.
func TestRequestPacketInfo(t *testing.T) {
	requireNetworkEnv(t)

	// localIP keeps value received from RequestPacketInfo.LocalIP
	// call inside handler.
	// If RequestPacketInfo is not supported, value is set to unspecified
	// IP address.
	var localIP net.IP
	var localIPMu sync.Mutex

	s := NewServer(
		func(_ string, rf io.ReaderFrom) error {
			localIPMu.Lock()
			if rpi, ok := rf.(RequestPacketInfo); ok {
				localIP = rpi.LocalIP()
			} else {
				localIP = net.IP{}
			}
			localIPMu.Unlock()
			_, err := rf.ReadFrom(io.LimitReader(
				newRandReader(rand.NewSource(42)), 42))
			if err != nil {
				t.Logf("sending to client: %v", err)
			}
			return nil
		},
		func(_ string, wt io.WriterTo) error {
			localIPMu.Lock()
			if rpi, ok := wt.(RequestPacketInfo); ok {
				localIP = rpi.LocalIP()
			} else {
				localIP = net.IP{}
			}
			localIPMu.Unlock()
			_, err := wt.WriteTo(io.Discard)
			if err != nil {
				t.Logf("receiving from client: %v", err)
			}
			return nil
		},
	)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatalf("listen UDP: %v", err)
	}

	_, port, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("parsing server port: %v", err)
	}

	// Start server
	errChan := make(chan error, 1)
	go func() {
		err := s.Serve(conn)
		if err != nil {
			errChan <- fmt.Errorf("serve: %w", err)
		}
	}()
	defer func() {
		s.Shutdown()
		select {
		case err := <-errChan:
			t.Errorf("server error: %v", err)
		default:
		}
	}()

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatalf("listing interface addresses: %v", err)
	}

	for _, addr := range addrs {
		ip := networkIP(addr.(*net.IPNet))
		if ip == nil {
			continue
		}

		c, err := NewClient(net.JoinHostPort(ip.String(), port))
		if err != nil {
			t.Fatalf("new client: %v", err)
		}

		// Skip re-tries to skip non-routable interfaces faster
		c.SetRetries(0)

		ot, err := c.Send("a", "octet")
		if err != nil {
			t.Logf("start sending to %v: %v", ip, err)
			continue
		}
		_, err = ot.ReadFrom(io.LimitReader(
			newRandReader(rand.NewSource(42)), 42))
		if err != nil {
			t.Logf("sending to %v: %v", ip, err)
			continue
		}

		// Check that read handler received IP that was used
		// to create the client.
		localIPMu.Lock()
		if localIP != nil && !localIP.IsUnspecified() { // Skip check if no packet info
			if !localIP.Equal(ip) {
				t.Errorf("sent to: %v, request packet: %v", ip, localIP)
			}
		} else {
			fmt.Printf("Skip %v\n", ip)
		}
		localIPMu.Unlock()

		it, err := c.Receive("a", "octet")
		if err != nil {
			t.Logf("start receiving from %v: %v", ip, err)
			continue
		}
		_, err = it.WriteTo(io.Discard)
		if err != nil {
			t.Logf("receiving from %v: %v", ip, err)
			continue
		}

		// Check that write handler received IP that was used
		// to create the client.
		localIPMu.Lock()
		if localIP != nil && !localIP.IsUnspecified() { // Skip check if no packet info
			if !localIP.Equal(ip) {
				t.Errorf("sent to: %v, request packet: %v", ip, localIP)
			}
		} else {
			fmt.Printf("Skip %v\n", ip)
		}
		localIPMu.Unlock()

		fmt.Printf("Done %v\n", ip)
	}
}

func networkIP(n *net.IPNet) net.IP {
	if ip := n.IP.To4(); ip != nil {
		return ip
	}
	if len(n.IP) == net.IPv6len {
		return n.IP
	}
	return nil
}

func TestSetLocalAddr(t *testing.T) {
	requireNetworkEnv(t)

	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("failed to get network interfaces: %v", err)
	}

	var addrs []net.IP
	for _, i := range interfaces {
		interfaceAddrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range interfaceAddrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			addrs = append(addrs, ip)
		}
	}

	for _, addrA := range addrs {
		for _, addrB := range addrs {
			t.Run(fmt.Sprintf("%s receives from %s", addrA.String(), addrB.String()), func(t *testing.T) {
				var mu sync.Mutex
				var remoteAddr net.UDPAddr
				s := NewServer(nil, func(filename string, wt io.WriterTo) error {
					_, err := wt.WriteTo(io.Discard)
					if err != nil {
						return err
					}
					mu.Lock()
					defer mu.Unlock()
					remoteAddr = wt.(IncomingTransfer).RemoteAddr()
					return nil
				})
				s.SetTimeout(2 * time.Second)

				conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: addrA, Port: 0})
				if err != nil {
					t.Fatalf("listen udp: %v", err)
				}

				go s.Serve(conn)
				defer s.Shutdown()

				c, err := NewClient(conn.LocalAddr().String())
				if err != nil {
					t.Fatalf("creating client: %v", err)
				}
				c.SetLocalAddr(addrB.String())

				wt, err := c.Send("testfile", "octet")
				if err != nil {
					return
				}
				_, _ = wt.ReadFrom(bytes.NewReader([]byte("test data for cross-interface")))

				// Compare addrB to remoteAddr in thread-safe manner
				mu.Lock()
				defer mu.Unlock()
				if remoteAddr.IP == nil {
					t.Error("remote address was not captured from server")
				} else if !remoteAddr.IP.Equal(addrB) {
					t.Errorf("remote address mismatch: expected %s, got %s", addrB.String(), remoteAddr.IP.String())
				}
			})
		}
	}
}
