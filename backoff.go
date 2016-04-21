package tftp

import (
	"math/rand"
	"time"
)

const (
	defaultTimeout = 5 * time.Second
	defaultRetries = 5
)

type backoff struct {
	attempt int
}

func (b *backoff) reset() {
	b.attempt = 0
}

func (b *backoff) count() int {
	return b.attempt
}

func (b *backoff) backoff() {
	time.Sleep(time.Duration(rand.Int63n(int64(time.Second))))
	b.attempt++
}
