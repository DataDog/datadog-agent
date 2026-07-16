// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package networkpath

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	tracerouteconfig "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
)

func TestNewCheckConfig(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("network_devices.namespace", "my-namespace")
	tests := []struct {
		name           string
		rawInstance    integration.Data
		rawInitConfig  integration.Data
		expectedConfig *CheckConfig
		expectedError  string
	}{
		{
			name: "valid minimal config",
			rawInstance: []byte(`
hostname: 1.2.3.4
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "test config id",
			rawInstance: []byte(`
test_config_id: test-config-a
hostname: 1.2.3.4
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				TestConfigID:          "test-config-a",
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name:          "invalid raw instance json",
			rawInstance:   []byte(`{{{`),
			expectedError: "invalid instance config",
		},
		{
			name:          "invalid raw instance json",
			rawInstance:   []byte(`hostname: 1.2.3.4`),
			rawInitConfig: []byte(`{{{`),
			expectedError: "invalid init_config",
		},
		{
			name: "invalid min_collection_interval",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: -1
`),
			expectedError: "min collection interval must be > 0",
		},
		{
			name: "min_collection_interval from instance",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "min_collection_interval from init_config",
			rawInstance: []byte(`
hostname: 1.2.3.4
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(10) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "min_collection_interval default value",
			rawInstance: []byte(`
hostname: 1.2.3.4
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(1) * time.Minute,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "source and destination service config",
			rawInstance: []byte(`
hostname: 1.2.3.4
source_service: service-a
destination_service: service-b
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				SourceService:         "service-a",
				DestinationService:    "service-b",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "lower case protocol",
			rawInstance: []byte(`
hostname: 1.2.3.4
protocol: udp
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Protocol:              payload.ProtocolUDP,
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "lower case protocol",
			rawInstance: []byte(`
hostname: 1.2.3.4
protocol: UDP
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Protocol:              payload.ProtocolUDP,
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "lower case protocol",
			rawInstance: []byte(`
hostname: 1.2.3.4
protocol: TCP
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Protocol:              payload.ProtocolTCP,
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "timeout from instance config",
			rawInstance: []byte(`
hostname: 1.2.3.4
timeout: 50000
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               50000 * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "timeout from instance config preferred over init config",
			rawInstance: []byte(`
hostname: 1.2.3.4
timeout: 50000
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
timeout: 70000
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               50000 * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "timeout from init config",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
timeout: 70000
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               70000 * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "default timeout",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "negative timeout returns an error",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
timeout: -1
`),
			expectedError: "timeout must be > 0",
		},
		{
			name: "maxTTL from instance config",
			rawInstance: []byte(`
hostname: 1.2.3.4
max_ttl: 50
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                50,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "maxTTL from instance config preferred over init config",
			rawInstance: []byte(`
hostname: 1.2.3.4
max_ttl: 50
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
max_ttl: 64
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                50,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "maxTTL from init config",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
max_ttl: 64
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                64,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "overriding the TCP method",
			rawInstance: []byte(`
hostname: 1.2.3.4
protocol: tcp
tcp_method: sack
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Protocol:              payload.ProtocolTCP,
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TCPMethod:             payload.TCPConfigSACK,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "TCP method converts to lower case",
			rawInstance: []byte(`
hostname: 1.2.3.4
protocol: tcp
tcp_method: prefer_SACK
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Protocol:              payload.ProtocolTCP,
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TCPMethod:             payload.TCPConfigPreferSACK,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "Enabling TCP SYN compatibility mode",
			rawInstance: []byte(`
hostname: 1.2.3.4
protocol: tcp
tcp_syn_paris_traceroute_mode: true
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:              "1.2.3.4",
				MinCollectionInterval:     time.Duration(60) * time.Second,
				Namespace:                 "my-namespace",
				Protocol:                  payload.ProtocolTCP,
				Timeout:                   setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                    setup.DefaultNetworkPathMaxTTL,
				TCPSynParisTracerouteMode: true,
				TracerouteQueries:         setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:                setup.DefaultNetworkPathStaticPathE2eQueries,
			},
		},
		{
			name: "tracerouteQueries and e2eQueries from instance config",
			rawInstance: []byte(`
hostname: 1.2.3.4
traceroute_queries: 5
e2e_queries: 100
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     5,
				E2eQueries:            100,
			},
		},
		{
			name: "tracerouteQueries and e2eQueries from instance config preferred over init config",
			rawInstance: []byte(`
hostname: 1.2.3.4
traceroute_queries: 5
e2e_queries: 100
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
traceroute_queries: 2
e2e_queries: 2
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     5,
				E2eQueries:            100,
			},
		},
		{
			name: "tracerouteQueries and e2eQueries from init config",
			rawInstance: []byte(`
hostname: 1.2.3.4
min_collection_interval: 42
`),
			rawInitConfig: []byte(`
min_collection_interval: 10
traceroute_queries: 4
e2e_queries: 20
`),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(42) * time.Second,
				Namespace:             "my-namespace",
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     4,
				E2eQueries:            20,
			},
		},
		{
			name: "Disabling Windows driver",
			rawInstance: []byte(`
hostname: 1.2.3.4
protocol: tcp
disable_windows_driver: true
`),
			rawInitConfig: []byte(``),
			expectedConfig: &CheckConfig{
				DestHostname:          "1.2.3.4",
				MinCollectionInterval: time.Duration(60) * time.Second,
				Namespace:             "my-namespace",
				Protocol:              payload.ProtocolTCP,
				Timeout:               setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:     setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:            setup.DefaultNetworkPathStaticPathE2eQueries,
				DisableWindowsDriver:  true,
			},
		},
		{
			name: "Disable collecting source public IP",
			rawInstance: []byte(`
hostname: 1.2.3.4
disable_source_public_ip_collection: true
`),
			expectedConfig: &CheckConfig{
				DestHostname:                    "1.2.3.4",
				MinCollectionInterval:           time.Duration(60) * time.Second,
				Namespace:                       "my-namespace",
				Timeout:                         setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                          setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:               setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:                      setup.DefaultNetworkPathStaticPathE2eQueries,
				DisableSourcePublicIPCollection: true,
			},
		},
		{
			name: "Disable collecting source public IP from init config",
			rawInstance: []byte(`
hostname: 1.2.3.4
`),
			rawInitConfig: []byte(`
disable_source_public_ip_collection: true
`),
			expectedConfig: &CheckConfig{
				DestHostname:                    "1.2.3.4",
				MinCollectionInterval:           time.Duration(60) * time.Second,
				Namespace:                       "my-namespace",
				Timeout:                         setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                          setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:               setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:                      setup.DefaultNetworkPathStaticPathE2eQueries,
				DisableSourcePublicIPCollection: true,
			},
		},
		{
			name: "Disable collecting source public IP from init config cannot be re-enabled per instance",
			rawInstance: []byte(`
hostname: 1.2.3.4
disable_source_public_ip_collection: false
`),
			rawInitConfig: []byte(`
disable_source_public_ip_collection: true
`),
			expectedConfig: &CheckConfig{
				DestHostname:                    "1.2.3.4",
				MinCollectionInterval:           time.Duration(60) * time.Second,
				Namespace:                       "my-namespace",
				Timeout:                         setup.DefaultNetworkPathTimeout * time.Millisecond,
				MaxTTL:                          setup.DefaultNetworkPathMaxTTL,
				TracerouteQueries:               setup.DefaultNetworkPathStaticPathTracerouteQueries,
				E2eQueries:                      setup.DefaultNetworkPathStaticPathE2eQueries,
				DisableSourcePublicIPCollection: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := NewCheckConfig(tt.rawInstance, tt.rawInitConfig)
			assert.Equal(t, tt.expectedConfig, config)
			if tt.expectedError != "" {
				assert.ErrorContains(t, err, tt.expectedError)
			}
		})
	}
}

type fakeTraceroute struct {
	path payload.NetworkPath
}

func (f *fakeTraceroute) Run(context.Context, tracerouteconfig.Config) (payload.NetworkPath, error) {
	return f.path, nil
}

func TestRunSetsTestConfigIDInPayload(t *testing.T) {
	configmock.New(t).SetInTest("network_devices.namespace", "my-namespace")

	rawInstance := integration.Data(`
test_config_id: test-config-a
hostname: api.example.com
source_service: frontend
destination_service: api
tags:
  - env:prod
`)
	rawInitConfig := integration.Data{}
	expectedID := checkid.BuildID(CheckName, integration.FakeConfigHash, rawInstance, rawInitConfig)
	sender := mocksender.NewMockSender(t, expectedID)
	sender.SetupAcceptAll()

	check := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		traceroute: &fakeTraceroute{path: payload.NetworkPath{
			Destination: payload.NetworkPathDestination{
				Hostname: "api.example.com",
				Port:     443,
			},
		}},
	}
	err := check.Configure(sender.GetSenderManager(), integration.FakeConfigHash, rawInstance, rawInitConfig, "network-path-remote-config:scheduled[0]", names.NetworkPathRemoteConfig)
	assert.NoError(t, err)

	err = check.Run()
	assert.NoError(t, err)

	sender.AssertCalled(t, "EventPlatformEvent", mock.MatchedBy(func(raw []byte) bool {
		var path payload.NetworkPath
		if err := json.Unmarshal(raw, &path); err != nil {
			return false
		}
		return path.TestConfigID == "test-config-a" &&
			path.Namespace == "my-namespace" &&
			path.Origin == payload.PathOriginNetworkPathIntegration &&
			path.TestRunType == payload.TestRunTypeScheduled &&
			path.TestConfigSource == payload.TestConfigSourceRemote &&
			path.SourceProduct == payload.SourceProductNetworkPath &&
			path.CollectorType == payload.CollectorTypeAgent &&
			path.Source.Service == "frontend" &&
			path.Destination.Service == "api" &&
			assert.ObjectsAreEqual([]string{"env:prod"}, path.Tags)
	}), eventplatform.EventTypeNetworkPath)
}

func TestConfigureSetsTestConfigSourceFromProvider(t *testing.T) {
	tests := []struct {
		name                 string
		provider             string
		expectedConfigSource payload.TestConfigSource
		expectedTestConfigID string
	}{
		{
			name:                 "file",
			provider:             names.File,
			expectedConfigSource: payload.TestConfigSourceOther,
		},
		{
			name:                 "container",
			provider:             names.Container,
			expectedConfigSource: payload.TestConfigSourceOther,
		},
		{
			name:                 "kubernetes",
			provider:             names.Kubernetes,
			expectedConfigSource: payload.TestConfigSourceOther,
		},
		{
			name:                 "kubernetes container",
			provider:             names.KubeContainer,
			expectedConfigSource: payload.TestConfigSourceOther,
		},
		{
			name:                 "network path remote config",
			provider:             names.NetworkPathRemoteConfig,
			expectedConfigSource: payload.TestConfigSourceRemote,
			expectedTestConfigID: "test-config-a",
		},
		{
			name:                 "generic remote config",
			provider:             names.RemoteConfig,
			expectedConfigSource: payload.TestConfigSourceOther,
		},
		{
			name:                 "unknown",
			provider:             "unknown",
			expectedConfigSource: payload.TestConfigSourceOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rawInstance := integration.Data(`
test_config_id: test-config-a
hostname: api.example.com
`)
			check := &Check{CheckBase: core.NewCheckBase(CheckName)}

			err := check.Configure(nil, integration.FakeConfigHash, rawInstance, integration.Data{}, "test source", tt.provider)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedTestConfigID, check.config.TestConfigID)
			assert.Equal(t, tt.expectedConfigSource, check.testConfigSource)
		})
	}
}

func TestRunSetsOtherTestConfigSourceInPayload(t *testing.T) {
	rawInstance := integration.Data("hostname: api.example.com")
	rawInitConfig := integration.Data{}
	expectedID := checkid.BuildID(CheckName, integration.FakeConfigHash, rawInstance, rawInitConfig)
	sender := mocksender.NewMockSender(t, expectedID)
	sender.SetupAcceptAll()

	check := &Check{
		CheckBase: core.NewCheckBase(CheckName),
		traceroute: &fakeTraceroute{path: payload.NetworkPath{
			Destination: payload.NetworkPathDestination{Hostname: "api.example.com"},
		}},
	}
	assert.NoError(t, check.Configure(sender.GetSenderManager(), integration.FakeConfigHash, rawInstance, rawInitConfig, "file:network_path.yaml[0]", names.File))
	assert.NoError(t, check.Run())

	sender.AssertCalled(t, "EventPlatformEvent", mock.MatchedBy(func(raw []byte) bool {
		var path payload.NetworkPath
		return json.Unmarshal(raw, &path) == nil && path.TestConfigSource == payload.TestConfigSourceOther
	}), eventplatform.EventTypeNetworkPath)
}

func TestFirstNonZero(t *testing.T) {
	tests := []struct {
		name          string
		values        []time.Duration
		expectedValue time.Duration
	}{
		{
			name:          "no value",
			expectedValue: 0,
		},
		{
			name: "one value",
			values: []time.Duration{
				time.Duration(10) * time.Second,
			},
			expectedValue: time.Duration(10) * time.Second,
		},
		{
			name: "multiple values - select first",
			values: []time.Duration{
				time.Duration(10) * time.Second,
				time.Duration(20) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(10) * time.Second,
		},
		{
			name: "multiple values - select second",
			values: []time.Duration{
				time.Duration(0) * time.Second,
				time.Duration(20) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(20) * time.Second,
		},
		{
			name: "multiple values - select third",
			values: []time.Duration{
				time.Duration(0) * time.Second,
				time.Duration(0) * time.Second,
				time.Duration(30) * time.Second,
			},
			expectedValue: time.Duration(30) * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedValue, firstNonZero(tt.values...))
		})
	}
}
