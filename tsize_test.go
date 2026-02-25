package tftp

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"net"
	"testing"
)

func TestTSize(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	c.RequestTSize(true)
	testSendReceive(t, c, 640)
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
