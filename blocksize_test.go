package tftp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func Test900(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	for i := 600; i < 4000; i++ {
		c.SetBlockSize(i)
		s.SetBlockSize(4600 - i)
		testSendReceive(t, c, 9000+int64(i))
	}
}

func Test1810(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	c.SetBlockSize(1810)
	testSendReceive(t, c, 9000+1810)
}

func TestNearBlockLength(t *testing.T) {
	s, c := makeTestServer(false)
	defer s.Shutdown()
	for i := 450; i < 520; i++ {
		testSendReceive(t, c, int64(i))
	}
}

func TestBlockSizeNegotiation_ClampsByPathLimit(t *testing.T) {
	got := negotiateBlockSizeForTest(t, 65432, 1472, true)
	if got != "1472" {
		t.Fatalf("unexpected negotiated blksize: got %q, want %q", got, "1472")
	}
}

func TestBlockSizeNegotiation_PreservedWhenPathAllows(t *testing.T) {
	got := negotiateBlockSizeForTest(t, 65432, 65508, true)
	if got != "65432" {
		t.Fatalf("unexpected negotiated blksize: got %q, want %q", got, "65432")
	}
}

func TestBlockSizeNegotiation_DisabledHonorsClientBlockSize(t *testing.T) {
	got := negotiateBlockSizeForTest(t, 65432, 1472, false)
	if got != "65432" {
		t.Fatalf("unexpected negotiated blksize: got %q, want %q", got, "65432")
	}
}

func negotiateBlockSizeForTest(t *testing.T, requestedBlockSize int, maxBlockLen int, smartBlock bool) string {
	t.Helper()

	clientConn, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer clientConn.Close()

	s := NewServer(func(_ string, rf io.ReaderFrom) error {
		_, err := rf.ReadFrom(bytes.NewReader([]byte{1}))
		return err
	}, nil)
	s.SetBlockSizeNegotiation(smartBlock)
	s.SetTimeout(100 * time.Millisecond)
	s.SetRetries(1)
	t.Cleanup(s.Shutdown)

	request := make([]byte, datagramLength)
	reqLen := packRQ(
		request,
		opRRQ,
		"blocksize-negotiation.bin",
		"octet",
		options{"blksize": strconv.Itoa(requestedBlockSize)},
	)
	remoteAddr := clientConn.LocalAddr().(*net.UDPAddr)
	if err := s.handlePacket(net.IPv4(127, 0, 0, 1), remoteAddr, request, reqLen, maxBlockLen, nil); err != nil {
		t.Fatalf("handle packet: %v", err)
	}

	packet := make([]byte, 70*1024)
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, serverAddr, err := clientConn.ReadFromUDP(packet)
	if err != nil {
		t.Fatalf("read OACK: %v", err)
	}
	p, err := parsePacket(packet[:n])
	if err != nil {
		t.Fatalf("parse OACK: %v", err)
	}
	oack, ok := p.(pOACK)
	if !ok {
		t.Fatalf("expected OACK, got %T", p)
	}
	opts, err := unpackOACK(oack)
	if err != nil {
		t.Fatalf("unpack OACK: %v", err)
	}
	got := opts["blksize"]

	ack := make([]byte, 4)
	binary.BigEndian.PutUint16(ack[0:2], opACK)
	if _, err := clientConn.WriteToUDP(ack, serverAddr); err != nil {
		t.Fatalf("write ACK(0): %v", err)
	}

	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, serverAddr, err = clientConn.ReadFromUDP(packet)
	if err != nil {
		t.Fatalf("read DATA(1): %v", err)
	}
	p, err = parsePacket(packet[:n])
	if err != nil {
		t.Fatalf("parse DATA(1): %v", err)
	}
	data, ok := p.(pDATA)
	if !ok {
		t.Fatalf("expected DATA, got %T", p)
	}
	if block := data.block(); block != 1 {
		t.Fatalf("unexpected DATA block: got %d, want 1", block)
	}

	binary.BigEndian.PutUint16(ack[2:4], 1)
	if _, err := clientConn.WriteToUDP(ack, serverAddr); err != nil {
		t.Fatalf("write ACK(1): %v", err)
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("transfer did not finish")
	}

	return got
}

func BenchmarkBlockSizeNegotiation(b *testing.B) {
	const (
		payloadSize   = 64 << 20 // 64 MiB
		clientBlkSize = 65432
	)

	payload := bytes.Repeat([]byte{0x5a}, payloadSize)

	bench := func(b *testing.B, serverMaxBlock int) {
		s := NewServer(func(_ string, rf io.ReaderFrom) error {
			_, err := rf.ReadFrom(bytes.NewReader(payload))
			return err
		}, nil)
		if serverMaxBlock > 0 {
			s.SetBlockSize(serverMaxBlock)
		}

		conn, err := net.ListenUDP("udp", &net.UDPAddr{})
		if err != nil {
			b.Fatalf("listen udp: %v", err)
		}
		defer conn.Close()

		go func() { _ = s.Serve(conn) }()
		b.Cleanup(s.Shutdown)

		c, err := NewClient(localSystem(conn))
		if err != nil {
			b.Fatalf("new client: %v", err)
		}
		c.SetBlockSize(clientBlkSize)

		b.ReportAllocs()
		b.SetBytes(payloadSize)
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			wt, err := c.Receive(fmt.Sprintf("blocksize-bench-%d", i), "octet")
			if err != nil {
				b.Fatalf("receive: %v", err)
			}
			n, err := wt.WriteTo(io.Discard)
			if err != nil {
				b.Fatalf("write to discard: %v", err)
			}
			if n != payloadSize {
				b.Fatalf("size mismatch: got %d want %d", n, payloadSize)
			}
		}
	}

	b.Run("LargeBlock_65432", func(b *testing.B) {
		bench(b, 65432)
	})
	b.Run("ClampedBlock_1472", func(b *testing.B) {
		bench(b, 1472)
	})
}
