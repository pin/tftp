package tftp

import "time"

const (
	defaultTimeout = 3 * time.Second
	defaultRetries = 5
)

type Retry interface {
	Reset()
	Count() int
	Backoff()
}

type backoff struct {
	attempt int
}

func (b *backoff) Reset() {
	b.attempt = 0
}

func (b *backoff) Count() int {
	return b.attempt
}

func (b *backoff) Backoff() {
	time.Sleep(time.Second)
	b.attempt++
}
