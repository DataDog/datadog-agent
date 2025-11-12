package stats

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestConcurrentNewStatSpanWithConfig tests the concurrent creation of StatSpan instances
// using NewStatSpanWithConfig method of SpanConcentrator to ensure thread safety.
func TestConcurrentNewStatSpanWithConfig(t *testing.T) {
	// Initialize a SpanConcentrator
	sc := NewSpanConcentrator(&SpanConcentratorConfig{
		ComputeStatsBySpanKind: true,
		BucketInterval:         int64((10 * time.Second).Nanoseconds()),
	}, time.Now())

	// Define a base StatSpanConfig to be used in span creation
	baseCfg := StatSpanConfig{
		Service:      "svc",
		Resource:     "res",
		Name:         "op",
		Type:         "custom",
		ParentID:     0,
		Start:        time.Now().UnixNano(),
		Duration:     int64(time.Millisecond * 100),
		Error:        0,
		Meta:         map[string]string{"span.kind": "client", "peer.service": "backend"},
		Metrics:      map[string]float64{"_dd.measured": 1},
		Mutex:        &sync.RWMutex{},
		PeerTags:     []string{"peer.service", "aws.s3.bucket", "db.instance"},
		HTTPMethod:   "GET",
		HTTPEndpoint: "/v1/test",
	}

	// Launch concurrent span creation to test for race conditions
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 300; j++ {
				cfg := baseCfg
				cfg.Service = fmt.Sprintf("service-%d", i)
				cfg.Resource = fmt.Sprintf("resource-%d", j%10)
                cfg.Mutex.Lock()
				cfg.Meta["peer.service"] = fmt.Sprintf("peer-%d", j%5)
                cfg.Mutex.Unlock()
				sc.NewStatSpanWithConfig(cfg)
			}
		}(i)
	}
	wg.Wait()
}
