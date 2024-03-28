// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test && trivy

package sbom

import (
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
	"testing"

	"github.com/stretchr/testify/assert"
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
	cfg := fxutil.Test[config.Component](t, config.MockModule())
	mockStore := fxutil.Test[workloadmeta.Mock](t, fx.Options(
		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.MockModuleV2(),
	))
	checkFactory := Factory(mockStore, cfg)
	assert.NotNil(t, checkFactory)

	check, ok := checkFactory.Get()
	assert.True(t, ok)
	assert.NotNil(t, check)
}
