package tftp

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"sync"
	"testing"
)

var (
	c *Client
	s *Server
)

func TestMain(m *testing.M) {
	addr, _ := net.ResolveUDPAddr("udp", "localhost:12312")

	log := log.New(os.Stderr, "", log.Ldate|log.Ltime)

	s = &Server{addr, handleWrite, handleRead, log}
	go s.Serve()

	c = &Client{addr, log}

	os.Exit(m.Run())
}

func TestSmallWrites(t *testing.T) {
	filename := "small-writes"
	mode := "octet"
	bs := []byte("just write a tftpHandler that writes a file into the pipe one byte at a time")
	c.Put(filename, mode, func(writer *io.PipeWriter) {
		for i := 0; i < len(bs); i++ {
			writer.Write(bs[i : i+1])
		}
		writer.Close()
	})
	buf := new(bytes.Buffer)
	c.Get(filename, mode, func(reader *io.PipeReader) {
		buf.ReadFrom(reader)
	})
	if !bytes.Equal(bs, buf.Bytes()) {
		t.Fatalf("sent: %s, received: %s", string(bs), buf.String())
	}
}

func TestPutGet(t *testing.T) {
	testPutGet(t, "f1", []byte("foobar"), "octet")
	testPutGet(t, "f2", []byte("La sonda New Horizons, a quasi due mesidal passaggio ravvicinato su Plutone, sta iniziando a inviare una dose consistente di immagini ad alta risoluzione del pianeta nano. La Nasa ha diffuso le prime foto il 10 settembre, come questa della Cthulhu Regio, ripresa il 14 luglio da una distanza di 80 mila km. Un’area più scura accanto alla chiara Sputnik Planum."), "octet")
	for i := 500; i < 520; i++ {
		testPutGet(t, fmt.Sprintf("size-%d", i), randomByteArray(i), "octet")
	}
}

func TestTimeout(t *testing.T) {
	addr, _ := net.ResolveUDPAddr("udp", "localhost:12322")

	log := log.New(os.Stderr, "", log.Ldate|log.Ltime)

	writeHandler := func(filename string, r *io.PipeReader) {
		buf := make([]byte, 64)
		for i := 0; i < 5; i++ {
			_, err := r.Read(buf)
			if err != nil {
				panic(err)
			}
		}
		// server "fail" during receive
	}

	readHandler := func(filename string, w *io.PipeWriter) {
		for i := 0; i < 5; i++ {
			_, err := w.Write(randomByteArray(64))
			if err != nil {
				panic(err)
			}
		}
		// server "fail" during send
	}

	s = &Server{addr, writeHandler, readHandler, log}
	go s.Serve()

	c = &Client{addr, log}

	var err error
	c.Put("test", "octet", func(writer *io.PipeWriter) {
		_, err = writer.Write(randomByteArray(5000))
		writer.Close()
	})
	if err != ErrSendTimeout {
		t.Fatalf("Send timeout expected, got %v", err)
	}

	buf := new(bytes.Buffer)
	c.Get("test", "octet", func(reader *io.PipeReader) {
		_, err = buf.ReadFrom(reader)
	})
	if err != ErrReceiveTimeout {
		t.Fatalf("Receive timeout expected, got %v", err)
	}
}

func randomByteArray(n int) []byte {
	bs := make([]byte, n)
	for i := 0; i < n; i++ {
		bs[i] = byte(rand.Int63() & 0xff)
	}
	return bs
}

func testPutGet(t *testing.T, filename string, bs []byte, mode string) {
	c.Put(filename, mode, func(writer *io.PipeWriter) {
		writer.Write(bs)
		writer.Close()
	})
	buf := new(bytes.Buffer)
	c.Get(filename, mode, func(reader *io.PipeReader) {
		buf.ReadFrom(reader)
	})
	if !bytes.Equal(bs, buf.Bytes()) {
		t.Fatalf("sent: %s, received: %s", string(bs), buf.String())
	}
}

var m = map[string][]byte{}
var mu sync.Mutex

func handleWrite(filename string, r *io.PipeReader) {
	mu.Lock()
	defer mu.Unlock()
	_, exists := m[filename]
	if exists {
		r.CloseWithError(fmt.Errorf("File already exists: %s", filename))
		return
	}
	buffer := &bytes.Buffer{}
	c, e := buffer.ReadFrom(r)
	if e != nil {
		fmt.Fprintf(os.Stderr, "Can't receive %s: %v\n", filename, e)
	} else {
		fmt.Fprintf(os.Stderr, "Received %s (%d bytes)\n", filename, c)
		m[filename] = buffer.Bytes()
	}
}

func handleRead(filename string, w *io.PipeWriter) {
	mu.Lock()
	defer mu.Unlock()
	b, exists := m[filename]
	if exists {
		buffer := bytes.NewBuffer(b)
		c, e := buffer.WriteTo(w)
		if e != nil {
			fmt.Fprintf(os.Stderr, "Can't send %s: %v\n", filename, e)
		} else {
			fmt.Fprintf(os.Stderr, "Sent %s (%d bytes)\n", filename, c)
		}
		w.Close()
	} else {
		w.CloseWithError(fmt.Errorf("File not found: %s", filename))
	}
}
