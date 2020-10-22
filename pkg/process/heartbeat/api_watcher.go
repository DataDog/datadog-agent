package heartbeat

import (
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	apiUnknown     = int64(0)
	apiUnreachable = int64(1)
	apiReachable   = int64(2)
)

type apiWatcher struct {
	started     int64
	apiState    int64
	gracePeriod int64
}

func newAPIWatcher(gracePeriod time.Duration) *apiWatcher {
	return &apiWatcher{gracePeriod: gracePeriod.Nanoseconds()}
}

func (w *apiWatcher) state() int64 {
	// we have determined already if the API is reachable or not
	if state := atomic.LoadInt64(&w.apiState); state != apiUnknown {
		return state
	}

	now := time.Now().UnixNano()
	atomic.CompareAndSwapInt64(&w.started, 0, now)
	then := atomic.LoadInt64(&w.started)
	if now-then < w.gracePeriod {
		return atomic.LoadInt64(&w.apiState)
	}

	// if we have finished waiting the grace period, and there were no successes
	// we can consider the API unreachable.
	swaped := atomic.CompareAndSwapInt64(&w.apiState, apiUnknown, apiUnreachable)
	if swaped {
		log.Warn("unable to flush heartbeats via API. switching over to statsd.")
	}
	return atomic.LoadInt64(&w.apiState)
}

// handler is meant to be passed to the `forwarder` instance responsible for flushing the data
func (w *apiWatcher) handler() forwarder.HTTPCompletionHandler {
	return func(_ *forwarder.HTTPTransaction, statusCode int, _ []byte, err error) {
		if err == nil && statusCode >= 200 && statusCode < 300 {
			atomic.StoreInt64(&w.apiState, apiReachable)
		}
	}
}
