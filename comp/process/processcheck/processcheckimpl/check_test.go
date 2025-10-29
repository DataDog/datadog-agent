// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processcheckimpl

import (
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	taggerfxnoop "github.com/DataDog/datadog-agent/comp/core/tagger/fx-noop"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	gpusubscriberfxmock "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/fx-mock"
	"github.com/DataDog/datadog-agent/comp/process/processcheck"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestProcessChecksIsEnabled(t *testing.T) {
	tests := []struct {
		name            string
		configs         map[string]interface{}
		sysProbeConfigs map[string]interface{}
		enabled         bool
	}{
		{
			name: "check enabled: collection enabled, discovery enabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled": true,
			},
			sysProbeConfigs: map[string]interface{}{
				"discovery.enabled": true,
			},
			enabled: true,
		},
		{
			name: "check enabled: collection enabled, discovery disabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled": true,
			},
			sysProbeConfigs: map[string]interface{}{
				"discovery.enabled": false,
			},
			enabled: true,
		},
		{
			name: "check enabled: collection disabled, discovery enabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled": false,
			},
			sysProbeConfigs: map[string]interface{}{
				"discovery.enabled": true,
			},
			enabled: true,
		},
		{
			name: "check disabled: collection disabled, discovery disabled",
			configs: map[string]interface{}{
				"process_config.process_collection.enabled": false,
			},
			sysProbeConfigs: map[string]interface{}{
				"discovery.enabled": false,
			},
			enabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := fxutil.Test[processcheck.Component](t, fx.Options(
				fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
				fx.Provide(func(t testing.TB) config.Component { return config.NewMockWithOverrides(t, tc.configs) }),
				sysprobeconfigimpl.MockModule(),
				fx.Replace(sysprobeconfigimpl.MockParams{Overrides: tc.sysProbeConfigs}),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				gpusubscriberfxmock.MockModule(),
				taggerfxnoop.Module(),
				fx.Provide(func() statsd.ClientInterface {
					return &statsd.NoOpClient{}
				}),
				fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
				Module(),
			))
			assert.Equal(t, tc.enabled, c.Object().IsEnabled())
		})
	}
}
