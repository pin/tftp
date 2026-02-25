package tftp

import (
	"net"
	"testing"
	"time"
)

func TestZeroLengthSinglePort(t *testing.T) {
	s, c := makeTestServer(true)
	defer s.Shutdown()
	testSendReceive(t, c, 0)
}

func TestSendReceiveSinglePort(t *testing.T) {
	s, c := makeTestServer(true)
	defer s.Shutdown()
	for i := 600; i < 1000; i++ {
		testSendReceive(t, c, 5000+int64(i))
	}
}

func TestSendReceiveSinglePortWithBlockSize(t *testing.T) {
	s, c := makeTestServer(true)
	defer s.Shutdown()
	for i := 600; i < 1000; i++ {
		c.blksize = i
		testSendReceive(t, c, 5000+int64(i))
	}
}

func TestServerSendTimeoutSinglePort(t *testing.T) {
	s, c := makeTestServer(true)
	serverTimeoutSendTest(s, c, t)
}

func TestServerReceiveTimeoutSinglePort(t *testing.T) {
	s, c := makeTestServer(true)
	serverReceiveTimeoutTest(s, c, t)
}

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
