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

// Shared test fixtures/helpers only. Do not add test cases here.

type transferMode string

const (
	modeRegular    transferMode = "regular"
	modeSinglePort transferMode = "single_port"
)

func forModes(t *testing.T, fn func(t *testing.T, mode transferMode)) {
	t.Helper()
	for _, mode := range []transferMode{modeRegular, modeSinglePort} {
		mode := mode
		t.Run(string(mode), func(t *testing.T) {
			fn(t, mode)
		})
	}
}

func newFixture(t *testing.T, mode transferMode) (*Server, *Client) {
	t.Helper()
	switch mode {
	case modeRegular:
		return makeTestServer(t, false)
	case modeSinglePort:
		return makeTestServer(t, true)
	default:
		t.Fatalf("unknown transfer mode: %q", mode)
		return nil, nil
	}
}

var (
	localhostOnce sync.Once
	localhostAddr string
	localhostErr  error
)

func determineLocalhost() (string, error) {
	l, err := net.ListenTCP("tcp", nil)
	if err != nil {
		return "", fmt.Errorf("listen tcp: %w", err)
	}
	_, lport, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		l.Close()
		return "", fmt.Errorf("split host port: %w", err)
	}
	defer l.Close()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				break
			}
			conn.Close()
		}
	}()

	for _, af := range []string{"tcp6", "tcp4"} {
		conn, err := net.Dial(af, net.JoinHostPort("", lport))
		if err != nil {
			continue
		}
		host, _, splitErr := net.SplitHostPort(conn.LocalAddr().String())
		conn.Close()
		if splitErr == nil {
			return host, nil
		}
	}

	return "", fmt.Errorf("could not determine localhost address family")
}

func resolveLocalhost() (string, error) {
	localhostOnce.Do(func() {
		localhostAddr, localhostErr = determineLocalhost()
	})
	return localhostAddr, localhostErr
}

func localSystem(tb testing.TB, c *net.UDPConn) string {
	tb.Helper()
	host, err := resolveLocalhost()
	if err != nil {
		tb.Fatalf("resolve localhost: %v", err)
	}
	_, port, err := net.SplitHostPort(c.LocalAddr().String())
	if err != nil {
		tb.Fatalf("split listener address: %v", err)
	}
	return net.JoinHostPort(host, port)
}

type testHook struct {
	*sync.Mutex
	transfersCompleted int
	transfersFailed    int
}

func newTestHook() *testHook {
	return &testHook{
		Mutex: &sync.Mutex{},
	}
}

func (h *testHook) OnSuccess(result TransferStats) {
	h.Lock()
	defer h.Unlock()
	h.transfersCompleted++
}

func (h *testHook) OnFailure(result TransferStats, err error) {
	h.Lock()
	defer h.Unlock()
	h.transfersFailed++
}

func testSendReceive(t *testing.T, client *Client, length int64) {
	t.Helper()
	filename := fmt.Sprintf("length-%d-bytes", length)
	mode := "octet"
	writeTransfer, err := client.Send(filename, mode)
	if err != nil {
		t.Fatalf("requesting write %s: %v", filename, err)
	}
	r := io.LimitReader(newRandReader(rand.NewSource(42)), length)
	n, err := writeTransfer.ReadFrom(r)
	if err != nil {
		t.Fatalf("%s write error: %v", filename, err)
	}
	if n != length {
		t.Errorf("%s write length mismatch: %d != %d", filename, n, length)
	}
	readTransfer, err := client.Receive(filename, mode)
	if err != nil {
		t.Fatalf("requesting read %s: %v", filename, err)
	}
	if it, ok := readTransfer.(IncomingTransfer); ok {
		if n, ok := it.Size(); ok {
			fmt.Printf("Transfer size: %d\n", n)
			if n != length {
				t.Errorf("tsize mismatch: %d vs %d", n, length)
			}
		}
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

type testBackend struct {
	m  map[string][]byte
	mu sync.Mutex
}

func makeTestServer(t *testing.T, singlePort bool) (*Server, *Client) {
	t.Helper()
	b := &testBackend{}
	b.m = make(map[string][]byte)

	// Create server
	s := NewServer(b.handleRead, b.handleWrite)

	if singlePort {
		s.SetBlockSize(2000)
		s.EnableSinglePort()
	}

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	go s.Serve(conn)

	// Create client for that server
	c, err := NewClient(localSystem(t, conn))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	return s, c
}

func (b *testBackend) handleWrite(filename string, wt io.WriterTo) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.m[filename]
	if ok {
		fmt.Fprintf(os.Stderr, "File %s already exists\n", filename)
		return fmt.Errorf("file already exists")
	}
	if t, ok := wt.(IncomingTransfer); ok {
		if n, ok := t.Size(); ok {
			fmt.Printf("Transfer size: %d\n", n)
		}
	}
	buf := &bytes.Buffer{}
	_, err := wt.WriteTo(buf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't receive %s: %v\n", filename, err)
		return err
	}
	b.m[filename] = buf.Bytes()
	return nil
}

func (b *testBackend) handleRead(filename string, rf io.ReaderFrom) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	bs, ok := b.m[filename]
	if !ok {
		fmt.Fprintf(os.Stderr, "File %s not found\n", filename)
		return fmt.Errorf("file not found")
	}
	if t, ok := rf.(OutgoingTransfer); ok {
		t.SetSize(int64(len(bs)))
	}
	_, err := rf.ReadFrom(bytes.NewBuffer(bs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't send %s: %v\n", filename, err)
		return err
	}
	return nil
}

type randReader struct {
	src  rand.Source
	next int64
	i    int8
}

func newRandReader(src rand.Source) io.Reader {
	r := &randReader{
		src:  src,
		next: src.Int63(),
	}
	return r
}

func (r *randReader) Read(p []byte) (n int, err error) {
	next, i := r.next, r.i
	for n = 0; n < len(p); n++ {
		if i == 7 {
			next, i = r.src.Int63(), 0
		}
		p[n] = byte(next)
		next >>= 8
		i++
	}
	r.next, r.i = next, i
	return
}

func serverTimeoutSendTest(s *Server, c *Client, t *testing.T) {
	t.Helper()
	s.SetTimeout(time.Second)
	s.SetRetries(2)
	sec := make(chan error, 1)
	s.mu.Lock()
	s.readHandler = func(filename string, rf io.ReaderFrom) error {
		r := io.LimitReader(newRandReader(rand.NewSource(42)), 80000)
		_, err := rf.ReadFrom(r)
		sec <- err
		return err
	}
	s.mu.Unlock()
	defer s.Shutdown()
	filename := "test-server-send-timeout"
	mode := "octet"
	readTransfer, err := c.Receive(filename, mode)
	if err != nil {
		t.Fatalf("requesting read %s: %v", filename, err)
	}
	w := &slowWriter{
		n:     3,
		delay: 8 * time.Second,
	}
	_, _ = readTransfer.WriteTo(w)
	servErr := <-sec
	netErr, ok := servErr.(net.Error)
	if !ok {
		t.Fatalf("network error expected: %T", servErr)
	}
	if !netErr.Timeout() {
		t.Fatalf("timout is expected: %v", servErr)
	}
}

func serverReceiveTimeoutTest(s *Server, c *Client, t *testing.T) {
	t.Helper()
	s.SetTimeout(time.Second)
	s.SetRetries(2)
	sec := make(chan error, 1)
	s.mu.Lock()
	s.writeHandler = func(filename string, wt io.WriterTo) error {
		buf := &bytes.Buffer{}
		_, err := wt.WriteTo(buf)
		sec <- err
		return err
	}
	s.mu.Unlock()
	defer s.Shutdown()
	filename := "test-server-receive-timeout"
	mode := "octet"
	writeTransfer, err := c.Send(filename, mode)
	if err != nil {
		t.Fatalf("requesting write %s: %v", filename, err)
	}
	r := &slowReader{
		r:     io.LimitReader(newRandReader(rand.NewSource(42)), 80000),
		n:     3,
		delay: 8 * time.Second,
	}
	_, _ = writeTransfer.ReadFrom(r)
	servErr := <-sec
	netErr, ok := servErr.(net.Error)
	if !ok {
		t.Fatalf("network error expected: %T", servErr)
	}
	if !netErr.Timeout() {
		t.Fatalf("timout is expected: %v", servErr)
	}
}

type slowReader struct {
	r     io.Reader
	n     int64
	delay time.Duration
}

func (r *slowReader) Read(p []byte) (n int, err error) {
	if r.n > 0 {
		r.n--
		return r.r.Read(p)
	}
	time.Sleep(r.delay)
	return r.r.Read(p)
}

type slowWriter struct {
	n     int64
	delay time.Duration
}

func (r *slowWriter) Write(p []byte) (n int, err error) {
	if r.n > 0 {
		r.n--
		return len(p), nil
	}
	time.Sleep(r.delay)
	return len(p), nil
}
