package tftp

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

// TestSinglePortLivelockRetransmission checks that a lost DATA block is still
// retransmitted in single-port mode while the client keeps re-ACKing the
// previous block.
//
// A client that misses a DATA block retransmits its last ACK every second.
// chanConnection.readFrom used to restart a full timeout on every call, so those
// stale ACKs postponed the deadline forever: the server never reached its
// retransmission path and both sides waited on each other until the client gave
// up.
func TestSinglePortLivelockRetransmission(t *testing.T) {
	const (
		blksize    = 1468
		serverWait = 3 * time.Second
	)

	payload := bytes.Repeat([]byte{0xa5}, 3*blksize+100)

	s := NewServer(func(filename string, rf io.ReaderFrom) error {
		_, err := rf.ReadFrom(bytes.NewReader(payload))
		return err
	}, nil)
	s.SetTimeout(serverWait)
	s.SetRetries(5)
	// SetBlockSize must run before EnableSinglePort: EnableSinglePort pins the
	// maximum block size to 512 while it is still unset.
	s.SetBlockSize(blksize + 4)
	s.EnableSinglePort()

	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go s.Serve(serverConn)
	defer s.Shutdown()

	conn, err := net.Dial("udp", localSystem(serverConn))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	defer conn.Close()

	send := func(packet []byte) {
		if _, err := conn.Write(packet); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	sendAck := func(block uint16) {
		ack := make([]byte, 4)
		binary.BigEndian.PutUint16(ack[0:2], opACK)
		binary.BigEndian.PutUint16(ack[2:4], block)
		send(ack)
	}

	readPacket := func(timeout time.Duration) (uint16, uint16, []byte, error) {
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			t.Fatalf("set deadline: %v", err)
		}

		buf := make([]byte, 1500)

		n, err := conn.Read(buf)
		if err != nil {
			return 0, 0, nil, err
		}

		return binary.BigEndian.Uint16(buf[0:2]), binary.BigEndian.Uint16(buf[2:4]), buf[4:n], nil
	}

	rrq := make([]byte, datagramLength)
	n := packRQ(rrq, opRRQ, "bigfile", "octet", options{"blksize": strconv.Itoa(blksize)})
	rrq = rrq[:n]

	// Serve starts asynchronously: resend the RRQ until the server answers, so
	// that a slow startup cannot make the very first datagram be lost.
	startup := time.Now().Add(10 * time.Second)

	for {
		send(rrq)

		op, _, _, err := readPacket(time.Second)
		if err != nil {
			if time.Now().After(startup) {
				t.Fatal("server never answered the RRQ: ", err)
			}

			continue
		}

		if op == opOACK {
			sendAck(0)
			break
		}
	}

	op, block, _, err := readPacket(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if op != opDATA || block != 1 {
		t.Fatalf("got op=%d block=%d, want op=%d block=1", op, block, opDATA)
	}

	sendAck(1)

	op, block, _, err = readPacket(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if op != opDATA || block != 2 {
		t.Fatalf("got op=%d block=%d, want op=%d block=2", op, block, opDATA)
	}

	// Deliberately drop DATA block 2 and re-ACK block 1 every second, like a
	// firmware that keeps retransmitting its last ACK.
	giveUp := time.Now().Add(10 * time.Second)

	var recovered []byte

	for len(recovered) == 0 {
		op, block, data, err := readPacket(time.Second)
		if err != nil {
			if time.Now().After(giveUp) {
				t.Fatal("server never retransmitted DATA block 2: single-port re-ACK livelock regression")
			}

			sendAck(1)

			continue
		}

		if op == opDATA && block == 2 {
			recovered = data
			break
		}
	}

	if len(recovered) != blksize {
		t.Fatalf("retransmitted DATA block 2 has %d bytes, want %d", len(recovered), blksize)
	}

	sendAck(2)

	op, block, _, err = readPacket(5 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if op != opDATA || block != 3 {
		t.Fatalf("got op=%d block=%d, want op=%d block=3", op, block, opDATA)
	}

	abort := make([]byte, datagramLength)
	n = packERROR(abort, 0, "test complete")
	send(abort[:n])
}
