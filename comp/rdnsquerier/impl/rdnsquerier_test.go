// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package rdnsquerierimpl

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	rdnsquerierdef "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test that the rdnsQuerier starts and stops as expected.
func TestStartStop(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	ts := testSetup(t, overrides, false, nil, 0)

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
	ts := testSetup(t, overrides, false, nil, 0)

	// IP address in private range
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
			assert.FailNow(t, "Sync callback should not be called when rdnsquerier is not started")
		},
		func(_ string, _ error) {
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
	ts := testSetup(t, overrides, true, nil, 0)

	var wg sync.WaitGroup

	// Invalid IP address
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{1, 2, 3},
		func(_ string) {
			assert.FailNow(t, "Sync callback should not be called for invalid IP address")
		},
		func(_ string, _ error) {
			assert.FailNow(t, "Async callback should not be called for invalid IP address")
		},
	)
	assert.ErrorContains(t, err, "invalid IP address")

	// IP address not in private range
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{8, 8, 8, 8},
		func(_ string) {
			assert.FailNow(t, "Sync callback should not be called for IP address not in private range")
		},
		func(_ string, _ error) {
			assert.FailNow(t, "Async allback should not be called for IP address not in private range")
		},
	)
	assert.NoError(t, err)

	// IP address in private range - async callback should be called the first time an IP address is queried
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
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
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
		func(_ string, _ error) {
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
	ts := testSetup(t, overrides, true, nil, 0)

	var wg sync.WaitGroup

	// Invalid IP address
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{1, 2, 3},
		func(_ string) {
			assert.FailNow(t, "Sync callback should not be called for invalid IP address")
		},
		func(_ string, _ error) {
			assert.FailNow(t, "Async callback should not be called for invalid IP address")
		},
	)
	assert.ErrorContains(t, err, "invalid IP address")

	// IP address not in private range
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{8, 8, 8, 8},
		func(_ string) {
			assert.FailNow(t, "Sync callback should not be called for IP address not in private range")
		},
		func(_ string, _ error) {
			assert.FailNow(t, "Async callback should not be called for IP address not in private range")
		},
	)
	assert.NoError(t, err)

	// IP address in private range - with cache disabled the async callback should be called every time
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
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
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
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
	ts := testSetup(t, overrides, true, nil, 0)

	// IP addresses in private range
	for i := range 20 {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(_ string) {
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

// Test that the rate limiter throttles the limit down when the error threshold is reached, and throttles the limit back up
// after a recovery interval when queries are once again successful.
func TestRateLimiterThrottled(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled":       true,
		"reverse_dns_enrichment.workers":                               2,
		"reverse_dns_enrichment.cache.max_retries":                     1,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":            50,
		"reverse_dns_enrichment.rate_limiter.limit_throttled_per_sec":  1,
		"reverse_dns_enrichment.rate_limiter.throttle_error_threshold": 4,
		"reverse_dns_enrichment.rate_limiter.recovery_intervals":       5,
		"reverse_dns_enrichment.rate_limiter.recovery_interval":        5 * time.Second,
	}
	ts := testSetup(t, overrides, true,
		map[string]*fakeResults{
			"192.168.1.30": {errors: []error{
				&net.DNSError{Err: "test timeout error", IsTimeout: true},
				&net.DNSError{Err: "test timeout error", IsTimeout: true},
			}},
			"192.168.1.31": {errors: []error{
				&net.DNSError{Err: "test timeout error", IsTimeout: true},
				&net.DNSError{Err: "test timeout error", IsTimeout: true},
			}},
		},
		0,
	)

	var wg sync.WaitGroup

	// Not throttled down yet so these should all complete quickly
	wg.Add(20)
	start := time.Now()
	for i := range 20 {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(_ string) {
				assert.FailNow(t, "Sync callback should not be called")
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
	duration := time.Since(start)
	assert.LessOrEqual(t, duration, 2*time.Second) // should all complete in < 1 sec., but allow some buffer for test timing

	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":      20.0,
		"private":    20.0,
		"chan_added": 20.0,
		"cache_miss": 20.0,
		"successful": 20.0,
	})
	ts.validateExpected(t, expectedTelemetry)
	ts.validateExpectedGauge(t, "rate_limiter_limit", 50.0)

	// These queries will get errors, exceeding throttle_error_threshold, which will cause the rate limiter to throttle down
	wg.Add(2)
	for i := 30; i < 32; i++ {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(_ string) {
				assert.FailNow(t, "Sync callback should not be called")
			},
			func(hostname string, err error) {
				assert.Error(t, err)
				assert.Equal(t, "", hostname)
				wg.Done()
			},
		)
		assert.NoError(t, err)
	}
	wg.Wait()

	expectedTelemetry = ts.makeExpectedTelemetry(map[string]float64{
		"total":                  22.0,
		"private":                22.0,
		"chan_added":             24.0,
		"lookup_err_timeout":     4.0,
		"cache_miss":             22.0,
		"successful":             20.0,
		"cache_retry":            2.0,
		"cache_retries_exceeded": 2.0,
	})
	ts.validateExpected(t, expectedTelemetry)
	ts.validateExpectedGauge(t, "rate_limiter_limit", 1.0)

	// The rate limiter is throttled now, but queries that are cache hits don't make it to the rate limiter so aren't throttled
	wg.Add(20)
	start = time.Now()
	for i := range 20 {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(hostname string) {
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
				wg.Done()
			},
			func(_ string, _ error) {
				assert.FailNow(t, "Async callback should not be called")
			},
		)
		assert.NoError(t, err)
	}
	wg.Wait()
	duration = time.Since(start)
	assert.LessOrEqual(t, duration, 2*time.Second) // should all complete in < 1 sec., but allow some buffer for test timing

	expectedTelemetry = ts.makeExpectedTelemetry(map[string]float64{
		"total":                  42.0,
		"private":                42.0,
		"chan_added":             24.0,
		"lookup_err_timeout":     4.0,
		"cache_hit":              20.0,
		"cache_miss":             22.0,
		"successful":             20.0,
		"cache_retry":            2.0,
		"cache_retries_exceeded": 2.0,
	})
	ts.validateExpected(t, expectedTelemetry)

	// Queries that are cache misses will be throttled by the rate limiter
	wg.Add(6)
	start = time.Now()
	for i := 40; i < 46; i++ {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(_ string) {
				assert.FailNow(t, "Sync callback should not be called")
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
	duration = time.Since(start)
	assert.GreaterOrEqual(t, duration, 5*time.Second)

	expectedTelemetry = ts.makeExpectedTelemetry(map[string]float64{
		"total":                  48.0,
		"private":                48.0,
		"chan_added":             30.0,
		"lookup_err_timeout":     4.0,
		"cache_hit":              20.0,
		"cache_miss":             28.0,
		"successful":             26.0,
		"cache_retry":            2.0,
		"cache_retries_exceeded": 2.0,
	})
	ts.validateExpected(t, expectedTelemetry)

	// The successful queries above should have taken longer than the recovery interval so the rate limiter should have started throttling back up
	wg.Add(10)
	start = time.Now()
	for i := 50; i < 60; i++ {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(_ string) {
				assert.FailNow(t, "Sync callback should not be called")
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
	duration = time.Since(start)
	assert.LessOrEqual(t, duration, 2*time.Second) // should all complete in < 1 sec., but allow some buffer for test timing

	expectedTelemetry = ts.makeExpectedTelemetry(map[string]float64{
		"total":                  58.0,
		"private":                58.0,
		"chan_added":             40.0,
		"lookup_err_timeout":     4.0,
		"cache_hit":              20.0,
		"cache_miss":             38.0,
		"successful":             36.0,
		"cache_retry":            2.0,
		"cache_retries_exceeded": 2.0,
	})
	ts.validateExpected(t, expectedTelemetry)
	ts.validateExpectedGauge(t, "rate_limiter_limit", 11.0)
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
	ts := testSetup(t, overrides, true, nil, 0)

	var wg sync.WaitGroup

	// IP addresses in private range
	var errCount int
	wg.Add(1) // only wait for one callback, most or all of the other requests will be dropped
	var once sync.Once
	for i := range 20 {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(_ string) {
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
	ts := testSetup(t, overrides, true, nil, 0)

	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)
		err := ts.rdnsQuerier.GetHostnameAsync(
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
		err = ts.rdnsQuerier.GetHostnameAsync(
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
		"network_devices.netflow.reverse_dns_enrichment.cache.max_retries": 3,
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
		0,
	)

	var wg sync.WaitGroup

	wg.Add(2)
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(hostname string, err error) {
			assert.NoError(t, err)
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)

	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 101},
		func(_ string) {
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
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
		func(_ string, _ error) {
			assert.FailNow(t, "Async callback should not be called")
		},
	)
	assert.NoError(t, err)
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 101},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.101", hostname)
			wg.Done()
		},
		func(_ string, _ error) {
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
		0,
	)

	var wg sync.WaitGroup

	wg.Add(1)
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
			assert.FailNow(t, "Sync callback should not be called")
		},
		func(_ string, err error) {
			assert.Error(t, err)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	// Because an error was returned for all available retries this IP address should now have hostname "" cached
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "", hostname)
			wg.Done()
		},
		func(_ string, _ error) {
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
		0,
	)

	var wg sync.WaitGroup

	wg.Add(1)
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
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
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "", hostname)
			wg.Done()
		},
		func(_ string, _ error) {
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
	ts := testSetup(t, overrides, true, nil, 0)

	var wg sync.WaitGroup

	// IP addresses in private range
	num := 20
	wg.Add(num)
	for i := range num {
		err := ts.rdnsQuerier.GetHostnameAsync(
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
	ts := testSetup(t, overrides, true, nil, 0)

	var wg sync.WaitGroup

	// IP addresses in private range
	num := 100
	wg.Add(num)
	for i := range num {
		err := ts.rdnsQuerier.GetHostnameAsync(
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
	ts.validateExpectedGauge(t, "cache_size", 0.0)

	internalRDNSQuerier := ts.rdnsQuerier.(*rdnsQuerierImpl)
	assert.NotNil(t, internalRDNSQuerier)
	internalCache := internalRDNSQuerier.cache.(*cacheImpl)
	assert.NotNil(t, internalCache)
	assert.Equal(t, 0, len(internalCache.data))
}

// Test that the cache is persisted and that it is loaded and used when the agent starts.
func TestCachePersist(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"run_path": t.TempDir(),
	}

	ts := testSetup(t, overrides, true, nil, 0)

	var wg sync.WaitGroup

	// async callback should be called the first time an IP address is queried
	wg.Add(1)
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
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
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
		func(_ string, _ error) {
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

	// stop the original test setup
	assert.NoError(t, ts.lc.Stop(ts.ctx))

	// create new testsetup, validate that the IP address previously queried and cached is still cached
	ts = testSetup(t, overrides, true, nil, 0)
	ts.validateExpectedGauge(t, "cache_size", 1.0)

	// cache hit should result in sync callback being called the first time the IP address is queried after
	// restart because persistent cache should have been loaded
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
		func(_ string, _ error) {
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

	// stop the second test setup
	assert.NoError(t, ts.lc.Stop(ts.ctx))

	// create new testsetup with shorter entryTTL, validate that the IP address previously
	// cached has new shorter expiration time
	overrides["reverse_dns_enrichment.cache.entry_ttl"] = time.Duration(100) * time.Millisecond
	ts = testSetup(t, overrides, true, nil, 0)
	ts.validateExpectedGauge(t, "cache_size", 1.0)

	time.Sleep(200 * time.Millisecond)

	// cache_hit_expired should result in async callback being called
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
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

	expectedTelemetry = ts.makeExpectedTelemetry(map[string]float64{
		"total":             1.0,
		"private":           1.0,
		"chan_added":        1.0,
		"successful":        1.0,
		"cache_hit_expired": 1.0,
		"cache_miss":        1.0,
	})
	ts.validateExpected(t, expectedTelemetry)
}

func TestGetHostname(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}

	ts := testSetup(t, overrides, true, nil, 0)
	internalRDNSQuerier := ts.rdnsQuerier.(*rdnsQuerierImpl)

	tts := map[string]struct {
		ip       string
		timeout  time.Duration
		expected string
		errMsg   string
	}{
		"invalid_ip should error": {
			ip:       "invalid_ip",
			timeout:  1 * time.Second,
			expected: "",
			errMsg:   "invalid IP address",
		},
		"public IPv4 should return empty no error": {
			ip:       "8.8.8.8",
			timeout:  1 * time.Second,
			expected: "",
			errMsg:   "",
		},
		"private IPv4 not in cache should return hostname": {
			ip:       "192.168.1.100",
			timeout:  1 * time.Second,
			expected: "fakehostname-192.168.1.100",
			errMsg:   "",
		},
		"private IPv4 in cache should return hostname": {
			ip:       "192.168.1.100",
			timeout:  1 * time.Second,
			expected: "fakehostname-192.168.1.100",
			errMsg:   "",
		},
		"public IPv6 should return empty no error": {
			ip:       "2001:4860:4860::8888",
			timeout:  1 * time.Second,
			expected: "",
			errMsg:   "",
		},
		"private IPv6 not in cache should return hostname": {
			ip:       "fd00::1",
			timeout:  1 * time.Second,
			expected: "fakehostname-fd00::1",
			errMsg:   "",
		},
		"private IPv6 in cache should return hostname": {
			ip:       "fd00::1",
			timeout:  1 * time.Second,
			expected: "fakehostname-fd00::1",
			errMsg:   "",
		},
	}

	for name, tt := range tts {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(ts.ctx, tt.timeout)
			defer cancel()
			hostname, err := internalRDNSQuerier.GetHostname(ctx, tt.ip)
			if tt.errMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, hostname)
		})
	}
}

func TestGetHostnameTimeout(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	// Set up with a delay to simulate timeout
	ts := testSetup(t, overrides, true, nil, 3*time.Second)
	internalRDNSQuerier := ts.rdnsQuerier.(*rdnsQuerierImpl)
	ctx, cancel := context.WithTimeout(ts.ctx, 1*time.Millisecond)

	// Test with a timeout exceeding the specified timeout limit
	hostname, err := internalRDNSQuerier.GetHostname(ctx, "192.168.1.100")
	assert.Equal(t, "", hostname)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout reached while resolving hostname for IP address 192.168.1.100")

	cancel()
}

// Test that when the rate limit is exceeded and the channel fills requests are dropped.
func TestGetHostnameChannelFullRequestsDroppedWhenRateLimited(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.workers":                         1,
		"reverse_dns_enrichment.chan_size":                       1,
		"reverse_dns_enrichment.rate_limiter.enabled":            true,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":      1,
	}
	ts := testSetup(t, overrides, true, nil, 1*time.Second)

	// IP addresses in private range
	var errCount atomic.Int32
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hostname, err := ts.rdnsQuerier.GetHostname(ts.ctx, fmt.Sprintf("192.168.1.%d", i))
			if err != nil {
				assert.ErrorContains(t, err, "channel is full, dropping query for IP address")
				errCount.Add(1)
			} else {
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
			}
		}(i)
	}
	wg.Wait()

	assert.GreaterOrEqual(t, errCount.Load(), int32(1))
	expectedTelemetry := ts.makeExpectedTelemetry(map[string]float64{
		"total":             20.0,
		"private":           20.0,
		"chan_added":        float64(20 - errCount.Load()),
		"dropped_chan_full": float64(errCount.Load()),
		"cache_miss":        20.0,
	})
	delete(expectedTelemetry, "successful")
	ts.validateExpected(t, expectedTelemetry)

	minimumTelemetry := map[string]float64{
		"successful": 1.0,
	}
	ts.validateMinimum(t, minimumTelemetry)
}

func TestGetHostnames(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}

	defaultTs := testSetup(t, overrides, true, nil, 100*time.Millisecond)

	tests := []struct {
		name     string
		ts       *testState
		ipAddrs  []string
		timeout  time.Duration
		expected map[string]rdnsquerierdef.ReverseDNSResult
	}{
		{
			name:    "valid IPs",
			ts:      defaultTs,
			ipAddrs: []string{"192.168.1.100", "192.168.1.101"},
			timeout: 1 * time.Second,
			expected: map[string]rdnsquerierdef.ReverseDNSResult{
				"192.168.1.100": {IP: "192.168.1.100", Hostname: "fakehostname-192.168.1.100"},
				"192.168.1.101": {IP: "192.168.1.101", Hostname: "fakehostname-192.168.1.101"},
			},
		},
		{
			name:    "invalid IP, private IPs, and public IP",
			ts:      defaultTs,
			ipAddrs: []string{"invalid_ip", "192.168.1.102", "8.8.8.8", "192.168.1.100"},
			timeout: 1 * time.Second,
			expected: map[string]rdnsquerierdef.ReverseDNSResult{
				"invalid_ip":    {IP: "invalid_ip", Err: fmt.Errorf("invalid IP address invalid_ip")},
				"192.168.1.102": {IP: "192.168.1.102", Hostname: "fakehostname-192.168.1.102"},
				"8.8.8.8":       {IP: "8.8.8.8"},
				"192.168.1.100": {IP: "192.168.1.100", Hostname: "fakehostname-192.168.1.100"},
			},
		},
		{
			name:    "invalid IP, timeout for private and public IPs",
			ts:      testSetup(t, overrides, true, nil, 10*time.Second),
			ipAddrs: []string{"192.168.1.105", "invalid", "8.8.8.8"},
			timeout: 1 * time.Second,
			expected: map[string]rdnsquerierdef.ReverseDNSResult{
				"192.168.1.105": {IP: "192.168.1.105", Err: fmt.Errorf("timeout reached while resolving hostname for IP address 192.168.1.105")},
				"invalid":       {IP: "invalid", Err: fmt.Errorf("invalid IP address invalid")},
				"8.8.8.8":       {IP: "8.8.8.8"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(tt.ts.ctx, tt.timeout)
			defer cancel()

			internalRDNSQuerier := tt.ts.rdnsQuerier.(*rdnsQuerierImpl)
			results := internalRDNSQuerier.GetHostnames(ctx, tt.ipAddrs)

			for ip, expectedResult := range tt.expected {
				result, ok := results[ip]
				require.True(t, ok, "result for IP %s not found", ip)
				assert.Equal(t, expectedResult.IP, result.IP)
				assert.Equal(t, expectedResult.Hostname, result.Hostname)
				if expectedResult.Err != nil {
					require.Error(t, result.Err)
					assert.Contains(t, result.Err.Error(), expectedResult.Err.Error())
				} else {
					assert.NoError(t, result.Err)
				}
			}
		})
	}
}
