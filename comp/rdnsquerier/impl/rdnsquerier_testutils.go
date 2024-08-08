// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package rdnsquerierimpl

import (
	"context"
	"testing"

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

// Fake resolver is used by tests that "resolve" IP addresses to hostnames because with a real resolver test results
// can be non-deterministic.  Some systems may be able to resolve the private IP addresses used in the tests, others may not.
type fakeResolver struct {
	config *rdnsQuerierConfig
}

func (r *fakeResolver) lookup(addr string) (string, error) {
	return "fakehostname-" + addr, nil
}

type testState struct {
	lc            *compdef.TestLifecycle
	rdnsQuerier   rdnsquerier.Component
	ctx           context.Context
	telemetryMock telemetry.Mock
	logComp       log.Component
}

func testSetup(t *testing.T, overrides map[string]interface{}, start bool) *testState {
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

	rdnsQuerier := provides.Comp
	internalRDNSQuerier := provides.Comp.(*rdnsQuerierImpl)
	assert.NotNil(t, internalRDNSQuerier)

	// use fake resolver so the test results are deterministic
	internalRDNSQuerier.resolver = &fakeResolver{internalRDNSQuerier.config}

	ctx := context.Background()

	telemetryMock, ok := telemetryComp.(telemetry.Mock)
	assert.True(t, ok)

	if start {
		assert.NoError(t, lc.Start(ctx))
	}

	return &testState{lc, rdnsQuerier, ctx, telemetryMock, logComp}
}

func (ts *testState) validateExpected(t *testing.T, expectedTelemetry map[string]float64) {
	for name, expected := range expectedTelemetry {
		ts.logComp.Debugf("Validating expected telemetry %s", name)
		metrics, err := ts.telemetryMock.GetCountMetric(moduleName, name)
		if expected == 0 {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Len(t, metrics, 1)
			assert.Equal(t, expected, metrics[0].Value())
		}
	}
}

func (ts *testState) validateMinimum(t *testing.T, minimumTelemetry map[string]float64) {
	for name, expected := range minimumTelemetry {
		ts.logComp.Debugf("Validating minimum telemetry %s", name)
		metrics, err := ts.telemetryMock.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.GreaterOrEqual(t, metrics[0].Value(), expected)
	}
}

func (ts *testState) validateMaximum(t *testing.T, maximumTelemetry map[string]float64) {
	for name, expected := range maximumTelemetry {
		ts.logComp.Debugf("Validating maximum telemetry %s", name)
		metrics, err := ts.telemetryMock.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.LessOrEqual(t, metrics[0].Value(), expected)
	}
}
