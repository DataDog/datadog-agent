package util

import (
	"time"
)

// LogLimit is a utility that can be used to avoid logging noisily
type LogLimit struct {
	firstN   int
	interval time.Duration
	next     time.Time
}

// NewLogLimit creates a LogLimit where shouldLog will return
// true the first N times it is called, and will return true once every
// interval thereafter.
func NewLogLimit(n int, interval time.Duration) *LogLimit {
	return &LogLimit{
		firstN:   n,
		interval: interval,
		next:     time.Now().Add(interval),
	}
}

// ShouldLog returns true if the caller should log
func (l *LogLimit) ShouldLog() bool {
	return l.shouldLogTime(time.Now())

}

func (l *LogLimit) shouldLogTime(now time.Time) bool {
	if l.firstN > 0 {
		l.firstN--
		return true
	}

	if now.After(l.next) {
		l.next = now.Add(l.interval)
		return true
	}

	return false
}
