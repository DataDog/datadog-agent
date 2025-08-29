// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

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
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	gpusubscriberfxmock "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/fx-mock"
	"github.com/DataDog/datadog-agent/comp/process/processcheck"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestProcessCheckEnablementOnCoreAgent Tests the process checks run on the core agent only
func TestProcessCheckEnablementOnCoreAgent(t *testing.T) {
	originalFlavor := flavor.GetFlavor()
	defer flavor.SetFlavor(originalFlavor)

	tests := []struct {
		name    string
		flavor  string
		enabled bool
	}{
		{
			name:    "Process check should not run in the process agent when run_in_core_agent is enabled",
			flavor:  flavor.ProcessAgent,
			enabled: false,
		},
		{
			name:    "Process check runs in the core agent when run_in_core_agent is enabled",
			flavor:  flavor.DefaultAgent,
			enabled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configs := map[string]interface{}{
				"process_config.process_collection.enabled": true,
				"process_config.run_in_core_agent.enabled":  true,
			}

			flavor.SetFlavor(tc.flavor)
			c := fxutil.Test[processcheck.Component](t, fx.Options(
				fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
				fx.Provide(func(t testing.TB) config.Component { return config.NewMockWithOverrides(t, configs) }),
				sysprobeconfigimpl.MockModule(),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				gpusubscriberfxmock.MockModule(),
				fx.Provide(func() statsd.ClientInterface {
					return &statsd.NoOpClient{}
				}),
				Module(),
				fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			))
			assert.Equal(t, tc.enabled, c.Object().IsEnabled())
		})
	}
}
