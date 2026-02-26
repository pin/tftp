package tftp

import (
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"
)

func TestHookSuccess(t *testing.T) {
	s, c := makeTestServer(t, false)
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
	s, c := makeTestServer(t, false)
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
