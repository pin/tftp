package tftp

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"testing/iotest"
	"time"
)

func TestZeroLength(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	testSendReceive(t, c, 0)
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
