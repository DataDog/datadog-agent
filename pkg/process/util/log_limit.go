package util

import (
	"sync/atomic"
	"time"
)

// LogLimit is a utility that can be used to avoid logging noisily
type LogLimit struct {
	// n is the times remaining that the LogLimit will return true for ShouldLog.
	// we repeatedly add 1 to it.
	n int32

	// reset will be channel that belongs to the ticket
	ticker *time.Ticker
	exit   chan struct{}
}

// NewLogLimit creates a LogLimit where shouldLog will return
// true the first N times it is called, and will return true once every
// interval thereafter.
func NewLogLimit(n int, interval time.Duration) *LogLimit {
	l := &LogLimit{
		n: int32(n),

		// exit and ticker must be different channels
		// becaues Stopping a ticker will not close the ticker channel,
		// and we will otherwise leak memory
		ticker: time.NewTicker(interval),
		exit:   make(chan struct{}),
	}

	go l.resetLoop()
	return l
}

// ShouldLog returns true if the caller should log
func (l *LogLimit) ShouldLog() bool {
	if l.n > 0 {
		l.n--
		return true
	}

	return false
}

// Close will stop the underlying ticker
func (l *LogLimit) Close() {
	l.ticker.Stop()
	close(l.exit)
}

func (l *LogLimit) resetLoop() {
	for {
		select {
		case <-l.ticker.C:
			l.resetCounter()
		case <-l.exit:
			return
		}
	}
}

func (l *LogLimit) resetCounter() {
	// c.n == 0, it means we have gotten through the first few logs, and after ticker.T we should
	// do another log
	atomic.CompareAndSwapInt32(&l.n, 0, 1)
}
