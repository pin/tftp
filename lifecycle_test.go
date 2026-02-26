package tftp

import (
	"bytes"
	"io"
	"math/rand"
	"net"
	"testing"
	"time"
)

func TestServerSendTimeout(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		serverTimeoutSendTest(s, c, t)
	})
}

func TestServerReceiveTimeout(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		serverReceiveTimeoutTest(s, c, t)
	})
}

func TestClientReceiveTimeout(t *testing.T) {
	s, c := makeTestServer(t, false)
	c.SetTimeout(time.Second)
	c.SetRetries(2)
	s.mu.Lock()
	s.readHandler = func(filename string, rf io.ReaderFrom) error {
		r := &slowReader{
			r:     io.LimitReader(newRandReader(rand.NewSource(42)), 80000),
			n:     3,
			delay: 8 * time.Second,
		}
		_, err := rf.ReadFrom(r)
		return err
	}
	s.mu.Unlock()
	defer s.Shutdown()
	filename := "test-client-receive-timeout"
	mode := "octet"
	readTransfer, err := c.Receive(filename, mode)
	if err != nil {
		t.Fatalf("requesting read %s: %v", filename, err)
	}
	buf := &bytes.Buffer{}
	_, err = readTransfer.WriteTo(buf)
	netErr, ok := err.(net.Error)
	if !ok {
		t.Fatalf("network error expected: %T", err)
	}
	if !netErr.Timeout() {
		t.Fatalf("timout is expected: %v", err)
	}
}

func TestClientSendTimeout(t *testing.T) {
	s, c := makeTestServer(t, false)
	c.SetTimeout(time.Second)
	c.SetRetries(2)
	s.mu.Lock()
	s.writeHandler = func(filename string, wt io.WriterTo) error {
		w := &slowWriter{
			n:     3,
			delay: 8 * time.Second,
		}
		_, err := wt.WriteTo(w)
		return err
	}
	s.mu.Unlock()
	defer s.Shutdown()
	filename := "test-client-send-timeout"
	mode := "octet"
	writeTransfer, err := c.Send(filename, mode)
	if err != nil {
		t.Fatalf("requesting write %s: %v", filename, err)
	}
	r := io.LimitReader(newRandReader(rand.NewSource(42)), 80000)
	_, err = writeTransfer.ReadFrom(r)
	netErr, ok := err.(net.Error)
	if !ok {
		t.Fatalf("network error expected: %T", err)
	}
	if !netErr.Timeout() {
		t.Fatalf("timout is expected: %v", err)
	}
}

// countingWriter signals through a channel when a certain number of bytes have been written
type countingWriter struct {
	w         io.Writer
	total     int64
	threshold int64
	signal    chan struct{}
	signaled  bool
}

func (w *countingWriter) Write(p []byte) (n int, err error) {
	n, err = w.w.Write(p)
	w.total += int64(n)
	if !w.signaled && w.total >= w.threshold {
		w.signal <- struct{}{}
		w.signaled = true
	}
	return n, err
}

// TestShutdownDuringTransfer starts a transfer, then shuts down the server mid-transfer.
// Checks that neither server nor client hang and server shuts down cleanly.
func TestShutdownDuringTransfer(t *testing.T) {
	for _, singlePort := range []bool{false, true} {
		name := "regular"
		if singlePort {
			name = "single_port"
		}
		t.Run(name, func(t *testing.T) {
			testShutdownDuringTransfer(t, singlePort)
		})
	}
}

func testShutdownDuringTransfer(t *testing.T, singlePort bool) {
	s := NewServer(func(_ string, rf io.ReaderFrom) error {
		// Simulate a slow reader: send 1MB, but slowly
		_, err := rf.ReadFrom(&slowReader{r: bytes.NewReader(make([]byte, 1<<23)), n: 1 << 20, delay: 10 * time.Millisecond})
		return err
	}, nil)

	if singlePort {
		s.EnableSinglePort()
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatal(err)
	}

	// Start a goroutine to monitor server errors
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.Serve(conn)
	}()

	c, err := NewClient(localSystem(t, conn))
	if err != nil {
		t.Fatal(err)
	}

	dl := make(chan error, 1)
	received := make(chan struct{}, 1)
	go func() {
		wt, err := c.Receive("file", "octet")
		if err != nil {
			dl <- err
			return
		}
		// Use custom writer to signal when 100KB is received
		counter := &countingWriter{
			w:         io.Discard,
			threshold: 100 * 1024, // 100KB
			signal:    received,
		}
		_, err = wt.WriteTo(counter)
		dl <- err
	}()

	// Wait for either 100KB to be received or timeout
	select {
	case <-received:
		// Received enough data, proceed with shutdown
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for data transfer to start")
	}
	s.Shutdown()

	// Server should shut down cleanly
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("server did not shut down in time")
	}

	// Client should shutdown cleanly too because server waits for transfers to finish
	select {
	case err := <-dl:
		if err != nil {
			t.Errorf("client transfer error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("client did not finish in time")
	}
}
