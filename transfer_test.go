package tftp

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

func TestZeroLength(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		defer s.Shutdown()
		testSendReceive(t, c, 0)
	})
}

func TestSendReceiveRange(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		defer s.Shutdown()
		for i := 600; i < 1000; i++ {
			testSendReceive(t, c, 5000+int64(i))
		}
	})
}

func TestSendReceiveWithBlockSizeRange(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		defer s.Shutdown()
		for i := 600; i < 1000; i++ {
			c.SetBlockSize(i)
			testSendReceive(t, c, 5000+int64(i))
		}
	})
}

func Test1000(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
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
	})
}

func TestBlockWrapsAround(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		defer s.Shutdown()
		n := 65535 * 512
		for i := n - 2; i < n+2; i++ {
			testSendReceive(t, c, int64(i))
		}
	})
}

func TestRandomLength(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		defer s.Shutdown()
		r := rand.New(rand.NewSource(42))
		for i := 0; i < 100; i++ {
			testSendReceive(t, c, r.Int63n(100000))
		}
	})
}

func TestBigFile(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		defer s.Shutdown()
		testSendReceive(t, c, 3*1000*1000)
	})
}

func TestByOneByte(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		defer s.Shutdown()
		filename := "test-by-one-byte"
		xferMode := "octet"
		const length = 80000
		sender, err := c.Send(filename, xferMode)
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
		readTransfer, err := c.Receive(filename, xferMode)
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
	})
}

func TestMultipleClientsSimultaneously(t *testing.T) {
	forModes(t, func(t *testing.T, mode transferMode) {
		s, c := newFixture(t, mode)
		defer s.Shutdown()

		var wg sync.WaitGroup
		numClients := 10
		downloadsPerClient := 10    // Each client will download its file this many times
		fileSize := 5 * 1024 * 1024 // 5MB files
		filesData := make([][]byte, numClients)
		errChan := make(chan error, numClients*downloadsPerClient)

		t.Logf("Creating %d files of %d MB each", numClients, fileSize/1024/1024)

		// Create different data for each client.
		for i := 0; i < numClients; i++ {
			filesData[i] = make([]byte, fileSize)
			rand.Read(filesData[i])
		}

		// Setup test files by uploading them to the server.
		t.Log("Uploading test files to server...")
		for i := 0; i < numClients; i++ {
			filename := fmt.Sprintf("test%d.bin", i)
			sender, err := c.Send(filename, "octet")
			if err != nil {
				t.Fatalf("failed to start upload of test file %s: %v", filename, err)
			}
			_, err = sender.ReadFrom(bytes.NewReader(filesData[i]))
			if err != nil {
				t.Fatalf("failed to upload test file %s: %v", filename, err)
			}
			t.Logf("Uploaded %s (%d MB)", filename, fileSize/1024/1024)
		}

		t.Logf("Starting %d clients, each downloading %d times", numClients, downloadsPerClient)

		// Start multiple clients simultaneously, but each client's downloads are sequential.
		for i := 0; i < numClients; i++ {
			wg.Add(1)
			go func(clientNum int) {
				defer wg.Done()

				filename := fmt.Sprintf("test%d.bin", clientNum)

				// Each client performs its downloads sequentially.
				for j := 0; j < downloadsPerClient; j++ {
					t.Logf("Client %d started download %d", clientNum, j)

					r, err := c.Receive(filename, "octet")
					if err != nil {
						errChan <- fmt.Errorf("client %d (download %d) failed to receive: %v", clientNum, j, err)
						return
					}

					var buf bytes.Buffer
					_, err = r.WriteTo(&buf)
					if err != nil {
						errChan <- fmt.Errorf("client %d (download %d) failed to read data: %v", clientNum, j, err)
						return
					}

					if !bytes.Equal(buf.Bytes(), filesData[clientNum]) {
						errChan <- fmt.Errorf("client %d (download %d) received incorrect data", clientNum, j)
						return
					}

					t.Logf("Client %d completed download %d", clientNum, j)
				}
				t.Logf("Client %d completed all %d downloads", clientNum, downloadsPerClient)
			}(i)
		}

		wg.Wait()
		close(errChan)

		// Check for any errors from the goroutines.
		for err := range errChan {
			t.Error(err)
		}
	})
}
