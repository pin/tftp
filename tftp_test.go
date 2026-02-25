package tftp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"testing"
	"testing/iotest"
	"time"
)

func TestPackUnpack(t *testing.T) {
	v := []string{"test-filename/with-subdir"}
	testOptsList := []options{
		nil,
		{
			"tsize":   "1234",
			"blksize": "22",
		},
	}
	for _, filename := range v {
		for _, mode := range []string{"octet", "netascii"} {
			for _, opts := range testOptsList {
				packUnpack(t, filename, mode, opts)
			}
		}
	}
}

func packUnpack(t *testing.T, filename, mode string, opts options) {
	b := make([]byte, datagramLength)
	for _, op := range []uint16{opRRQ, opWRQ} {
		n := packRQ(b, op, filename, mode, opts)
		f, m, o, err := unpackRQ(b[:n])
		if err != nil {
			t.Errorf("%s pack/unpack: %v", filename, err)
		}
		if f != filename {
			t.Errorf("filename mismatch (%s): '%x' vs '%x'",
				filename, f, filename)
		}
		if m != mode {
			t.Errorf("mode mismatch (%s): '%x' vs '%x'",
				mode, m, mode)
		}
		for name, value := range opts {
			v, ok := o[name]
			if !ok {
				t.Errorf("missing %s option", name)
			}
			if v != value {
				t.Errorf("option %s mismatch: '%x' vs '%x'", name, v, value)
			}
		}
	}
}

func TestZeroLength(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	testSendReceive(t, c, 0)
}

func Test900(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	for i := 600; i < 4000; i++ {
		c.SetBlockSize(i)
		s.SetBlockSize(4600 - i)
		testSendReceive(t, c, 9000+int64(i))
	}
}

func Test1000(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	for i := int64(0); i < 5000; i++ {
		filename := fmt.Sprintf("length-%d-bytes-%d", i, time.Now().UnixNano())
		rf, err := c.Send(filename, "octet")
		if err != nil {
			t.Fatalf("requesting %s write: %v", filename, err)
		}
		r := io.LimitReader(newRandReader(rand.NewSource(i)), i)
		n, err := rf.ReadFrom(r)
		if err != nil {
			t.Fatalf("sending %s: %v", filename, err)
		}
		if n != i {
			t.Errorf("%s length mismatch: %d != %d", filename, n, i)
		}
	}
}

func Test1810(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	c.SetBlockSize(1810)
	testSendReceive(t, c, 9000+1810)
}

func TestHookSuccess(t *testing.T) {
	s, c := makeTestServer(false)
	th := newTestHook()
	s.SetHook(th)
	c.SetBlockSize(1810)
	length := int64(9000)
	filename := fmt.Sprintf("length-%d-bytes-%d", length, time.Now().UnixNano())
	rf, err := c.Send(filename, "octet")
	if err != nil {
		t.Fatalf("requesting %s write: %v", filename, err)
	}
	r := io.LimitReader(newRandReader(rand.NewSource(length)), length)
	n, err := rf.ReadFrom(r)
	if err != nil {
		t.Fatalf("sending %s: %v", filename, err)
	}
	if n != length {
		t.Errorf("%s length mismatch: %d != %d", filename, n, length)
	}
	s.Shutdown()
	th.Lock()
	defer th.Unlock()
	if th.transfersCompleted != 1 {
		t.Errorf("unexpected completed transfers count: %d", th.transfersCompleted)
	}
}

func TestHookFailure(t *testing.T) {
	s, c := makeTestServer(false)
	th := newTestHook()
	s.SetHook(th)
	filename := "test-not-exists"
	mode := "octet"
	_, err := c.Receive(filename, mode)
	if err == nil {
		t.Fatalf("file not exists: %v", err)
	}
	t.Logf("receiving file that does not exist: %v", err)
	s.Shutdown()
	th.Lock()
	defer th.Unlock()
	if th.transfersFailed == 0 { // TODO: there are two failures, not one on Windows?
		t.Errorf("unexpected failed transfers count: %d", th.transfersFailed)
	}
}

func TestTSize(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	c.tsize = true
	testSendReceive(t, c, 640)
}

func TestNearBlockLength(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	for i := 450; i < 520; i++ {
		testSendReceive(t, c, int64(i))
	}
}

func TestBlockWrapsAround(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	n := 65535 * 512
	for i := n - 2; i < n+2; i++ {
		testSendReceive(t, c, int64(i))
	}
}

func TestRandomLength(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 100; i++ {
		testSendReceive(t, c, r.Int63n(100000))
	}
}

func TestBigFile(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	testSendReceive(t, c, 3*1000*1000)
}

func TestByOneByte(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	filename := "test-by-one-byte"
	mode := "octet"
	const length = 80000
	sender, err := c.Send(filename, mode)
	if err != nil {
		t.Fatalf("requesting write: %v", err)
	}
	r := iotest.OneByteReader(io.LimitReader(
		newRandReader(rand.NewSource(42)), length))
	n, err := sender.ReadFrom(r)
	if err != nil {
		t.Fatalf("send error: %v", err)
	}
	if n != length {
		t.Errorf("%s read length mismatch: %d != %d", filename, n, length)
	}
	readTransfer, err := c.Receive(filename, mode)
	if err != nil {
		t.Fatalf("requesting read %s: %v", filename, err)
	}
	buf := &bytes.Buffer{}
	n, err = readTransfer.WriteTo(buf)
	if err != nil {
		t.Fatalf("%s read error: %v", filename, err)
	}
	if n != length {
		t.Errorf("%s read length mismatch: %d != %d", filename, n, length)
	}
	bs, _ := io.ReadAll(io.LimitReader(
		newRandReader(rand.NewSource(42)), length))
	if !bytes.Equal(bs, buf.Bytes()) {
		t.Errorf("\nsent: %x\nrcvd: %x", bs, buf)
	}
}

func TestDuplicate(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	filename := "test-duplicate"
	mode := "octet"
	bs := []byte("lalala")
	sender, err := c.Send(filename, mode)
	if err != nil {
		t.Fatalf("requesting write: %v", err)
	}
	buf := bytes.NewBuffer(bs)
	_, err = sender.ReadFrom(buf)
	if err != nil {
		t.Fatalf("send error: %v", err)
	}
	_, err = c.Send(filename, mode)
	if err == nil {
		t.Fatalf("file already exists")
	}
	t.Logf("sending file that already exists: %v", err)
}

func TestNotFound(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	filename := "test-not-exists"
	mode := "octet"
	_, err := c.Receive(filename, mode)
	if err == nil {
		t.Fatalf("file not exists: %v", err)
	}
	t.Logf("receiving file that does not exist: %v", err)
}

func TestSendTsizeFromSeek(t *testing.T) {
	// create read-only server
	s := NewServer(func(filename string, rf io.ReaderFrom) error {
		b := make([]byte, 100)
		rr := newRandReader(rand.NewSource(42))
		rr.Read(b)
		// bytes.Reader implements io.Seek
		r := bytes.NewReader(b)
		_, err := rf.ReadFrom(r)
		if err != nil {
			t.Errorf("sending bytes: %v", err)
		}
		return nil
	}, nil)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatalf("listening: %v", err)
	}

	go s.Serve(conn)
	defer s.Shutdown()

	c, _ := NewClient(localSystem(conn))
	c.RequestTSize(true)
	r, _ := c.Receive("f", "octet")
	var size int64
	if it, ok := r.(IncomingTransfer); ok {
		if n, ok := it.Size(); ok {
			size = n
			fmt.Printf("Transfer size: %d\n", n)
		}
	}

	if size != 100 {
		t.Errorf("size expected: 100, got %d", size)
	}

	r.WriteTo(io.Discard)

	c.RequestTSize(false)
	r, _ = c.Receive("f", "octet")
	if it, ok := r.(IncomingTransfer); ok {
		_, ok := it.Size()
		if ok {
			t.Errorf("unexpected size received")
		}
	}

	r.WriteTo(io.Discard)
}

func TestNoHandlers(t *testing.T) {
	s := NewServer(nil, nil)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		panic(err)
	}

	go s.Serve(conn)

	c, err := NewClient(localSystem(conn))
	if err != nil {
		panic(err)
	}

	_, err = c.Send("test", "octet")
	if err == nil {
		t.Errorf("error expected")
	}

	_, err = c.Receive("test", "octet")
	if err == nil {
		t.Errorf("error expected")
	}
}

func TestServerSendTimeout(t *testing.T) {
	s, c := makeTestServer(false)
	serverTimeoutSendTest(s, c, t)
}

func TestServerReceiveTimeout(t *testing.T) {
	s, c := makeTestServer(false)
	serverReceiveTimeoutTest(s, c, t)
}

func TestClientReceiveTimeout(t *testing.T) {
	s, c := makeTestServer(false)
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
	s, c := makeTestServer(false)
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

// TestFileIOExceptions checks that errors returned by io.Reader or io.Writer used by
// the handler are handled correctly.
func TestReadWriteErrors(t *testing.T) {
	s := NewServer(
		func(_ string, rf io.ReaderFrom) error {
			_, err := rf.ReadFrom(&failingReader{}) // Read operation fails immediately.
			if err != errRead {
				t.Errorf("want: %v, got: %v", errRead, err)
			}
			// return no error from handler, client still should receive error
			return nil
		},
		func(_ string, wt io.WriterTo) error {
			_, err := wt.WriteTo(&failingWriter{}) // Write operation fails immediately.
			if err != errWrite {
				t.Errorf("want: %v, got: %v", errWrite, err)
			}
			// return no error from handler, client still should receive error
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
			errChan <- fmt.Errorf("running serve: %w", err)
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

	// Create client
	c, err := NewClient(net.JoinHostPort(localhost, port))
	if err != nil {
		t.Fatalf("creating new client: %v", err)
	}

	ot, err := c.Send("a", "octet")
	if err != nil {
		t.Errorf("start sending: %v", err)
	}

	_, err = ot.ReadFrom(io.LimitReader(
		newRandReader(rand.NewSource(42)), 42))
	if err == nil {
		t.Errorf("missing write error")
	}

	_, err = c.Receive("a", "octet")
	if err == nil {
		t.Errorf("missing read error")
	}
}

type failingReader struct{}

var errRead = errors.New("read error")

func (r *failingReader) Read(_ []byte) (int, error) {
	return 0, errRead
}

type failingWriter struct{}

var errWrite = errors.New("write error")

func (r *failingWriter) Write(_ []byte) (int, error) {
	return 0, errWrite
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

	c, err := NewClient(localSystem(conn))
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
