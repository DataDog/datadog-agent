package heartbeat

import (
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	apiUnknown     = int64(0)
	apiUnreachable = int64(1)
	apiReachable   = int64(2)
)

// apiWatcher is responsible for detecting if the Datadog API is reachable from
// the host/container running the heartbeat.Monitor. The idea is to detect
// critical issues after the agent initialization, such as lack of network
// connectivity or a misconfigured API key.
// Given that, the allowed state transitions are simply:
// apiUnknown -> apiUnreachable
// apiUnknown -> apiReachable
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
	swapped := atomic.CompareAndSwapInt64(&w.apiState, apiUnknown, apiUnreachable)
	if swapped {
		log.Warn("unable to flush heartbeats via API. switching over to statsd.")
	}
	return atomic.LoadInt64(&w.apiState)
}

// handler is meant to be passed to the `forwarder` instance responsible for flushing the data
func (w *apiWatcher) handler() transaction.HTTPCompletionHandler {
	return func(_ *transaction.HTTPTransaction, statusCode int, _ []byte, err error) {
		if err == nil && statusCode >= 200 && statusCode < 300 {
			atomic.StoreInt64(&w.apiState, apiReachable)
		}
	}
}
