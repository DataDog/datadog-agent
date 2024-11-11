// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package rdnsquerierimpl

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
)

type fakeResults struct {
	errors []error
}

// Fake resolver is used by tests that "resolve" IP addresses to hostnames because with a real resolver test results
// can be non-deterministic.  Some systems may be able to resolve the private IP addresses used in the tests, others may not.
// It is also used by certain tests to force specific error(s) for an IP address.
type fakeResolver struct {
	config        *rdnsQuerierConfig
	fakeIPResults map[string]*fakeResults
	delay         time.Duration
	logger        log.Component
}

func (r *fakeResolver) lookup(addr string) (string, error) {

	r.logger.Infof("fakeResolver.lookup(%s) with timeout %d", addr, r.delay)

	if r.delay > 0 {
		time.Sleep(r.delay)
	}

	fr, ok := r.fakeIPResults[addr]
	if ok && len(fr.errors) > 0 {
		err := fr.errors[0]
		if len(fr.errors) > 1 {
			fr.errors = fr.errors[1:]
		} else {
			fr.errors = nil
		}
		return "", err
	}

	return "fakehostname-" + addr, nil
}

type testState struct {
	lc            *compdef.TestLifecycle
	rdnsQuerier   rdnsquerier.Component
	ctx           context.Context
	telemetryMock telemetry.Mock
	logComp       log.Component
}

func testSetup(t *testing.T, overrides map[string]interface{}, start bool, fakeIPResults map[string]*fakeResults, delay time.Duration) *testState {
	lc := compdef.NewTestLifecycle(t)

	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))

	logComp := logmock.New(t)
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

	ctx := context.Background()
	telemetryMock, ok := telemetryComp.(telemetry.Mock)
	assert.True(t, ok)
	ts := testState{lc, provides.Comp, ctx, telemetryMock, logComp}

	// use fake resolver so the test results are deterministic
	internalRDNSQuerier := provides.Comp.(*rdnsQuerierImpl)
	assert.NotNil(t, internalRDNSQuerier)
	if internalRDNSQuerier.config.cache.enabled {
		internalCache := internalRDNSQuerier.cache.(*cacheImpl)
		assert.NotNil(t, internalCache)
		internalQuerier := internalCache.querier.(*querierImpl)
		assert.NotNil(t, internalQuerier)
		internalQuerier.resolver = &fakeResolver{internalRDNSQuerier.config, fakeIPResults, delay, logComp}
	} else {
		internalCache := internalRDNSQuerier.cache.(*cacheNone)
		assert.NotNil(t, internalCache)
		internalQuerier := internalCache.querier.(*querierImpl)
		assert.NotNil(t, internalQuerier)
		internalQuerier.resolver = &fakeResolver{internalRDNSQuerier.config, fakeIPResults, delay, logComp}
	}

	if start {
		assert.NoError(t, lc.Start(ctx))
	}

	return &ts
}

func (ts *testState) makeExpectedTelemetry(checkTelemetry map[string]float64) map[string]float64 {
	et := map[string]float64{
		"total":                   0.0,
		"private":                 0.0,
		"chan_added":              0.0,
		"dropped_chan_full":       0.0,
		"dropped_rate_limiter":    0.0,
		"invalid_ip_address":      0.0,
		"lookup_err_not_found":    0.0,
		"lookup_err_timeout":      0.0,
		"lookup_err_temporary":    0.0,
		"lookup_err_other":        0.0,
		"successful":              0.0,
		"cache_hit":               0.0,
		"cache_hit_expired":       0.0,
		"cache_hit_in_progress":   0.0,
		"cache_miss":              0.0,
		"cache_retry":             0.0,
		"cache_retries_exceeded":  0.0,
		"cache_expired":           0.0,
		"cache_max_size_exceeded": 0.0,
	}
	for name, value := range checkTelemetry {
		et[name] = value
	}
	return et
}

// validate that telemetry counter values are equal to the expected values
func (ts *testState) validateExpected(t *testing.T, expectedTelemetry map[string]float64) {
	for name, expected := range expectedTelemetry {
		ts.logComp.Debugf("Validating expected telemetry counter %s", name)
		metrics, err := ts.telemetryMock.GetCountMetric(moduleName, name)
		if expected == 0 {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Len(t, metrics, 1)
			assert.Equal(t, expected, metrics[0].Value(), name)
		}
	}
}

// validate that telemetry counter values are greater than or equal to the expected minimum values
func (ts *testState) validateMinimum(t *testing.T, minimumTelemetry map[string]float64) {
	for name, expected := range minimumTelemetry {
		ts.logComp.Debugf("Validating minimum telemetry counter %s", name)
		metrics, err := ts.telemetryMock.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.GreaterOrEqual(t, metrics[0].Value(), expected)
	}
}

// validate that telemetry counter values are less than or equal to the expected maximum values
func (ts *testState) validateMaximum(t *testing.T, maximumTelemetry map[string]float64) {
	for name, expected := range maximumTelemetry {
		ts.logComp.Debugf("Validating maximum telemetry counter %s", name)
		metrics, err := ts.telemetryMock.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.LessOrEqual(t, metrics[0].Value(), expected)
	}
}

// validate that telemetry gauge value is equal to the expected value
func (ts *testState) validateExpectedGauge(t *testing.T, name string, value float64) {
	ts.logComp.Debugf("Validating expected telemetry gauge %s", name)
	metrics, err := ts.telemetryMock.GetGaugeMetric(moduleName, name)
	assert.NoError(t, err)
	assert.Len(t, metrics, 1)
	assert.Equal(t, value, metrics[0].Value())
}
