// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package connectionscheckimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/npcollectorimpl"
	"github.com/DataDog/datadog-agent/comp/process/connectionscheck"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func TestConnectionsCheckIsEnabled(t *testing.T) {
	tests := []struct {
		name            string
		sysprobeConfigs map[string]interface{}
		flavor          string
		enabled         bool
	}{
		{
			name: "Network check enabled and running on the process-agent",
			sysprobeConfigs: map[string]interface{}{
				"network_config.enabled": true,
			},
			flavor:  flavor.ProcessAgent,
			enabled: true,
		},
		{
			name: "SysProbe enabled and running on the process-agent",
			sysprobeConfigs: map[string]interface{}{
				"system_probe_config.enabled": true,
			},
			flavor:  flavor.ProcessAgent,
			enabled: true,
		},
		{
			name: "disabled in the process-agent",
			sysprobeConfigs: map[string]interface{}{
				"network_config.enabled": false,
			},
			flavor:  flavor.ProcessAgent,
			enabled: false,
		},
		{
			name: "the check should not be enabled when in the core agent",
			sysprobeConfigs: map[string]interface{}{
				"network_config.enabled": true,
			},
			flavor:  flavor.DefaultAgent,
			enabled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			originalFlavor := flavor.GetFlavor()
			defer flavor.SetFlavor(originalFlavor)

			c := fxutil.Test[connectionscheck.Component](t, fx.Options(
				core.MockBundle(),
				fx.Replace(sysprobeconfigimpl.MockParams{Overrides: tc.sysprobeConfigs}),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				npcollectorimpl.MockModule(),
				Module(),
			))

			flavor.SetFlavor(tc.flavor)
			assert.Equal(t, tc.enabled, c.Object().IsEnabled())
		})
	}
}
