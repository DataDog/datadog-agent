// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build otlp && test

package collectorimpl

import (
	"context"
	"testing"

	datadogextension "github.com/open-telemetry/opentelemetry-collector-contrib/extension/datadogextension"
	datadogconfig "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/datadog/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configopaque"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/pdata/pcommon"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func testAgentConfig(t *testing.T) coreconfig.Component {
	t.Helper()
	cfg := coreconfig.NewMock(t)
	// Written above SourceEnvVar so an ambient DD_API_KEY cannot mask it.
	cfg.Set("api_key", "agent-key", pkgconfigmodel.SourceConfigPostInit)
	return cfg
}

// TestBridgeDatadogExtensionAPIKeyReplacesExistingFactory verifies the override swaps the
// upstream factory for one carrying the API-key bridge.
func TestBridgeDatadogExtensionAPIKeyReplacesExistingFactory(t *testing.T) {
	original := datadogextension.NewFactory()
	ddExtType := original.Type()

	factories := otelcol.Factories{
		Extensions: map[component.Type]extension.Factory{ddExtType: original},
	}

	bridgeDatadogExtensionAPIKey(factories, testAgentConfig(t), logmock.New(t))

	replaced, ok := factories.Extensions[ddExtType]
	require.True(t, ok, "the datadog extension factory must still be registered")
	assert.Equal(t, ddExtType, replaced.Type())
	assert.NotSame(t, original, replaced, "the factory should have been replaced")
}

// TestDatadogExtensionFactoryNotAddedWhenAbsent is the BYOC guard. A user-supplied OCB
// manifest may legitimately omit the datadog extension -- it is not in
// MANDATORY_COMPONENTS -- and this wiring must not silently add back a component the
// operator deliberately excluded.
func TestDatadogExtensionFactoryNotAddedWhenAbsent(t *testing.T) {
	factories := otelcol.Factories{
		Extensions: map[component.Type]extension.Factory{},
	}

	bridgeDatadogExtensionAPIKey(factories, testAgentConfig(t), logmock.New(t))

	assert.Empty(t, factories.Extensions,
		"the datadog extension must not be added to a build that omitted it")
}

// TestBridgedFactoryRotatesExtensionForwarder is the end-to-end wiring proof: it drives the
// replaced factory the way the collector does -- Create an extension from it -- then rotates
// the Agent's key and asserts the rotation reached the extension's own forwarder.
//
// The other two tests above only observe the factory map. They would still pass if the
// override installed a plain NewFactory(), i.e. if the bridge option were dropped; this one
// would not. The observable is the log the forwarder's domain resolver emits from the
// callback NewOptions arms on the extension's private config -- the same line Support greps
// for in the field.
func TestBridgedFactoryRotatesExtensionForwarder(t *testing.T) {
	const agentKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const rotatedKey = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	shared := coreconfig.NewMock(t)
	shared.Set("api_key", agentKey, pkgconfigmodel.SourceConfigPostInit)
	require.Equal(t, agentKey, shared.GetString("api_key"), "test setup: api_key not effective")

	ddExtType := datadogextension.NewFactory().Type()
	factories := otelcol.Factories{
		Extensions: map[component.Type]extension.Factory{ddExtType: datadogextension.NewFactory()},
	}
	bridgeDatadogExtensionAPIKey(factories, shared, logmock.New(t))

	factory := factories.Extensions[ddExtType]
	cfg := factory.CreateDefaultConfig().(*datadogextension.Config)
	cfg.API.Key = configopaque.String(agentKey)
	cfg.API.Site = datadogconfig.DefaultSite
	cfg.API.FailOnInvalidKey = false
	// A set hostname keeps newExtension off the cloud-metadata probes.
	cfg.Hostname = "test-host"

	// Creating the extension builds its private config and forwarder, which is what arms the
	// resolver's rotation callback. Capture the collector logger the forwarder logs through.
	core, logs := observer.New(zapcore.InfoLevel)
	set := extension.Settings{
		ID: component.NewID(ddExtType),
		TelemetrySettings: component.TelemetrySettings{
			Logger:         zap.New(core),
			MeterProvider:  noopmetric.NewMeterProvider(),
			TracerProvider: nooptrace.NewTracerProvider(),
			Resource:       pcommon.NewResource(),
		},
		BuildInfo: component.NewDefaultBuildInfo(),
	}

	ext, err := factory.Create(context.Background(), set, cfg)
	require.NoError(t, err)
	require.NotNil(t, ext)

	shared.Set("api_key", rotatedKey, pkgconfigmodel.SourceLocalConfigProcess)

	rotations := logs.FilterMessageSnippet("rotating API key for 'api_key'").All()
	assert.Len(t, rotations, 1, "the extension's forwarder did not rotate to the new API key")
}
