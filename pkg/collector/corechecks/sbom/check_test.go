// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test && trivy && linux

package sbom

import (
	"testing"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	scanner2 "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"
)

func TestConfigParsing(t *testing.T) {
	for _, tt := range []struct {
		name     string
		raw      string
		expected Config
	}{
		{
			name: "default values",
			raw:  ``,
			expected: Config{
				ChunkSize:                       1,
				NewSBOMMaxLatencySeconds:        30,
				ContainerPeriodicRefreshSeconds: 3600,
				HostPeriodicRefreshSeconds:      3600,
				HostHeartbeatValiditySeconds:    3600 * 24,
			},
		},
		{
			name: "custom values",
			raw: `
chunk_size: 10
new_sbom_max_latency_seconds: 120
periodic_refresh_seconds: 3600
host_periodic_refresh_seconds: 7200
host_heartbeat_validity_seconds: 86400
`,
			expected: Config{
				ChunkSize:                       10,
				NewSBOMMaxLatencySeconds:        120,
				ContainerPeriodicRefreshSeconds: 3600,
				HostPeriodicRefreshSeconds:      7200,
				HostHeartbeatValiditySeconds:    86400,
			},
		},
		{
			name: "invalid values",
			raw: `
chunk_size: -10
new_sbom_max_latency_seconds: -120
periodic_refresh_seconds: -3600
host_periodic_refresh_seconds: -7200
host_heartbeat_validity_seconds: -86400
`,
			expected: Config{
				ChunkSize:                       1,
				NewSBOMMaxLatencySeconds:        1,
				ContainerPeriodicRefreshSeconds: 60,
				HostPeriodicRefreshSeconds:      60,
				HostHeartbeatValiditySeconds:    60,
			},
		},
		{
			name: "exceeding max values",
			raw: `
chunk_size: 1000
new_sbom_max_latency_seconds: 10000
periodic_refresh_seconds: 1000000
host_periodic_refresh_seconds: 1000000
host_heartbeat_validity_seconds: 1000000
`,

			expected: Config{
				ChunkSize:                       100,
				NewSBOMMaxLatencySeconds:        300,
				ContainerPeriodicRefreshSeconds: 604800,
				HostPeriodicRefreshSeconds:      604800,
				HostHeartbeatValiditySeconds:    604800,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got Config
			err := got.Parse([]byte(tt.raw))
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestFactory(t *testing.T) {
	cfg := config.NewMock(t)
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	mockStore := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	checkFactory := Factory(mockStore, cfg, fakeTagger)
	assert.NotNil(t, checkFactory)

	check, ok := checkFactory.Get()
	assert.True(t, ok)
	assert.NotNil(t, check)
}

type mockSenderManager struct {
	mock.Mock
}

func (m *mockSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	args := m.Called(id)
	return args.Get(0).(sender.Sender), args.Error(1)
}

func (m *mockSenderManager) SetSender(sender.Sender, checkid.ID) error {
	args := m.Called()
	return args.Error(0)
}
func (m *mockSenderManager) DestroySender(id checkid.ID) {
	m.Called(id)
}
func (m *mockSenderManager) GetDefaultSender() (sender.Sender, error) {
	args := m.Called()
	return args.Get(0).(sender.Sender), args.Error(1)
}

type workloadmetaAndConfig struct {
	fx.In

	Store workloadmeta.Component
	Cfg   config.Component
}

func TestConfigure(t *testing.T) {
	// Workloadmeta initializes the scanner. The check does not start if the scanner is not initialized
	// TODO(Components): Provide a way to inject a scanner to the workloadmeta component
	scanner := scanner2.GetGlobalScanner()
	defer scanner2.SetGlobalScanner(scanner)

	app := fxutil.Test[workloadmetaAndConfig](t, fx.Options(
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{
				"sbom.enabled":      true,
				"sbom.host.enabled": true,
			},
		}),
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.Params{
			AgentType:  workloadmeta.NodeAgent,
			InitHelper: common.GetWorkloadmetaInit(),
		}),
	))
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	cfg := app.Cfg
	mockStore := app.Store

	checkFactory := Factory(mockStore, cfg, fakeTagger)
	assert.NotNil(t, checkFactory)

	check, ok := checkFactory.Get()
	assert.True(t, ok)
	assert.NotNil(t, check)

	senderManager := &mockSenderManager{}
	s := &mocksender.MockSender{}
	s.SetupAcceptAll()
	senderManager.On("GetSender", mock.Anything).Return(s, nil)
	c := check()
	assert.NotNil(t, c)

	err := c.Configure(senderManager,
		123,
		integration.Data{},
		integration.Data{},
		"source",
	)
	assert.NoError(t, err)
}
