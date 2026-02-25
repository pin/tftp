package tftp

import "testing"

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
