// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package connectionscheckimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigdef "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	npcollectormock "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/mock"
	connectionscheck "github.com/DataDog/datadog-agent/comp/process/connectionscheck/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
				"network_config.enabled":     true,
				"network_config.direct_send": false,
			},
			flavor:  flavor.ProcessAgent,
			enabled: true,
		},
		{
			name: "SysProbe enabled and running on the process-agent",
			sysprobeConfigs: map[string]interface{}{
				"system_probe_config.enabled": true,
				"network_config.direct_send":  false,
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

			sysprobeConf := sysprobeconfigmock.NewMockWithOverrides(t, tc.sysprobeConfigs)
			c := fxutil.Test[connectionscheck.Component](t, fx.Options(
				fx.Provide(func(t testing.TB) config.Component { return config.NewMock(t) }),
				fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
				fx.Provide(func() sysprobeconfigdef.Component { return sysprobeConf }),
				workloadmetafxmock.MockModule(workloadmeta.NewParams()),
				npcollectormock.MockModule(),
				fx.Provide(func(t testing.TB) tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
				fxutil.ProvideComponentConstructor(NewComponent),
			))

			flavor.SetFlavor(tc.flavor)
			assert.Equal(t, tc.enabled, c.Object().IsEnabled())
		})
	}
}
