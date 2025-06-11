// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profilerimpl

import (
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	configcomponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	profilerdef "github.com/DataDog/datadog-agent/comp/core/profiler/def"
	profilermock "github.com/DataDog/datadog-agent/comp/core/profiler/mock"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"

	"github.com/DataDog/datadog-agent/comp/core"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func createGenericConfig(t *testing.T) model.Config {
	handler := profilermock.NewMockHandler()

	server := httptest.NewServer(handler)

	t.Cleanup(func() {
		if server != nil {
			server.Close()
			server = nil
		}
	})

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	port := u.Port()

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("expvar_port", port)
	mockConfig.SetWithoutSource("apm_config.debug.port", port)
	mockConfig.SetWithoutSource("process_config.expvar_port", port)
	mockConfig.SetWithoutSource("security_agent.expvar_port", port)

	mockConfig.SetWithoutSource("process_config.run_in_core_agent.enabled", false)
	mockConfig.SetWithoutSource("process_config.enabled", false)
	mockConfig.SetWithoutSource("process_config.container_collection.enabled", false)
	mockConfig.SetWithoutSource("process_config.process_collection.enabled", false)
	mockConfig.SetWithoutSource("apm_config.enabled", false)

	return mockConfig
}

type reqs struct {
	fx.In

	Comp profilerdef.Component
}

func getProfiler(t testing.TB, overrideConfig map[string]interface{}, overrideSysProbe map[string]interface{}) profiler {
	deps := fxutil.Test[reqs](
		t,
		core.MockBundle(),
		fx.Replace(configcomponent.MockParams{
			Overrides: overrideConfig,
		}),
		fx.Replace(sysprobeconfigimpl.MockParams{
			Overrides: overrideSysProbe,
		}),
		settingsimpl.MockModule(),
		fxutil.ProvideComponentConstructor(NewComponent),
		fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
		fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
	)

	return deps.Comp.(profiler)
}

func TestProfileSetting(t *testing.T) {

	scenarios := []struct {
		name     string
		newVal   int
		oldVal   int
		expVal   int
		expDefer bool
	}{
		{
			name:     "Base case - no change",
			newVal:   0,
			oldVal:   10,
			expVal:   10,
			expDefer: false,
		},
		{
			name:     "Same value - no change",
			newVal:   10,
			oldVal:   10,
			expVal:   10,
			expDefer: false,
		},
		{
			name:     "Overwrite value",
			newVal:   20,
			oldVal:   10,
			expVal:   20,
			expDefer: true,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			fb := helpers.NewFlareBuilderMockWithArgs(t, true, types.FlareArgs{})
			profiler := getProfiler(t, map[string]interface{}{}, map[string]interface{}{})
			profiler.settingsComponent.SetRuntimeSetting("runtime_block_profile_rate", s.oldVal, model.SourceDefault)

			deferFunc := profiler.setProfilerSetting("runtime_block_profile_rate", s.newVal, fb)
			curVal, _ := profiler.settingsComponent.GetRuntimeSetting("runtime_block_profile_rate")
			assert.Equal(t, s.expVal, curVal)

			if s.expDefer {
				assert.NotNil(t, deferFunc)
			} else {
				assert.Nil(t, deferFunc)
			}
			if deferFunc != nil {
				deferFunc()
			}

			curVal, _ = profiler.settingsComponent.GetRuntimeSetting("runtime_block_profile_rate")
			assert.Equal(t, s.oldVal, curVal)
		})
	}
}

func TestTimeout(t *testing.T) {
	baseTimeout := 10 * time.Minute

	scenarios := []struct {
		name            string
		extraCfgs       map[string]interface{}
		extraSysCfgs    map[string]interface{}
		profileDuration time.Duration
		expTimeout      time.Duration
	}{
		{
			name:            "Base Enabled Case",
			extraCfgs:       map[string]interface{}{},
			extraSysCfgs:    map[string]interface{}{},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 4*(10*time.Second),
		},
		{
			name:            "Base Disabled Case",
			extraCfgs:       map[string]interface{}{},
			extraSysCfgs:    map[string]interface{}{},
			profileDuration: 0,
			expTimeout:      0,
		},
		{
			name: "APM Enabled, Default Runtime",
			extraCfgs: map[string]interface{}{
				"apm_config.enabled": true,
			},
			extraSysCfgs:    map[string]interface{}{},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 4*(10*time.Second) + 2*(4*time.Second), // APM default runtime has a ceiling of 4
		},
		{
			name: "APM Enabled, Small Runtime",
			extraCfgs: map[string]interface{}{
				"apm_config.enabled":          true,
				"apm_config.receiver_timeout": 20,
			},
			extraSysCfgs:    map[string]interface{}{},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 6*(10*time.Second), // APM timeout is floored to the profile duration
		},
		{
			name: "APM Enabled, Large Runtime",
			extraCfgs: map[string]interface{}{
				"apm_config.enabled":          true,
				"apm_config.receiver_timeout": 5,
			},
			extraSysCfgs:    map[string]interface{}{},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 4*(10*time.Second) + 2*(5*time.Second), // APM timeout is the ceiling, limiting profile duration
		},
		{
			name: "Process Agent Enabled",
			extraCfgs: map[string]interface{}{
				"process_config.enabled": true,
			},
			extraSysCfgs:    map[string]interface{}{},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 6*(10*time.Second),
		},
		{
			name: "Process Agent Enabled, Alternate Setting",
			extraCfgs: map[string]interface{}{
				"process_config.process_collection.enabled": true,
			},
			extraSysCfgs:    map[string]interface{}{},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 6*(10*time.Second),
		},
		{
			name: "Process Agent Checks in Core Agent",
			extraCfgs: map[string]interface{}{
				"process_config.run_in_core_agent.enabled": true,
			},
			extraSysCfgs:    map[string]interface{}{},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 4*(10*time.Second),
		},
		{
			name: "Process Agent Enabled, via NPM",
			extraCfgs: map[string]interface{}{
				"process_config.run_in_core_agent.enabled": true,
			},
			extraSysCfgs: map[string]interface{}{
				"network_config.enabled": true,
			},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 8*(10*time.Second),
		},
		{
			name: "Process Agent Enabled, via USM",
			extraCfgs: map[string]interface{}{
				"process_config.run_in_core_agent.enabled": true,
			},
			extraSysCfgs: map[string]interface{}{
				"service_monitoring_config.enabled": true,
			},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 8*(10*time.Second),
		},
		{
			name:      "SysProbe Enabled",
			extraCfgs: map[string]interface{}{},
			extraSysCfgs: map[string]interface{}{
				"system_probe_config.enabled": true,
			},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 8*(10*time.Second), // config enables NPM, which enables process agent
		},
		{
			name: "Everything Enabled",
			extraCfgs: map[string]interface{}{
				"process_config.container_collection.enabled": true,
				"apm_config.enabled":                          true,
				"apm_config.receiver_timeout":                 10,
			},
			extraSysCfgs: map[string]interface{}{
				"system_probe_config.enabled": true,
			},
			profileDuration: 10 * time.Second,
			expTimeout:      baseTimeout + 10*(10*time.Second),
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			cfg := createGenericConfig(t)
			cfg.SetWithoutSource("flare.profile_overhead_runtime", baseTimeout)
			fArgs := types.FlareArgs{
				ProfileDuration: s.profileDuration,
			}
			for k, v := range s.extraCfgs {
				cfg.SetWithoutSource(k, v)
			}
			fb := helpers.NewFlareBuilderMockWithArgs(t, true, fArgs)
			profiler := getProfiler(t, cfg.AllSettings(), s.extraSysCfgs)

			timeout := profiler.timeout(fb)

			assert.Equal(t, s.expTimeout, timeout)
		})
	}
}
