package tftp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"testing"
)

func TestDuplicate(t *testing.T) {
	s, c := makeTestServer(t, false)
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
	s, c := makeTestServer(t, false)
	defer s.Shutdown()
	filename := "test-not-exists"
	mode := "octet"
	_, err := c.Receive(filename, mode)
	if err == nil {
		t.Fatalf("file not exists: %v", err)
	}
	t.Logf("receiving file that does not exist: %v", err)
}

func TestNoHandlers(t *testing.T) {
	s := NewServer(nil, nil)

	conn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	go s.Serve(conn)

	c, err := NewClient(localSystem(t, conn))
	if err != nil {
		t.Fatalf("new client: %v", err)
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
	c, err := NewClient(localSystem(t, conn))
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
