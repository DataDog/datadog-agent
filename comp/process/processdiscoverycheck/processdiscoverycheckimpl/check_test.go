// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processdiscoverycheckimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/comp/process/processdiscoverycheck"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestProcessDiscoveryIsEnabled(t *testing.T) {
	tests := []struct {
		name            string
		configs         map[string]interface{}
		sysProbeConfigs map[string]interface{}
		enabled         bool
	}{
		{
			name: "enabled",
			configs: map[string]interface{}{
				"process_config.process_discovery.enabled": true,
			},
			sysProbeConfigs: map[string]interface{}{},
			enabled:         true,
		},
		{
			name: "disabled",
			configs: map[string]interface{}{
				"process_config.process_discovery.enabled": false,
			},
			sysProbeConfigs: map[string]interface{}{},
			enabled:         false,
		},
		{
			name: "process collection disables the process discovery check",
			configs: map[string]interface{}{
				"process_config.process_discovery.enabled":  true,
				"process_config.process_collection.enabled": true,
			},
			sysProbeConfigs: map[string]interface{}{},
			enabled:         false,
		},
		{
			name: "service discovery disables the process discovery check",
			configs: map[string]interface{}{
				"process_config.process_discovery.enabled": true,
			},
			sysProbeConfigs: map[string]interface{}{
				"discovery.enabled": true,
			},
			enabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := fxutil.Test[processdiscoverycheck.Component](t, fx.Options(
				fx.Provide(func(t testing.TB) config.Component { return config.NewMockWithOverrides(t, tc.configs) }),
				sysprobeconfigimpl.MockModule(),
				fx.Replace(sysprobeconfigimpl.MockParams{Overrides: tc.sysProbeConfigs}),
				Module(),
			))
			assert.Equal(t, tc.enabled, c.Object().IsEnabled())
		})
	}
}
