// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package rdnsquerierimpl

import (
	"fmt"
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
	ts := testSetup(t, overrides, false)

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
	ts := testSetup(t, overrides, false)

	// IP address in private range
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(_ string) {
			assert.FailNow(t, "Callback should not be called when rdnsquerier is not started")
		},
	)
	assert.Error(t, err)

	expectedTelemetry := map[string]float64{
		"total":                1.0,
		"private":              1.0,
		"chan_added":           0.0,
		"dropped_chan_full":    1.0,
		"dropped_rate_limiter": 0.0,
		"invalid_ip_address":   0.0,
		"lookup_err_not_found": 0.0,
		"lookup_err_timeout":   0.0,
		"lookup_err_temporary": 0.0,
		"lookup_err_other":     0.0,
		"successful":           0.0,
	}

	ts.validateExpected(t, expectedTelemetry)
}

// Test normal operations with default config when reverse DNS enrichment is enabled.
func TestNormalOperations(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	ts := testSetup(t, overrides, true)

	var wg sync.WaitGroup

	// Invalid IP address
	err := ts.rdnsQuerier.GetHostnameAsync(
		[]byte{1, 2, 3},
		func(_ string) {
			assert.FailNow(t, "Callback should not be called for invalid IP address")
		},
	)
	assert.Error(t, err)

	// IP address not in private range
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{8, 8, 8, 8},
		func(_ string) {
			assert.FailNow(t, "Callback should not be called for IP address not in private range")
		},
	)
	assert.NoError(t, err)

	// IP address in private range
	wg.Add(1)
	err = ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
	)
	assert.NoError(t, err)
	wg.Wait()

	expectedTelemetry := map[string]float64{
		"total":                3.0,
		"private":              1.0,
		"chan_added":           1.0,
		"dropped_chan_full":    0.0,
		"dropped_rate_limiter": 0.0,
		"invalid_ip_address":   1.0,
		"lookup_err_not_found": 0.0,
		"lookup_err_timeout":   0.0,
		"lookup_err_temporary": 0.0,
		"lookup_err_other":     0.0,
		"successful":           1.0,
	}

	ts.validateExpected(t, expectedTelemetry)
}

// Test that the rate limiter limits the rate as expected.  Set rate limit to 1 per second, send a bunch of requests,
// wait for N seconds, assert that no more that the expected number of requests succeed within that time period.
func TestRateLimiter(t *testing.T) {
	const numSeconds = 2

	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.workers":                         256,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":      1,
	}
	ts := testSetup(t, overrides, true)

	// IP addresses in private range
	for i := range 256 {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(hostname string) {
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
			},
		)
		assert.NoError(t, err)
	}

	time.Sleep(numSeconds * time.Second)

	expectedTelemetry := map[string]float64{
		"total":                256.0,
		"private":              256.0,
		"chan_added":           256.0,
		"dropped_chan_full":    0.0,
		"dropped_rate_limiter": 0.0,
		"invalid_ip_address":   0.0,
		"lookup_err_not_found": 0.0,
		"lookup_err_timeout":   0.0,
		"lookup_err_temporary": 0.0,
		"lookup_err_other":     0.0,
	}
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
	ts := testSetup(t, overrides, true)

	var wg sync.WaitGroup

	// IP addresses in private range
	var errCount int
	wg.Add(1) // only wait for one callback, some or all of the other requests will be dropped
	var once sync.Once
	for i := range 256 {
		err := ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(hostname string) {
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
				once.Do(func() {
					wg.Done()
				})
			},
		)
		if err != nil {
			errCount++
		}
	}
	wg.Wait()

	expectedTelemetry := map[string]float64{
		"total":                256.0,
		"private":              256.0,
		"chan_added":           float64(256 - errCount),
		"dropped_chan_full":    float64(errCount),
		"dropped_rate_limiter": 0.0,
		"invalid_ip_address":   0.0,
		"lookup_err_not_found": 0.0,
		"lookup_err_timeout":   0.0,
		"lookup_err_temporary": 0.0,
		"lookup_err_other":     0.0,
	}
	ts.validateExpected(t, expectedTelemetry)

	minimumTelemetry := map[string]float64{
		"successful": 1.0,
	}
	ts.validateMinimum(t, minimumTelemetry)
}
