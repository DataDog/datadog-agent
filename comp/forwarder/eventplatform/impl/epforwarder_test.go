// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/core/config"
	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformreceiver "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/def"
	eventplatformreceivermock "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/mock"
	laconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionfxmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	defaultUseCompression       = true
	zstdCompressionKind         = "zstd"
	defaultZstdCompressionLevel = 1
	gzipCompressionKind         = "gzip"
	defaultGzipCompressionLevel = 6
)

type EventPlatformForwarderTestSuite struct {
	suite.Suite
	config      config.Component
	receiver    eventplatformreceiver.Component
	compression logscompression.Component
}

func (suite *EventPlatformForwarderTestSuite) SetupTest() {
	suite.config = config.NewMock(suite.T())

	suite.receiver = fxutil.Test[eventplatformreceiver.Component](suite.T(), eventplatformreceivermock.MockModule())
	suite.compression = fxutil.Test[logscompression.Component](suite.T(), logscompressionfxmock.MockModule())
}

func TestEventPlatformForwarderTestSuite(t *testing.T) {
	suite.Run(t, new(EventPlatformForwarderTestSuite))
}

func TestPassthroughPipelineDescriptions(t *testing.T) {
	descs := getPassthroughPipelines()
	require.NotEmpty(t, descs)

	seenEventTypes := make(map[string]struct{}, len(descs))
	for _, desc := range descs {
		require.NotEmpty(t, desc.EventType)
		if _, found := seenEventTypes[desc.EventType]; found {
			t.Fatalf("duplicate event-platform pipeline descriptor for eventType=%s", desc.EventType)
		}
		seenEventTypes[desc.EventType] = struct{}{}

		require.NotEmpty(t, desc.ContentType, "missing content type for eventType=%s", desc.EventType)
		require.NotEmpty(t, desc.EndpointsConfigPrefix, "missing config prefix for eventType=%s", desc.EventType)
		require.NotEmpty(t, desc.HostnameEndpointPrefix, "missing hostname endpoint prefix for eventType=%s", desc.EventType)
		require.GreaterOrEqual(t, desc.DefaultBatchMaxConcurrentSend, 0, "invalid batch concurrency default for eventType=%s", desc.EventType)
		require.Positive(t, desc.DefaultBatchMaxContentSize, "missing batch content size default for eventType=%s", desc.EventType)
		require.Positive(t, desc.DefaultBatchMaxSize, "missing batch size default for eventType=%s", desc.EventType)
		require.Positive(t, desc.DefaultInputChanSize, "missing input channel default for eventType=%s", desc.EventType)
	}
}

func (suite *EventPlatformForwarderTestSuite) TestNewHTTPPassthroughPipelineCompression() {

	tests := []struct {
		name           string
		configSetup    func(config.Component)
		expectedKind   string
		expectedLevel  int
		useCompression bool
	}{
		{
			name: "additional endpoints",
			configSetup: func(c config.Component) {
				c.SetWithoutSource("database_monitoring.metrics.additional_endpoints", `[{"api_key":"foo","host":"bar"}]`)
			},
			expectedKind:   gzipCompressionKind,
			expectedLevel:  defaultGzipCompressionLevel,
			useCompression: defaultUseCompression,
		},
		{
			name: "no compression",
			configSetup: func(c config.Component) {
				c.SetWithoutSource("database_monitoring.metrics.use_compression", false)
			},
			expectedKind:   "none",
			expectedLevel:  0,
			useCompression: !defaultUseCompression,
		},
		{
			name:           "default compression",
			configSetup:    func(_ config.Component) {},
			expectedKind:   zstdCompressionKind,
			expectedLevel:  defaultZstdCompressionLevel,
			useCompression: defaultUseCompression,
		},
		{
			name: "zstd custom compression level",
			configSetup: func(c config.Component) {
				c.SetWithoutSource("database_monitoring.metrics.compression_kind", "zstd")
				c.SetWithoutSource("database_monitoring.metrics.zstd_compression_level", 3)
			},
			expectedKind:   zstdCompressionKind,
			expectedLevel:  3,
			useCompression: defaultUseCompression,
		},
		{
			name: "gzip compression",
			configSetup: func(c config.Component) {
				c.SetWithoutSource("database_monitoring.metrics.compression_kind", "gzip")
			},
			expectedKind:   gzipCompressionKind,
			expectedLevel:  defaultGzipCompressionLevel,
			useCompression: defaultUseCompression,
		},
		{
			name: "gzip custom compression level",
			configSetup: func(c config.Component) {
				c.SetWithoutSource("database_monitoring.metrics.compression_kind", "gzip")
				c.SetWithoutSource("database_monitoring.metrics.compression_level", 8)
			},
			expectedKind:   gzipCompressionKind,
			expectedLevel:  8,
			useCompression: defaultUseCompression,
		},
		{
			name: "invalid compression",
			configSetup: func(c config.Component) {
				c.SetWithoutSource("database_monitoring.metrics.use_compression", true)
				c.SetWithoutSource("database_monitoring.metrics.compression_kind", "gipz")
			},
			expectedKind:   zstdCompressionKind,
			expectedLevel:  defaultZstdCompressionLevel,
			useCompression: defaultUseCompression,
		},
	}

	for _, t := range tests {
		suite.Run(t.name, func() {

			t.configSetup(suite.config)

			desc := eventplatform.PipelineDesc{
				// Only registered config prefixes trigger correct parsing and defaults.
				EndpointsConfigPrefix: "database_monitoring.metrics.",
			}

			pipeline, err := newHTTPPassthroughPipeline(
				suite.config,
				nil,
				suite.compression,
				desc,
				nil,
				0,
				"test-hostname",
				secretsnoopimpl.NewComponent().Comp,
			)
			suite.Require().NoError(err)
			suite.Require().NotNil(pipeline)

			configKeys := laconfig.NewLogsConfigKeys(desc.EndpointsConfigPrefix, suite.config)
			endpoints, err := laconfig.BuildHTTPEndpointsWithConfig(
				suite.config,
				configKeys,
				"", // hostnameEndpointPrefix not needed
				"", // intakeTrackType not needed
				"", // protocol not needed
				"", // origin not needed
			)
			suite.Require().NoError(err)

			// Check compression settings on endpoint
			suite.Equal(t.useCompression, endpoints.Main.UseCompression, "UseCompression mismatch")
			if t.useCompression {
				suite.Equal(t.expectedKind, endpoints.Main.CompressionKind, "CompressionKind mismatch")
				suite.Equal(t.expectedLevel, endpoints.Main.CompressionLevel, "CompressionLevel mismatch")
			}
		})
		suite.resetCompression()
	}
}

func (suite *EventPlatformForwarderTestSuite) resetCompression() {
	// Reset compression settings to default state
	suite.config.SetWithoutSource("database_monitoring.metrics.use_compression", true)
	suite.config.SetWithoutSource("database_monitoring.metrics.compression_kind", "zstd")
	suite.config.SetWithoutSource("database_monitoring.metrics.compression_level", 6)
	suite.config.SetWithoutSource("database_monitoring.metrics.zstd_compression_level", defaultZstdCompressionLevel)
	suite.config.SetWithoutSource("database_monitoring.metrics.additional_endpoints", "{}")

}
