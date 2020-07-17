package logutil

import (
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewThrottled returns a new throttled logger. The returned logger will allow up to n calls in
// a time period of length d.
func NewThrottled(n int, d time.Duration) *ThrottledLogger {
	return &ThrottledLogger{n: uint64(n), d: d}
}

// ThrottledLogger limits the number of log calls during a time window. To create a new logger
// use NewThrottled.
type ThrottledLogger struct {
	n uint64 // number of log calls allowed during interval d
	c uint64 // number of log calls performed during an interval d; atomic value
	d time.Duration
}

type loggerFunc func(format string, params ...interface{}) error

func (tl *ThrottledLogger) log(logFunc loggerFunc, format string, params ...interface{}) {
	c := atomic.AddUint64(&tl.c, 1) - 1
	if c == 0 {
		// first call, trigger the reset
		time.AfterFunc(tl.d, func() { atomic.StoreUint64(&tl.c, 0) })
	}
	if c >= tl.n {
		if c == tl.n {
			logFunc("Too many similar messages, pausing up to %s...", tl.d)
		}
		return
	}
	logFunc(format, params...)
}

// Error logs the message at the error level.
func (tl *ThrottledLogger) Error(format string, params ...interface{}) {
	tl.log(log.Errorf, format, params...)
}

// Warn logs the message at the warning level.
func (tl *ThrottledLogger) Warn(format string, params ...interface{}) {
	tl.log(log.Warnf, format, params...)
}

// Write implements io.Writer.
func (tl *ThrottledLogger) Write(p []byte) (n int, err error) {
	tl.Error(string(p))
	return len(p), nil
}
