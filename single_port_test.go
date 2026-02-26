package tftp

import (
	"net"
	"testing"
	"time"
)

func TestSinglePortShutdownReturns(t *testing.T) {
	s := NewServer(nil, nil)
	s.EnableSinglePort()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Serve(conn)
	}()

	// Give Serve time to enter the read loop so it blocks on ReadFrom.
	time.Sleep(100 * time.Millisecond)
	s.Shutdown()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve returned error: %v", err)
		}
	case <-time.After(1500 * time.Millisecond):
		conn.Close()
		t.Fatalf("Serve did not return after Shutdown in single-port mode")
	}
}
