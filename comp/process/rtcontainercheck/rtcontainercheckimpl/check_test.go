// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package rtcontainercheckimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/process/rtcontainercheck"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestRTContainerCheckIsEnabled(t *testing.T) {
	tests := []struct {
		name             string
		configs          map[string]interface{}
		sysProbeConfigs  map[string]interface{}
		containerizedEnv bool
		flavor           string
		enabled          bool
	}{
		{
			// the container collection is enabled by default in containerized environments
			name:             "empty config - container collection is enabled",
			configs:          nil,
			containerizedEnv: true,
			enabled:          true,
		},
		{
			name: "container collection is disabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled":   false,
				"process_config.container_collection.enabled": false,
			},
			containerizedEnv: true,
			enabled:          false,
		},
		{
			name: "config is enabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled":   false,
				"process_config.container_collection.enabled": true,
			},
			containerizedEnv: true,
			enabled:          true,
		},
		{
			name: "rt collection explicitly disabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled":   false,
				"process_config.container_collection.enabled": true,
				"process_config.disable_realtime_checks":      true,
			},
			containerizedEnv: true,
			enabled:          false,
		},
		{
			name: "process check is enabled and also collects containers so the standalone container check is disabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled":   true,
				"process_config.container_collection.enabled": true,
			},
			containerizedEnv: true,
			enabled:          false,
		},
		{
			name: "check disabled as it is not in a container env",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled":   false,
				"process_config.container_collection.enabled": true,
			},
			containerizedEnv: false,
			enabled:          false,
		},
		{
			name: "check is disabled in the process-agent as run in core agent is enabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled":   false,
				"process_config.container_collection.enabled": true,
				"process_config.run_in_core_agent.enabled":    true,
			},

			containerizedEnv: true,
			flavor:           flavor.ProcessAgent,
			enabled:          false,
		},
		{
			name: "check is enabled in the core agent as run in core agent is enabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled":   false,
				"process_config.container_collection.enabled": true,
				"process_config.run_in_core_agent.enabled":    true,
			},

			containerizedEnv: true,
			flavor:           flavor.DefaultAgent,
			enabled:          true,
		},
		{
			name: "service discovery disables the real-time container check",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled":   false,
				"process_config.container_collection.enabled": true,
			},
			sysProbeConfigs: map[string]interface{}{
				"discovery.enabled": true,
			},
			enabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			originalFlavor := flavor.GetFlavor()
			defer flavor.SetFlavor(originalFlavor)

			if tc.containerizedEnv {
				env.SetFeatures(t, env.Docker)
			}
			if tc.flavor != "" {
				flavor.SetFlavor(tc.flavor)
			}

			c := fxutil.Test[rtcontainercheck.Component](t, fx.Options(
				fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
				fx.Provide(func(t testing.TB) config.Component { return config.NewMockWithOverrides(t, tc.configs) }),
				sysprobeconfigimpl.MockModule(),
				fx.Replace(sysprobeconfigimpl.MockParams{Overrides: tc.sysProbeConfigs}),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				fx.Provide(func() statsd.ClientInterface {
					return &statsd.NoOpClient{}
				}),
				Module(),
			))

			assert.Equal(t, tc.enabled, c.Object().IsEnabled())
		})
	}
}
