package tftp

import (
	"net"
	"testing"
)

// derived from Test900
func TestAnticipateWindow900(t *testing.T) {
	s, c := makeTestServerAnticipateWindow()
	defer s.Shutdown()
	for i := 600; i < 4000; i++ {
		c.blksize = i
		testSendReceive(t, c, 9000+int64(i))
	}
}

// TestAnticipateHookSuccess verifies that OnSuccess hook is called on transfer completion when SetAnticipate is used
func TestAnticipateHookSuccess(t *testing.T) {
	s, c := makeTestServerAnticipateWindow()
	th := newTestHook()
	s.SetHook(th)
	testSendReceive(t, c, 154242)
	s.Shutdown()
	th.Lock()
	defer th.Unlock()
	if th.transfersCompleted != 2 {
		t.Errorf("unexpected completed transfers count: %d", th.transfersCompleted)
	}
}

// derived from makeTestServer
func makeTestServerAnticipateWindow() (*Server, *Client) {
	b := &testBackend{}
	b.m = make(map[string][]byte)

	// Create server
	s := NewServer(b.handleRead, b.handleWrite)
	s.SetAnticipate(16) /* senderAnticipate window size set to 16 */

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		panic(err)
	}

	go s.Serve(conn)

	// Create client for that server
	c, err := NewClient(localSystem(conn))
	if err != nil {
		panic(err)
	}

	return s, c
}
