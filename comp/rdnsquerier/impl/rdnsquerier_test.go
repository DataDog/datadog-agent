// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

package rdnsquerierimpl

import (
	"context"
	//JMWNEXT"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

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
	config := fxutil.Test[config.Component](t, config.MockModule())
	logger := fxutil.Test[log.Component](t, logimpl.MockModule())
	telemetry := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
	//JMWpacketsTelemetryStore := NewTelemetryStore(nil, telemetryComponent)
	//JMWpool := NewPool(1024, packetsTelemetryStore)

	requires := Requires{
		Lifecycle:   lc,
		AgentConfig: config,
		Logger:      logger,
		Telemetry:   telemetry,
	}

	provides, err := NewComponent(requires)
	assert.NoError(t, err)
	assert.NotNil(t, provides.Comp)
	//JMWNEXTrdnsQuerier := provides.Comp

	ctx := context.Background()
	assert.NoError(t, lc.Start(ctx))

	/*JMWNEXT
	var wg sync.WaitGroup
	wg.Add(1)
	rdnsQuerier.GetHostnameAsync(
		[]byte{192, 168, 1, 100},
		func(hostname string) {
			requires.Logger.Debugf("JMW Got hostname %s", hostname)
			wg.Done()
		},
	)

	wg.Wait()
	*/

	assert.NoError(t, lc.Stop(ctx))
}

/*
// testOptions is a fx collection of common dependencies for all tests
var testOptions = fx.Options(
	rdnsquerierfx.Module(),
	core.MockBundle(),
)

func newTestRDNSQuerier(t *testing.T, agentConfigs map[string]any) (*fxtest.App, *rdnsQuerierImpl) {
	var component rdnsquerier.Component
	app := fxtest.New(t, fx.Options(
		testOptions,
		fx.Supply(fx.Annotate(t, fx.As(new(testing.TB)))),
		fx.Replace(config.MockParams{Overrides: agentConfigs}),
		fx.Populate(&component),
	))
	rdnsQuerier := component.(*rdnsQuerierImpl)

	require.NotNil(t, rdnsQuerier)
	require.NotNil(t, app)
	return app, rdnsQuerier
}

func Test_RDNSQuerier_StartAndStop(t *testing.T) {
	// GIVEN
	agentConfigs := map[string]any{
		"network_devices.netflow.reverse_dns_enrichment_enabled": true,
	}
	app, rdnsQuerier := newTestRDNSQuerier(t, agentConfigs)
	app.RequireStart()
	app.RequireStop()

	// JMW validate logs and telemetry
	//JMWmetrics, err := telemetry.GetCountMetric("reverse_dns_enrichment", "total")
	//JMWassert.NoError(t, err)
	//JMWrequire.Len(t, metrics, 1)
	//JMWassert.Equal(t, metrics[0].Value(), 0.0)
	//JMWassert.Equal(t, 0, rdnsQuerier.internalTelemetry.total.GetValue())
	assert.Equal(t, rdnsQuerier.internalTelemetry.total.WithValues("").Get(), 0.0)
}
*/
