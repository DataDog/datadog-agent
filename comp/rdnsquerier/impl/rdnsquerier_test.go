// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package rdnsquerierimpl

import (
	"context"
	"sync"
	"testing"

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

	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())

	requires := Requires{
		Lifecycle:   lc,
		AgentConfig: config,
		Logger:      logger,
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

func TestRDNSQuerierJMW(t *testing.T) {
	lc := compdef.NewTestLifecycle()

	overrides := map[string]interface{}{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}

	config := fxutil.Test[config.Component](t, fx.Options(
		config.MockModule(),
		fx.Replace(config.MockParams{Overrides: overrides}),
	))

	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
	telemetryComp := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())

	requires := Requires{
		Lifecycle:   lc,
		AgentConfig: config,
		Logger:      logger,
		Telemetry:   telemetryComp,
	}

	provides, err := NewComponent(requires)
	assert.NoError(t, err)
	assert.NotNil(t, provides.Comp)
	rdnsQuerier := provides.Comp

	ctx := context.Background()
	assert.NoError(t, lc.Start(ctx)) //JMWNEEDED?

	var wg sync.WaitGroup

	// Invalid IP address
	rdnsQuerier.GetHostnameAsync(
		[]byte{1, 2, 3},
		func(hostname string) {
			assert.FailNow(t, "Callback should be called for invalid IP address")
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
			// Expect "" due to no match because we don't have a real DNS resolver
			assert.Equal(t, "", hostname)
			wg.Done()
		},
	)
	wg.Wait()

	validateTelemetry := map[string]float64{
		"total":                3.0,
		"private":              1.0,
		"chan_added":           1.0,
		"dropped_chan_full":    0.0,
		"dropped_rate_limiter": 0.0,
		"invalid_ip_address":   1.0,
		//JMW"lookup_err_not_found": 1.0, //JMW will be 1 on my laptop, could be 0 on other test runners
		"lookup_err_timeout":   0.0,
		"lookup_err_temporary": 0.0,
		"lookup_err_other":     0.0,
		//"successful": 0.0, //JMW will be 0 on my laptop, could be 1 on other test runners
	}

	// Validate telemetry
	telemetryMock, ok := telemetryComp.(telemetry.Mock)
	assert.True(t, ok)
	for name, expected := range validateTelemetry {
		logger.Debugf("Validating metric %s", name)
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

	/*JMWTRY
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
