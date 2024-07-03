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
	telemetry := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())

	requires := Requires{
		Lifecycle:   lc,
		AgentConfig: config,
		Logger:      logger,
		Telemetry:   telemetry,
	}

	provides, err := NewComponent(requires)
	assert.NoError(t, err)
	assert.NotNil(t, provides.Comp)
	rdnsQuerier := provides.Comp

	ctx := context.Background()
	assert.NoError(t, lc.Start(ctx))

	var wg sync.WaitGroup

	// Invalid IP address
	rdnsQuerier.GetHostnameAsync(
		[]byte{1, 2, 3},
		func(hostname string) {
			assert.FailNow(t, "Callback should not have been called for invalid IP address")
		},
	)

	// IP address not in private range
	rdnsQuerier.GetHostnameAsync(
		[]byte{8, 8, 8, 8},
		func(hostname string) {
			assert.FailNow(t, "Callback should not have been called for IP address not in private range")
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

	//mock.go:50: 2024-07-02 17:50:41 MDT | DEBUG | (comp/rdnsquerier/impl/rdnsquerier_test.go:89 in TestRDNSQuerierStartStop) | rdnsQuerierImpl.internalTelemetry.total = &{pc:0x1400012c6b8}
	rdnsQuerierImpl := provides.Comp.(*rdnsQuerierImpl)
	logger.Debugf("rdnsQuerierImpl.internalTelemetry.total = %+v", rdnsQuerierImpl.internalTelemetry.total)

	// comp/rdnsquerier/impl/rdnsquerier_test.go:94:44: telemetry.Mock is not a type
	// FAIL	github.com/DataDog/datadog-agent/comp/rdnsquerier/impl [build failed]
	/*
		telemetryMock, ok := telemetry.(telemetry.Mock)
		assert.True(t, ok)
		metrics, err := telemetryMock.GetCountMetric(moduleName, "total")
		assert.NoError(t, err)
		assert.Len(t, metrics, 1)
		assert.Equal(t, metrics[0].Value(), 1.0)
	*/

	assert.NoError(t, lc.Stop(ctx))
}
