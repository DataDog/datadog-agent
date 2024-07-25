// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test that the rdnsQuerier starts and stops as expected.
func TestStartStop(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	ts := testSetup(t, overrides, false, nil)

	internalRDNSQuerier := ts.rdnsQuerier.(*rdnsQuerierImpl)
	assert.NotNil(t, internalRDNSQuerier)
	assert.Equal(t, false, internalRDNSQuerier.started)

	assert.NoError(t, ts.lc.Start(ts.ctx))
	assert.Equal(t, true, internalRDNSQuerier.started)

	assert.NoError(t, ts.lc.Stop(ts.ctx))
	assert.Equal(t, false, internalRDNSQuerier.started)
}

// Test that requests sent to the rdnsQuerier are handled reasonably when the rdnsQuerier is not started.
func TestNotStarted(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	ts := testSetup(t, overrides, false, nil)

	// IP address in private range
	err := ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called when rdnsquerier is not started")
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called when rdnsquerier is not started")
		},
	)
	assert.Error(t, err)

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":             1.0,
		"private":           1.0,
		"dropped_chan_full": 1.0,
		"cache_miss":        1.0,
	})
	ts.validateExpected(t, expectedTelemetry)
}

// Test with default config when reverse DNS enrichment is enabled, which includes cache enabled and rate limiter enabled.
func TestNormalOperationsDefaultConfig(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	ts := testSetup(t, overrides, true, nil)

	var wg sync.WaitGroup

	// Invalid IP address
	err := ts.rdnsQuerier.GetHostname(
		[]byte{1, 2, 3},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called for invalid IP address")
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called for invalid IP address")
		},
	)
	assert.ErrorContains(t, err, "invalid IP address")

	// IP address not in private range
	err = ts.rdnsQuerier.GetHostname(
		[]byte{8, 8, 8, 8},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called for IP address not in private range")
		},
		func(string, error) {
			assert.FailNow(t, "Async allback should not be called for IP address not in private range")
		},
	)
	assert.NoError(t, err)

	// IP address in private range - async callback should be called the first time an IP address is queried
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	// IP address in private range - cache hit should result in sync callback being called the second time an IP address is queried
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called")
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":              4.0,
		"private":            2.0,
		"chan_added":         1.0,
		"invalid_ip_address": 1.0,
		"successful":         1.0,
		"cache_hit":          1.0,
		"cache_miss":         1.0,
	})
	ts.validateExpected(t, expectedTelemetry)
}

// Test with reverse DNS enrichment enabled and cache disabled.
func TestNormalOperationsCacheDisabled(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.cache.enabled":                   false,
	}
	ts := testSetup(t, overrides, true, nil)

	var wg sync.WaitGroup

	// Invalid IP address
	err := ts.rdnsQuerier.GetHostname(
		[]byte{1, 2, 3},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called for invalid IP address")
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called for invalid IP address")
		},
	)
	assert.ErrorContains(t, err, "invalid IP address")

	// IP address not in private range
	err = ts.rdnsQuerier.GetHostname(
		[]byte{8, 8, 8, 8},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called for IP address not in private range")
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called for IP address not in private range")
		},
	)
	assert.NoError(t, err)

	// IP address in private range - with cache disabled the async callback should be called every time
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	wg.Add(1)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":              4.0,
		"private":            2.0,
		"chan_added":         2.0,
		"invalid_ip_address": 1.0,
		"successful":         2.0,
	})
	ts.validateExpected(t, expectedTelemetry)
}

// Test that the rate limiter limits the rate as expected.  Set rate limit to 1 per second, send a bunch of requests,
// wait for N seconds, assert that no more that the expected number of requests succeed within that time period.
func TestRateLimiter(t *testing.T) {
	const numSeconds = 2

	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":      1,
	}
	ts := testSetup(t, overrides, true, nil)

	// IP addresses in private range
	for i := range 20 {
		err := ts.rdnsQuerier.GetHostname(
			[]byte{192, 168, 1, byte(i)},
			func(string) {
				assert.FailNow(t, "Sync callback should not be called")
			},
			func(hostname string, err error) {
				assert.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
			},
		)
		assert.NoError(t, err)
	}

	time.Sleep(numSeconds * time.Second)

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":      20.0,
		"private":    20.0,
		"chan_added": 20.0,
		"cache_miss": 20.0,
	})
	delete(expectedTelemetry, "successful")
	ts.validateExpected(t, expectedTelemetry)

	maximumTelemetry := map[string]float64{
		"successful": float64(numSeconds + 3), // expect maximum of 1 per second, plus some buffer for test timing
	}
	ts.validateMaximum(t, maximumTelemetry)
}

// Test that when the rate limit is exceeded and the channel fills requests are dropped.
func TestChannelFullRequestsDroppedWhenRateLimited(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.workers":                         1,
		"reverse_dns_enrichment.chan_size":                       1,
		"reverse_dns_enrichment.rate_limiter.enabled":            true,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":      1,
	}
	ts := testSetup(t, overrides, true, nil)

	var wg sync.WaitGroup

	// IP addresses in private range
	var errCount int
	wg.Add(1) // only wait for one callback, most or all of the other requests will be dropped
	var once sync.Once
	for i := range 20 {
		err := ts.rdnsQuerier.GetHostname(
			[]byte{192, 168, 1, byte(i)},
			func(string) {
				assert.FailNow(t, "Sync callback should not be called")
			},
			func(hostname string, err error) {
				assert.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
				once.Do(func() {
					wg.Done()
				})
			},
		)
		if err != nil {
			assert.ErrorContains(t, err, "channel is full, dropping query for IP address")
			errCount++
		}
	}
	wg.Wait()

	assert.GreaterOrEqual(t, errCount, 1)
	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":             20.0,
		"private":           20.0,
		"chan_added":        float64(20 - errCount),
		"dropped_chan_full": float64(errCount),
		"cache_miss":        20.0,
	})
	delete(expectedTelemetry, "successful")
	ts.validateExpected(t, expectedTelemetry)

	minimumTelemetry := map[string]float64{
		"successful": 1.0,
	}
	ts.validateMinimum(t, minimumTelemetry)
}

// Test that the cache prevents multiple outstanding requests for an IP address.
func TestCacheHitInProgress(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":      1,
	}
	ts := testSetup(t, overrides, true, nil)

	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)
		err := ts.rdnsQuerier.GetHostname(
			[]byte{192, 168, 1, 100},
			func(hostname string) {
				assert.Equal(t, "fakehostname-192.168.1.100", hostname)
				wg.Done()
			},
			func(hostname string, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "fakehostname-192.168.1.100", hostname)
				wg.Done()
			},
		)
		assert.NoError(t, err)

		wg.Add(1)
		err = ts.rdnsQuerier.GetHostname(
			[]byte{192, 168, 1, 101},
			func(hostname string) {
				assert.Equal(t, "fakehostname-192.168.1.101", hostname)
				wg.Done()
			},
			func(hostname string, err error) {
				assert.NoError(t, err)
				assert.Equal(t, "fakehostname-192.168.1.101", hostname)
				wg.Done()
			},
		)
		assert.NoError(t, err)
	}
	wg.Wait()

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":      20.0,
		"private":    20.0,
		"chan_added": 2.0,
		"successful": 2.0,
		"cache_miss": 2.0,
	})
	delete(expectedTelemetry, "cache_hit") // "cache_hit" is not deterministic in this test
	delete(expectedTelemetry, "cache_hit_in_progress")
	ts.validateExpected(t, expectedTelemetry)

	minimumTelemetry := map[string]float64{
		"cache_hit_in_progress": 2.0,
	}
	ts.validateMinimum(t, minimumTelemetry)
}

// Test that lookup results are properly returned and cached in the event of retries that do not exceed cache_max_retries
func TestRetries(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled":           true,
		"network_devices.netflow.reverse_dns_enrichment_cache_max_retries": 3,
	}
	ts := testSetup(t, overrides, true,
		map[string]*fakeResults{
			"192.168.1.100": {errors: []error{
				fmt.Errorf("test error1"),
				fmt.Errorf("test error2")},
			},
			"192.168.1.101": {errors: []error{
				&net.DNSError{Err: "test timeout error", IsTimeout: true},
				&net.DNSError{Err: "test temporary error", IsTemporary: true},
				fmt.Errorf("test error")},
			},
		},
	)

	var wg sync.WaitGroup

	wg.Add(2)
	err := ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)

	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 101},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "fakehostname-192.168.1.101", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	// both were within retry limits so should be cached
	wg.Add(2)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called")
		},
	)
	assert.NoError(t, err)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 101},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.101", hostname)
			wg.Done()
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called")
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":                4.0,
		"private":              4.0,
		"chan_added":           7.0,
		"lookup_err_timeout":   1.0,
		"lookup_err_temporary": 1.0,
		"lookup_err_other":     3.0,
		"successful":           2.0,
		"cache_hit":            2.0,
		"cache_miss":           2.0,
		"cache_retry":          5.0,
	})
	ts.validateExpected(t, expectedTelemetry)
}

// Test that when retries are exceeded an empty hostname is returned and cached
func TestRetriesExceeded(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.cache.max_retries":               2,
	}
	ts := testSetup(t, overrides, true,
		map[string]*fakeResults{
			"192.168.1.100": {errors: []error{
				fmt.Errorf("test error1"),
				fmt.Errorf("test error2"),
				fmt.Errorf("test error3")},
			},
		},
	)

	var wg sync.WaitGroup

	wg.Add(1)
	err := ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.Error(t, err)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	// Because an error was returned for all available retries this IP address should now have hostname "" cached
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "", hostname)
			wg.Done()
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called")
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":                  2.0,
		"private":                2.0,
		"chan_added":             3.0,
		"lookup_err_other":       3.0,
		"cache_hit":              1.0,
		"cache_miss":             1.0,
		"cache_retry":            2.0,
		"cache_retries_exceeded": 1.0,
	})
	ts.validateExpected(t, expectedTelemetry)
}

// Test that IsNotFound error is not treated as error, but as successful resolution with hostname "", and that it is properly returned and cached
func TestIsNotFound(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	ts := testSetup(t, overrides, true,
		map[string]*fakeResults{
			"192.168.1.100": {errors: []error{
				&net.DNSError{Err: "no such host", IsNotFound: true}},
			},
		},
	)

	var wg sync.WaitGroup

	wg.Add(1)
	err := ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	wg.Add(1)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "", hostname)
			wg.Done()
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called")
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":                2.0,
		"private":              2.0,
		"chan_added":           1.0,
		"lookup_err_not_found": 1.0,
		"cache_hit":            1.0,
		"cache_miss":           1.0,
	})
	ts.validateExpected(t, expectedTelemetry)
}

// Test that cache_max_size is enforced
func TestCacheMaxSize(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.cache.max_size":                  5,
	}
	ts := testSetup(t, overrides, true, nil)

	var wg sync.WaitGroup

	// IP addresses in private range
	num := 20
	wg.Add(num)
	for i := range num {
		err := ts.rdnsQuerier.GetHostname(
			[]byte{192, 168, 1, byte(i)},
			func(hostname string) {
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
				wg.Done()
			},
			func(hostname string, err error) {
				assert.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
				wg.Done()
			},
		)
		assert.NoError(t, err)
	}
	wg.Wait()

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":                   20.0,
		"private":                 20.0,
		"chan_added":              20.0,
		"successful":              20.0,
		"cache_miss":              20.0,
		"cache_max_size_exceeded": 15.0,
	})
	ts.validateExpected(t, expectedTelemetry)

	internalRDNSQuerier := ts.rdnsQuerier.(*rdnsQuerierImpl)
	assert.NotNil(t, internalRDNSQuerier)
	internalCache := internalRDNSQuerier.cache.(*cacheImpl)
	assert.NotNil(t, internalCache)
	assert.Equal(t, 5, len(internalCache.data))
}

// Test cache expiration
func TestCacheExpiration(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.cache.entry_ttl":                 time.Duration(100) * time.Millisecond,
		"reverse_dns_enrichment.cache.clean_interval":            time.Duration(1) * time.Second,
	}
	ts := testSetup(t, overrides, true, nil)

	var wg sync.WaitGroup

	// IP addresses in private range
	num := 100
	wg.Add(num)
	for i := range num {
		err := ts.rdnsQuerier.GetHostname(
			[]byte{192, 168, 1, byte(i)},
			func(hostname string) {
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
				wg.Done()
			},
			func(hostname string, err error) {
				assert.NoError(t, err)
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
				wg.Done()
			},
		)
		assert.NoError(t, err)
	}
	wg.Wait()

	time.Sleep(2 * time.Second)

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":         float64(num),
		"private":       float64(num),
		"chan_added":    float64(num),
		"successful":    float64(num),
		"cache_miss":    float64(num),
		"cache_expired": float64(num),
	})
	ts.validateExpected(t, expectedTelemetry)

	internalRDNSQuerier := ts.rdnsQuerier.(*rdnsQuerierImpl)
	assert.NotNil(t, internalRDNSQuerier)
	internalCache := internalRDNSQuerier.cache.(*cacheImpl)
	assert.NotNil(t, internalCache)
	assert.Equal(t, 0, len(internalCache.data))
}

// Test that the cache is persisted periodically and that it is loaded and used when the agent starts.
func TestCachePersist(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.cache.persist_interval":          time.Duration(100) * time.Millisecond,
		"run_path": t.TempDir(),
	}

	ts := testSetup(t, overrides, true, nil)

	var wg sync.WaitGroup

	// async callback should be called the first time an IP address is queried
	wg.Add(1)
	err := ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	// cache hit should result in sync callback being called the second time an IP address is queried
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called")
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":      2.0,
		"private":    2.0,
		"chan_added": 1.0,
		"successful": 1.0,
		"cache_hit":  1.0,
		"cache_miss": 1.0,
	})
	ts.validateExpected(t, expectedTelemetry)

	// sleep long enough for the persist interval to trigger
	time.Sleep(2 * time.Second)

	// stop the original test setup
	assert.NoError(t, ts.lc.Stop(ts.ctx))

	// create new testsetup, validate that the IP address previously queried and cached is still cached
	ts = testSetup(t, overrides, true, nil)

	// cache hit should result in sync callback being called the first time the IP address is queried after
	// restart because persistent cache should have been loaded
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostname(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
		func(string, error) {
			assert.FailNow(t, "Async callback should not be called")
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	expectedTelemetry = ts.makeExpectedTelemetry(map[string]float64{
		"total":     1.0,
		"private":   1.0,
		"cache_hit": 1.0,
	})
	ts.validateExpected(t, expectedTelemetry)
}
