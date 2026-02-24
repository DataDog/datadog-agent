// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventplatformimpl

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
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

	suite.receiver = fxutil.Test[eventplatformreceiver.Component](suite.T(), eventplatformreceiverimpl.MockModule())
	suite.compression = fxutil.Test[logscompression.Component](suite.T(), logscompressionfxmock.MockModule())
}

func TestEventPlatformForwarderTestSuite(t *testing.T) {
	suite.Run(t, new(EventPlatformForwarderTestSuite))
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
				c.SetInTest("database_monitoring.metrics.additional_endpoints", `[{"api_key":"foo","host":"bar"}]`)
			},
			expectedKind:   gzipCompressionKind,
			expectedLevel:  defaultGzipCompressionLevel,
			useCompression: defaultUseCompression,
		},
		{
			name: "no compression",
			configSetup: func(c config.Component) {
				c.SetInTest("database_monitoring.metrics.use_compression", false)
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
				c.SetInTest("database_monitoring.metrics.compression_kind", "zstd")
				c.SetInTest("database_monitoring.metrics.zstd_compression_level", 3)
			},
			expectedKind:   zstdCompressionKind,
			expectedLevel:  3,
			useCompression: defaultUseCompression,
		},
		{
			name: "gzip compression",
			configSetup: func(c config.Component) {
				c.SetInTest("database_monitoring.metrics.compression_kind", "gzip")
			},
			expectedKind:   gzipCompressionKind,
			expectedLevel:  defaultGzipCompressionLevel,
			useCompression: defaultUseCompression,
		},
		{
			name: "gzip custom compression level",
			configSetup: func(c config.Component) {
				c.SetInTest("database_monitoring.metrics.compression_kind", "gzip")
				c.SetInTest("database_monitoring.metrics.compression_level", 8)
			},
			expectedKind:   gzipCompressionKind,
			expectedLevel:  8,
			useCompression: defaultUseCompression,
		},
		{
			name: "invalid compression",
			configSetup: func(c config.Component) {
				c.SetInTest("database_monitoring.metrics.use_compression", true)
				c.SetInTest("database_monitoring.metrics.compression_kind", "gipz")
			},
			expectedKind:   zstdCompressionKind,
			expectedLevel:  defaultZstdCompressionLevel,
			useCompression: defaultUseCompression,
		},
	}

	for _, t := range tests {
		suite.Run(t.name, func() {

			t.configSetup(suite.config)

			desc := passthroughPipelineDesc{
				// Only registered config prefixes trigger correct parsing and defaults.
				endpointsConfigPrefix: "database_monitoring.metrics.",
			}

			pipeline, err := newHTTPPassthroughPipeline(
				suite.config,
				nil,
				suite.compression,
				desc,
				nil,
				0,
			)
			suite.Require().NoError(err)
			suite.Require().NotNil(pipeline)

			configKeys := laconfig.NewLogsConfigKeys(desc.endpointsConfigPrefix, suite.config)
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
	suite.config.SetInTest("database_monitoring.metrics.use_compression", true)
	suite.config.SetInTest("database_monitoring.metrics.compression_kind", "zstd")
	suite.config.SetInTest("database_monitoring.metrics.compression_level", 6)
	suite.config.SetInTest("database_monitoring.metrics.zstd_compression_level", defaultZstdCompressionLevel)
	suite.config.SetInTest("database_monitoring.metrics.additional_endpoints", "{}")

}
