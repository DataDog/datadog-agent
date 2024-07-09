// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package rdnsquerierimpl

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

func TestRDNSQuerierStartStop(t *testing.T) {
	lc := compdef.NewTestLifecycle()

	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}

	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))

	logComp := fxutil.Test[log.Component](t, logimpl.MockModule())
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())

	requires := Requires{
		Lifecycle:   lc,
		AgentConfig: config,
		Logger:      logComp,
		Telemetry:   telemetryComp,
	}

	provides, err := NewComponent(requires)
	assert.NoError(t, err)
	assert.NotNil(t, provides.Comp)
	//JMWrdnsQuerier := provides.Comp

	internalRDNSQuerier := provides.Comp.(*rdnsQuerierImpl)
	assert.NotNil(t, internalRDNSQuerier)
	assert.Equal(t, false, internalRDNSQuerier.started)

	ctx := context.Background()

	assert.NoError(t, lc.Start(ctx))
	assert.Equal(t, true, internalRDNSQuerier.started)

	assert.NoError(t, lc.Stop(ctx))
	assert.Equal(t, false, internalRDNSQuerier.started)
}

// Fake resolver is used because otherwise the test results can be indeterminate. Some systems may be able to
// resolve the private IP addresses used in the tests, others may not.
type fakeResolver struct {
	config *rdnsQuerierConfig
}

func (r *fakeResolver) lookup(addr string) (string, error) {
	return "fakehostname-" + addr, nil
}

func TestRDNSQuerierJMW(t *testing.T) {
	lc := compdef.NewTestLifecycle()

	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}

	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))

	logComp := fxutil.Test[log.Component](t, logimpl.MockModule())
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())

	requires := Requires{
		Lifecycle:   lc,
		AgentConfig: config,
		Logger:      logComp,
		Telemetry:   telemetryComp,
	}

	provides, err := NewComponent(requires)
	assert.NoError(t, err)
	assert.NotNil(t, provides.Comp)
	rdnsQuerier := provides.Comp

	// use fake resolver so the test results are determinate
	internalRDNSQuerier := provides.Comp.(*rdnsQuerierImpl)
	internalRDNSQuerier.resolver = &fakeResolver{internalRDNSQuerier.config}

	ctx := context.Background()
	assert.NoError(t, lc.Start(ctx)) //JMWNEEDED?

	var wg sync.WaitGroup

	// Invalid IP address
	rdnsQuerier.GetHostnameAsync(
		[]byte{1, 2, 3},
		func(hostname string) {
			assert.FailNow(t, "Callback should not be called for invalid IP address")
		},
	)

	// IP address not in private range
	rdnsQuerier.GetHostnameAsync(
		[]byte{8, 8, 8, 8},
		func(hostname string) {
			assert.FailNow(t, "Callback should not be called for IP address not in private range")
		},
	)

	// IP address in private range
	wg.Add(1)
	rdnsQuerier.GetHostnameAsync(
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

	// Validate telemetry
	telemetryMock, ok := telemetryComp.(telemetry.Mock)
	assert.True(t, ok)
	for name, expected := range expectedTelemetry {
		logComp.Debugf("Validating metric %s", name)
		metrics, err := telemetryMock.GetCountMetric(moduleName, name)
		if expected == 0 {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Len(t, metrics, 1)
			assert.Equal(t, expected, metrics[0].Value())
		}
	}

	assert.NoError(t, lc.Stop(ctx))

	/*JMWTRY after stopping - or as separate test
	rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			assert.FailNow(t, "Callback should not be called since rdnsquerier is stopped")
		},
	)

	metrics, err := telemetryMock.GetCountMetric(moduleName, "dropped_chan_full")
	assert.NoError(t, err)
	assert.Len(t, metrics, 1)
	assert.Equal(t, 1.0, metrics[0].Value())
	*/
}

// JMWNEW add test to validate rate limiter and channel that 1) rate is limited and 2) once rate limit is exceeded, channel gets full and requests are dropped
func TestRDNSQuerierJMW2(t *testing.T) {
	lc := compdef.NewTestLifecycle()

	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.workers":                         1,
		"reverse_dns_enrichment.chan_size":                       1,
		"reverse_dns_enrichment.rate_limiter.enabled":            true,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":      1,
	}

	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))

	logComp := fxutil.Test[log.Component](t, logimpl.MockModule())
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())

	requires := Requires{
		Lifecycle:   lc,
		AgentConfig: config,
		Logger:      logComp,
		Telemetry:   telemetryComp,
	}

	provides, err := NewComponent(requires)
	assert.NoError(t, err)
	assert.NotNil(t, provides.Comp)
	rdnsQuerier := provides.Comp

	// use fake resolver so the test results are determinate
	internalRDNSQuerier := provides.Comp.(*rdnsQuerierImpl)
	internalRDNSQuerier.resolver = &fakeResolver{internalRDNSQuerier.config}

	ctx := context.Background()
	assert.NoError(t, lc.Start(ctx)) //JMWNEEDED?

	var wg sync.WaitGroup

	// IP addresses in private range
	wg.Add(1) // only wait for one callback, some or all of the other requests will be dropped
	var once sync.Once
	for i := range 256 {
		rdnsQuerier.GetHostnameAsync(
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

	minimumTelemetry := map[string]float64{
		"chan_added":        1.0,
		"dropped_chan_full": 1.0,
		"successful":        1.0,
	}

	logComp.Debugf("JMW internalRDNSQuerier telemetry: %+v", internalRDNSQuerier.internalTelemetry)
	// Validate telemetry
	telemetryMock, ok := telemetryComp.(telemetry.Mock)
	assert.True(t, ok)
	for name, expected := range expectedTelemetry {
		logComp.Debugf("Validating expected telemetry %s", name)
		metrics, err := telemetryMock.GetCountMetric(moduleName, name)
		if expected == 0 {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Len(t, metrics, 1)
			assert.Equal(t, expected, metrics[0].Value())
		}
	}
	for name, expected := range minimumTelemetry {
		logComp.Debugf("Validating minimum telemetry %s", name)
		metrics, err := telemetryMock.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.GreaterOrEqual(t, metrics[0].Value(), expected)
	}

	assert.NoError(t, lc.Stop(ctx))
}

// JMWNEW add test for rate limiter - set to 1 per second, fill channel with 256 requests, run for 5 seconds, assert <= 5 requests are successful within that time
func TestRDNSQuerierJMW3(t *testing.T) {
	lc := compdef.NewTestLifecycle()

	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
		"reverse_dns_enrichment.rate_limiter.limit_per_sec":      1,
	}

	//JMWDUPSETUP
	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))

	logComp := fxutil.Test[log.Component](t, logimpl.MockModule())
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())

	requires := Requires{
		Lifecycle:   lc,
		AgentConfig: config,
		Logger:      logComp,
		Telemetry:   telemetryComp,
	}

	provides, err := NewComponent(requires)
	assert.NoError(t, err)
	assert.NotNil(t, provides.Comp)
	rdnsQuerier := provides.Comp

	// use fake resolver so the test results are determinate
	internalRDNSQuerier := provides.Comp.(*rdnsQuerierImpl)
	internalRDNSQuerier.resolver = &fakeResolver{internalRDNSQuerier.config}

	ctx := context.Background()
	assert.NoError(t, lc.Start(ctx)) //JMWNEEDED?

	// IP addresses in private range
	for i := range 256 {
		rdnsQuerier.GetHostnameAsync(
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

	maximumTelemetry := map[string]float64{
		"successful": 6.0, // running for 2 seconds, rate limit is 1 per second, add some buffer for timing
	}

	logComp.Debugf("JMW internalRDNSQuerier telemetry: %+v", internalRDNSQuerier.internalTelemetry)
	// Validate telemetry
	telemetryMock, ok := telemetryComp.(telemetry.Mock)
	assert.True(t, ok)

	for name, expected := range expectedTelemetry {
		logComp.Debugf("Validating expected telemetry %s", name)
		metrics, err := telemetryMock.GetCountMetric(moduleName, name)
		if expected == 0 {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Len(t, metrics, 1)
			assert.Equal(t, expected, metrics[0].Value())
		}
	}

	for name, expected := range maximumTelemetry {
		logComp.Debugf("Validating maximum telemetry %s", name)
		metrics, err := telemetryMock.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.LessOrEqual(t, metrics[0].Value(), expected)
	}

	assert.NoError(t, lc.Stop(ctx))

	//JMW now check for dropped_rate_limiter - or skip this since timing of shutdown could cause this to be lower than expected, even zero?
	logComp.Debugf("JMW2 internalRDNSQuerier telemetry: %+v", internalRDNSQuerier.internalTelemetry)

	minimumTelemetry := map[string]float64{
		"dropped_rate_limiter": 1.0, // stopping the rdnsquerier will cause requests blocked in the rate limiter to be dropped
	}
	for name, expected := range minimumTelemetry {
		logComp.Debugf("Validating minimum telemetry %s", name)
		metrics, err := telemetryMock.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.GreaterOrEqual(t, metrics[0].Value(), expected)
	}
}
