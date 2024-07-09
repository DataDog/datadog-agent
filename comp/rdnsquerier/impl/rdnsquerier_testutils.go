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

	"github.com/stretchr/testify/assert" //JMW require for some?

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
)

// Fake resolver is used by tests that "resolve" IP addresses to hostnames because with the real resolver test results
// can be non-deterministic.  Some systems may be able to resolve the private IP addresses used in the tests, others may not.
type fakeResolver struct {
	config *rdnsQuerierConfig
}

func (r *fakeResolver) lookup(addr string) (string, error) {
	return "fakehostname-" + addr, nil
}

func testSetup(t *testing.T, overrides map[string]interface{}) (*compdef.TestLifecycle, rdnsquerier.Component, context.Context, telemetry.Mock, log.Component) {
	lc := compdef.NewTestLifecycle()

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
	internalRDNSQuerier := provides.Comp.(*rdnsQuerierImpl)
	assert.NotNil(t, internalRDNSQuerier)

	// use fake resolver so the test results are deterministic
	internalRDNSQuerier.resolver = &fakeResolver{internalRDNSQuerier.config}

	ctx := context.Background()

	telemetryMock, ok := telemetryComp.(telemetry.Mock)
	assert.True(t, ok)

	return lc, rdnsQuerier, ctx, telemetryMock, logComp
}

func validateExpected(t *testing.T, tm telemetry.Mock, logComp log.Component, expectedTelemetry map[string]float64) {
	for name, expected := range expectedTelemetry {
		logComp.Debugf("Validating expected telemetry %s", name)
		metrics, err := tm.GetCountMetric(moduleName, name)
		if expected == 0 {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Len(t, metrics, 1)
			assert.Equal(t, expected, metrics[0].Value())
		}
	}
}

func validateMinimum(t *testing.T, tm telemetry.Mock, logComp log.Component, minimumTelemetry map[string]float64) {
	for name, expected := range minimumTelemetry {
		logComp.Debugf("Validating minimum telemetry %s", name)
		metrics, err := tm.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.GreaterOrEqual(t, metrics[0].Value(), expected)
	}
}

func validateMaximum(t *testing.T, tm telemetry.Mock, logComp log.Component, maximumTelemetry map[string]float64) {
	for name, expected := range maximumTelemetry {
		logComp.Debugf("Validating maximum telemetry %s", name)
		metrics, err := tm.GetCountMetric(moduleName, name)
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.LessOrEqual(t, metrics[0].Value(), expected)
	}
}
