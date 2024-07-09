// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package rdnsquerierimpl

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert" //JMW require for some?
)

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

func TestNotStarted(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	ts := testSetup(t, overrides, false)

	// IP address in private range
	ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.FailNow(t, "Callback should not be called when rdnsquerier is not started")
		},
	)

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

func TestRDNSQuerierJMW(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	ts := testSetup(t, overrides, true)

	var wg sync.WaitGroup

	// Invalid IP address
	ts.rdnsQuerier.GetHostnameAsync(
		[]byte{1, 2, 3},
		func(hostname string) {
			assert.FailNow(t, "Callback should not be called for invalid IP address")
		},
	)

	// IP address not in private range
	ts.rdnsQuerier.GetHostnameAsync(
		[]byte{8, 8, 8, 8},
		func(hostname string) {
			assert.FailNow(t, "Callback should not be called for IP address not in private range")
		},
	)

	// IP address in private range
	wg.Add(1)
	ts.rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.Equal(t, "fakehostname-192.168.1.100", hostname)
			wg.Done()
		},
	)
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

// JMWNEW add test to validate rate limiter and channel that 1) rate is limited and 2) once rate limit is exceeded, channel gets full and requests are dropped
func TestRDNSQuerierJMW2(t *testing.T) {
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
	wg.Add(1) // only wait for one callback, some or all of the other requests will be dropped
	var once sync.Once
	for i := range 256 {
		ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(hostname string) {
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
				once.Do(func() {
					wg.Done()
				})
			},
		)
	}
	wg.Wait()

	expectedTelemetry := map[string]float64{
		"total":                256.0,
		"private":              256.0,
		"dropped_rate_limiter": 0.0,
		"invalid_ip_address":   0.0,
		"lookup_err_not_found": 0.0,
		"lookup_err_timeout":   0.0,
		"lookup_err_temporary": 0.0,
		"lookup_err_other":     0.0,
	}
	ts.validateExpected(t, expectedTelemetry)

	minimumTelemetry := map[string]float64{
		"chan_added":        1.0,
		"dropped_chan_full": 1.0,
		"successful":        1.0,
	}
	ts.validateMinimum(t, minimumTelemetry)
}

// JMWNEW add test for rate limiter - set to 1 per second, fill channel with 256 requests, run for 5 seconds, assert <= 5 requests are successful within that time
func TestRDNSQuerierJMW3(t *testing.T) {
	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.workers":                         256,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":      1,
	}
	ts := testSetup(t, overrides, true)

	// IP addresses in private range
	for i := range 256 {
		ts.rdnsQuerier.GetHostnameAsync(
			[]byte{192, 168, 1, byte(i)},
			func(hostname string) {
				assert.Equal(t, fmt.Sprintf("fakehostname-192.168.1.%d", i), hostname)
			},
		)
	}

	time.Sleep(2 * time.Second)

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
		"successful": 6.0, // running for 2 seconds, rate limit is 1 per second, add some buffer for timing
	}
	ts.validateMaximum(t, maximumTelemetry)

	assert.NoError(t, ts.lc.Stop(ts.ctx))

	//JMW now check for dropped_rate_limiter - or skip this since timing of shutdown could cause this to be lower than expected, even zero? OR separate test
	minimumTelemetry := map[string]float64{
		"dropped_rate_limiter": 1.0, // stopping the rdnsquerier will cause requests blocked in the rate limiter to be dropped
	}
	ts.validateMinimum(t, minimumTelemetry)
}
