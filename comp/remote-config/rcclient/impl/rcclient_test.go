// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

package rcclientimpl

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	settings "github.com/DataDog/datadog-agent/comp/core/settings/def"
	settingsfx "github.com/DataDog/datadog-agent/comp/core/settings/fx"
	settingsmock "github.com/DataDog/datadog-agent/comp/core/settings/mock"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

type mockLogLevelRuntimeSettings struct {
	cfg           config.Component
	expectedError error
	logLevel      string
}

func (m *mockLogLevelRuntimeSettings) Get(_ config.Component) (interface{}, error) {
	return m.logLevel, nil
}

func (m *mockLogLevelRuntimeSettings) Set(_ config.Component, v interface{}, source model.Source) error {
	if m.expectedError != nil {
		return m.expectedError
	}
	m.logLevel = v.(string)
	m.cfg.Set(m.Name(), m.logLevel, source)
	return nil
}

func (m *mockLogLevelRuntimeSettings) Name() string {
	return "log_level"
}

func (m *mockLogLevelRuntimeSettings) Description() string {
	return ""
}

func (m *mockLogLevelRuntimeSettings) Hidden() bool {
	return true
}

func applyEmpty(_ string, _ state.ApplyStatus) {}

type testDeps struct {
	fx.In
	Comp rcclient.Component
}

func TestRCClientCreate(t *testing.T) {
	// Test missing params — expect the fx app to fail to build
	_, _, err := fxutil.TestApp[testDeps](
		fxutil.ProvideComponentConstructor(NewComponent),
		fxutil.ProvideOptional[rcclient.Component](),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return configmock.New(t) }),
		settingsmock.MockModule(),
		sysprobeconfig.NoneModule(),
		fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
	)
	// Missing params
	assert.Error(t, err)

	// Test success case
	app, deps, err := fxutil.TestApp[testDeps](
		fxutil.ProvideComponentConstructor(NewComponent),
		fxutil.ProvideOptional[rcclient.Component](),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return configmock.New(t) }),
		sysprobeconfig.NoneModule(),
		fx.Supply(
			rcclient.Params{
				AgentName:    "test-agent",
				AgentVersion: "7.0.0",
			},
		),
		settingsmock.MockModule(),
		fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
	)
	assert.NoError(t, err)
	t.Cleanup(func() { app.Stop(context.Background()) }) //nolint:errcheck
	assert.NotNil(t, deps.Comp)
	assert.NotNil(t, deps.Comp.(*rcClient).client)
}

func TestAgentConfigCallback(t *testing.T) {
	pkglog.SetupLogger(pkglog.Default(), "info")
	cfg := configmock.New(t)

	var ipcComp ipc.Component

	rcComponent := fxutil.Test[rcclient.Component](t,
		fx.Options(
			fxutil.Component(
				fxutil.ProvideComponentConstructor(NewComponent),
				fxutil.ProvideOptional[rcclient.Component](),
			),
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return cfg }),
			sysprobeconfig.NoneModule(),
			fx.Supply(
				rcclient.Params{
					AgentName:    "test-agent",
					AgentVersion: "7.0.0",
				},
			),
			fx.Supply(
				settings.Params{
					Settings: map[string]settings.RuntimeSetting{
						"log_level": &mockLogLevelRuntimeSettings{cfg: cfg, logLevel: "info"},
					},
					Config: cfg,
				},
			),
			settingsfx.Module(),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Populate(&ipcComp),
		),
	)

	layerStartFlare := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": "debug"}}`)}
	layerEndFlare := state.RawConfig{Config: []byte(`{"name": "layer1", "config": {"log_level": ""}}`)}
	configOrder := state.RawConfig{Config: []byte(`{"internal_order": ["layer1", "layer2"]}`)}

	rc := rcComponent.(*rcClient)

	ipcAddress, err := pkgconfighelper.GetIPCAddress(cfg)
	assert.NoError(t, err)

	rc.client, _ = client.NewUnverifiedGRPCClient(
		ipcAddress,
		pkgconfighelper.GetIPCPort(pkgconfigsetup.Datadog()),
		ipcComp.GetAuthToken(),
		ipcComp.GetTLSClientConfig(),
		client.WithAgent("test-agent", "9.99.9"),
		client.WithProducts(state.ProductAgentConfig),
		client.WithPollInterval(time.Hour),
	)

	// -----------------
	// Test scenario #1: Agent Flare request by RC and the log level hadn't been changed by the user before
	assert.Equal(t, model.SourceDefault, cfg.GetSource("log_level"))

	// Set log level to debug
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceRC, cfg.GetSource("log_level"))

	// Send an empty log level request, as RC would at the end of the Agent Flare request
	// Should fallback to the default level
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "info", cfg.Get("log_level"))
	assert.Equal(t, model.SourceDefault, cfg.GetSource("log_level"))

	// -----------------
	// Test scenario #2: log level was changed by the user BEFORE Agent Flare request
	cfg.Set("log_level", "info", model.SourceCLI)
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	// Log level should still be "info" because it was enforced by the user
	assert.Equal(t, "info", cfg.Get("log_level"))
	// Source should still be CLI as it has priority over RC
	assert.Equal(t, model.SourceCLI, cfg.GetSource("log_level"))

	// -----------------
	// Test scenario #3: log level is changed by the user DURING the Agent Flare request
	cfg.UnsetForSource("log_level", model.SourceCLI)
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerStartFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceRC, cfg.GetSource("log_level"))

	cfg.Set("log_level", "debug", model.SourceCLI)
	rc.agentConfigUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_CONFIG/layer1/configname":              layerEndFlare,
		"datadog/2/AGENT_CONFIG/configuration_order/configname": configOrder,
	}, applyEmpty)
	assert.Equal(t, "debug", cfg.Get("log_level"))
	assert.Equal(t, model.SourceCLI, cfg.GetSource("log_level"))
}

func TestAgentMRFConfigCallback(t *testing.T) {
	pkglog.SetupLogger(pkglog.Default(), "info")
	cfg := configmock.New(t)

	var ipcComp ipc.Component
	var settingsComp settings.Component

	rcComponent := fxutil.Test[rcclient.Component](t,
		fx.Options(
			fxutil.Component(
				fxutil.ProvideComponentConstructor(NewComponent),
				fxutil.ProvideOptional[rcclient.Component](),
			),
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() config.Component { return cfg }),
			sysprobeconfig.NoneModule(),
			fx.Supply(
				rcclient.Params{
					AgentName:    "test-agent",
					AgentVersion: "7.0.0",
				},
			),
			settingsmock.MockModule(),
			fx.Provide(func() ipc.Component { return ipcmock.New(t) }),
			fx.Populate(&ipcComp),
			fx.Populate(&settingsComp),
		),
	)

	allInactive := state.RawConfig{Config: []byte(`{"name": "none"}`)}
	noLogs := state.RawConfig{Config: []byte(`{"name": "nologs", "failover_logs": false}`)}
	noServiceChecks := state.RawConfig{Config: []byte(`{"name": "noservicechecks", "failover_service_checks": false}`)}
	activeMetrics := state.RawConfig{Config: []byte(`{"name": "yesmetrics", "failover_metrics": true}`)}
	activeAPM := state.RawConfig{Config: []byte(`{"name": "yesapm", "failover_apm": true}`)}
	activeAllowlist := state.RawConfig{Config: []byte(`{"name": "yesallowlist", "metrics_allowlist": ["system.cpu.usage"]}`)}
	emptyAllowlist := state.RawConfig{Config: []byte(`{"name": "emptyallowlist", "metrics_allowlist": []}`)}
	nilAllowlist := state.RawConfig{Config: []byte(`{"name": "nilallowlist"}`)}
	activeServices := state.RawConfig{Config: []byte(`{"name": "yesservices", "logs_service_allowlist": ["web", "api"]}`)}
	moreServices := state.RawConfig{Config: []byte(`{"name": "moreservices", "logs_service_allowlist": ["api", "worker"]}`)}
	emptyServices := state.RawConfig{Config: []byte(`{"name": "emptyservices", "logs_service_allowlist": []}`)}

	rc := rcComponent.(*rcClient)

	ipcAddress, err := pkgconfighelper.GetIPCAddress(cfg)
	assert.NoError(t, err)

	rc.client, _ = client.NewUnverifiedGRPCClient(
		ipcAddress,
		pkgconfighelper.GetIPCPort(pkgconfigsetup.Datadog()),
		ipcComp.GetAuthToken(),
		ipcComp.GetTLSClientConfig(),
		client.WithAgent("test-agent", "9.99.9"),
		client.WithProducts(state.ProductAgentConfig),
		client.WithPollInterval(time.Hour),
	)

	// Should enable metrics failover, disable service checks and logs failover,
	// set the metrics allowlist and merge the logs service allowlists
	rc.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/none/configname":            allInactive,
		"datadog/2/AGENT_FAILOVER/nologs/configname":          noLogs,
		"datadog/2/AGENT_FAILOVER/noservicechecks/configname": noServiceChecks,
		"datadog/2/AGENT_FAILOVER/yesmetrics/configname":      activeMetrics,
		"datadog/2/AGENT_FAILOVER/yesapm/configname":          activeAPM,
		"datadog/2/AGENT_FAILOVER/yesallowlist/configname":    activeAllowlist,
		"datadog/2/AGENT_FAILOVER/yesservices/configname":     activeServices,
		"datadog/2/AGENT_FAILOVER/moreservices/configname":    moreServices,
	}, applyEmpty)

	metricsVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.failover_metrics")
	serviceChecksVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.failover_service_checks")
	logsVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.failover_logs")
	apmVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.failover_apm")
	allowlistVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.metric_allowlist")
	servicesVal, _ := settingsComp.GetRuntimeSetting("multi_region_failover.logs_service_allowlist")
	assert.True(t, metricsVal.(bool))
	assert.False(t, serviceChecksVal.(bool))
	assert.False(t, logsVal.(bool))
	assert.True(t, apmVal.(bool))
	assert.ElementsMatch(t, []string{"system.cpu.usage"}, allowlistVal.([]string))
	assert.Equal(t, []string{"api", "web", "worker"}, servicesVal)

	// Should set empty allowlists
	rc.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/yesallowlist/configname": emptyAllowlist,
		"datadog/2/AGENT_FAILOVER/yesmetrics/configname":   activeMetrics,
		"datadog/2/AGENT_FAILOVER/yesservices/configname":  emptyServices,
	}, applyEmpty)

	metricsVal, _ = settingsComp.GetRuntimeSetting("multi_region_failover.failover_metrics")
	allowlistVal, _ = settingsComp.GetRuntimeSetting("multi_region_failover.metric_allowlist")
	servicesVal, _ = settingsComp.GetRuntimeSetting("multi_region_failover.logs_service_allowlist")
	assert.True(t, metricsVal.(bool))
	assert.ElementsMatch(t, []string{}, allowlistVal.([]string))
	assert.Equal(t, []string{}, servicesVal)

	// Should not set an allowlist (nil means not configured, so we fallback)
	// First, let's set a new mock to verify allowlist is not set
	settingsComp2 := fxutil.Test[settings.Component](t, settingsmock.MockModule())
	rc.settingsComponent = settingsComp2
	rc.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/emptyallowlist/configname": nilAllowlist,
		"datadog/2/AGENT_FAILOVER/yesmetrics/configname":     activeMetrics,
	}, applyEmpty)

	metricsVal, _ = settingsComp2.GetRuntimeSetting("multi_region_failover.failover_metrics")
	allowlistVal, _ = settingsComp2.GetRuntimeSetting("multi_region_failover.metric_allowlist")
	servicesVal, _ = settingsComp2.GetRuntimeSetting("multi_region_failover.logs_service_allowlist")
	assert.True(t, metricsVal.(bool))
	assert.Nil(t, allowlistVal)
	assert.Nil(t, servicesVal)
}

// recordingSettings records the runtime settings set through it.
type recordingSettings struct {
	settings.Component
	events *[]string
}

func (s *recordingSettings) SetRuntimeSetting(setting string, value interface{}, source model.Source) error {
	*s.events = append(*s.events, "set "+setting)
	return s.Component.SetRuntimeSetting(setting, value, source)
}

// Logs failover must never be enabled with the fallback logs service allowlist, which forwards every log.
func TestAgentMRFLogsServiceAllowlistOrdering(t *testing.T) {
	cfg := configmock.New(t)
	var events []string
	cfg.OnUpdate(func(setting string, _ model.Source, _, _ any, _ uint64, _ model.Source) {
		events = append(events, "update "+setting)
	})
	rc := &rcClient{settingsComponent: &recordingSettings{
		Component: fxutil.Test[settings.Component](t, settingsmock.MockModule()),
		events:    &events,
	}}

	activeLogs := state.RawConfig{Config: []byte(`{"name": "yeslogs", "failover_logs": true}`)}
	noLogs := state.RawConfig{Config: []byte(`{"name": "nologs", "failover_logs": false}`)}
	services := state.RawConfig{Config: []byte(`{"name": "services", "logs_service_allowlist": ["web"]}`)}

	// Enabling logs failover: the allowlist is set first
	rc.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/yeslogs/configname":  activeLogs,
		"datadog/2/AGENT_FAILOVER/services/configname": services,
	}, applyEmpty)
	assert.Equal(t, []string{"set " + logsServiceAllowlistSetting, "set " + failoverLogsSetting}, events)

	// Disabling logs failover and removing the allowlist: the allowlist is removed last.
	// The mocked settings component does not write to the config, so store the enabled state and the
	// allowlist there as the runtime settings would.
	cfg.Set(failoverLogsSetting, true, model.SourceRC)
	cfg.Set(logsServiceAllowlistSetting, []string{"web"}, model.SourceRC)
	events = nil
	rc.mrfUpdateCallback(map[string]state.RawConfig{
		"datadog/2/AGENT_FAILOVER/nologs/configname": noLogs,
	}, applyEmpty)
	assert.Equal(t, []string{"set " + failoverLogsSetting, "update " + logsServiceAllowlistSetting}, events)
	assert.Empty(t, cfg.GetStringSlice(logsServiceAllowlistSetting))
}

// writeThroughSettings writes runtime settings to the config, as the Multi-Region Failover runtime
// settings do, so that every write of the callback is observable through the config notifications.
type writeThroughSettings struct {
	settings.Component
	cfg    model.ReaderWriter
	failOn string // setting whose writes fail with err
	err    error
}

func (s *writeThroughSettings) SetRuntimeSetting(setting string, value interface{}, source model.Source) error {
	if setting == s.failOn && s.err != nil {
		return s.err
	}
	s.cfg.Set(setting, value, source)
	return nil
}

// mrfPairs are the failover flag and allowlist pairs the callback applies with the same rule.
var mrfPairs = []struct {
	name, flagSetting, allowlistSetting, flagField, allowlistField string
}{
	{"logs", failoverLogsSetting, logsServiceAllowlistSetting, "failover_logs", "logs_service_allowlist"},
	{"metrics", failoverMetricsSetting, metricsAllowlistSetting, "failover_metrics", "metrics_allowlist"},
}

// While the callback switches a failover flag and its allowlist, no intermediate state may forward an
// entry (a log service, a metric) that neither the previous nor the new configuration forwards.
func TestAgentMRFFailoverTransitions(t *testing.T) {
	entries := []string{"web", "api", "worker"}
	forwards := func(active bool, allowlist []string, entry string) bool {
		// An empty allowlist forwards everything.
		return active && (len(allowlist) == 0 || slices.Contains(allowlist, entry))
	}
	boolPtr := func(b bool) *bool { return &b }

	// FLAG and LIST stand for the pair's JSON fields. The configuration file enables failover or not and
	// allows "web"; a previous remote config update may have set both settings.
	tests := []struct {
		name       string
		fileActive bool
		rcActive   *bool
		rcList     []string
		update     string
		wantActive bool
		wantList   []string
	}{
		{
			// Disjoint from the file list on purpose: an intermediate state that still carries the file list
			// would forward "web", which neither the previous nor the new configuration forwards.
			name:       "enable with allowlist disjoint from the file",
			rcActive:   boolPtr(false),
			update:     `{"FLAG": true, "LIST": ["api"]}`,
			wantActive: true,
			wantList:   []string{"api"},
		},
		{
			name:       "enable with allowlist omitted, previous remote allowlist wider than the file",
			rcActive:   boolPtr(false),
			rcList:     []string{"web", "api"},
			update:     `{"FLAG": true}`,
			wantActive: true,
			wantList:   []string{"web"},
		},
		{
			name:       "enable by falling back to the file, previous remote allowlist wider than the file",
			fileActive: true,
			rcActive:   boolPtr(false),
			rcList:     []string{"web", "api"},
			update:     `{}`,
			wantActive: true,
			wantList:   []string{"web"},
		},
		{
			name:       "disable with empty allowlist",
			rcActive:   boolPtr(true),
			rcList:     []string{"web"},
			update:     `{"FLAG": false, "LIST": []}`,
			wantActive: false,
			wantList:   []string{},
		},
		{
			name:       "disable with wider allowlist",
			rcActive:   boolPtr(true),
			rcList:     []string{"web"},
			update:     `{"FLAG": false, "LIST": ["web", "api"]}`,
			wantActive: false,
			wantList:   []string{"api", "web"},
		},
		{
			name:       "disable with allowlist omitted",
			rcActive:   boolPtr(true),
			rcList:     []string{"web"},
			update:     `{"FLAG": false}`,
			wantActive: false,
			wantList:   []string{"web"},
		},
		{
			name:       "disable by falling back to the file, with empty allowlist",
			rcActive:   boolPtr(true),
			rcList:     []string{"web"},
			update:     `{"LIST": []}`,
			wantActive: false,
			wantList:   []string{},
		},
		{
			name:       "change allowlist while active",
			rcActive:   boolPtr(true),
			rcList:     []string{"web"},
			update:     `{"FLAG": true, "LIST": ["api"]}`,
			wantActive: true,
			wantList:   []string{"api"},
		},
		{
			name:       "change allowlist while inactive",
			rcList:     []string{"web"},
			update:     `{"LIST": ["api"]}`,
			wantActive: false,
			wantList:   []string{"api"},
		},
	}
	for _, pair := range mrfPairs {
		fields := strings.NewReplacer("FLAG", pair.flagField, "LIST", pair.allowlistField)
		for _, tt := range tests {
			t.Run(pair.name+"/"+tt.name, func(t *testing.T) {
				cfg := configmock.New(t)
				cfg.Set(pair.flagSetting, tt.fileActive, model.SourceFile)
				cfg.Set(pair.allowlistSetting, []string{"web"}, model.SourceFile)
				if tt.rcActive != nil {
					cfg.Set(pair.flagSetting, *tt.rcActive, model.SourceRC)
				}
				if tt.rcList != nil {
					cfg.Set(pair.allowlistSetting, tt.rcList, model.SourceRC)
				}
				wasActive, wasList := cfg.GetBool(pair.flagSetting), cfg.GetStringSlice(pair.allowlistSetting)

				var leaked []string
				cfg.OnUpdate(func(setting string, _ model.Source, _, _ any, _ uint64, _ model.Source) {
					if setting != pair.flagSetting && setting != pair.allowlistSetting {
						return
					}
					active, list := cfg.GetBool(pair.flagSetting), cfg.GetStringSlice(pair.allowlistSetting)
					for _, entry := range entries {
						if forwards(active, list, entry) && !forwards(wasActive, wasList, entry) && !forwards(tt.wantActive, tt.wantList, entry) {
							leaked = append(leaked, entry+" after "+setting)
						}
					}
				})

				rc := &rcClient{settingsComponent: &writeThroughSettings{cfg: cfg}}
				rc.mrfUpdateCallback(map[string]state.RawConfig{
					"datadog/2/AGENT_FAILOVER/transition/configname": {Config: []byte(fields.Replace(tt.update))},
				}, applyEmpty)

				assert.Equal(t, tt.wantActive, cfg.GetBool(pair.flagSetting))
				assert.Equal(t, tt.wantList, cfg.GetStringSlice(pair.allowlistSetting))
				assert.Empty(t, leaked, "forwarded in an intermediate state")
			})
		}
	}
}

// Every config contributing to a merged allowlist is told whether the allowlist was applied.
func TestAgentMRFAllowlistReportsStatusToEveryConfig(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want state.ApplyState
	}{
		{name: "acknowledged", want: state.ApplyStateAcknowledged},
		{name: "error", err: errors.New("cannot set"), want: state.ApplyStateError},
	}
	for _, pair := range mrfPairs {
		fields := strings.NewReplacer("LIST", pair.allowlistField)
		for _, tt := range tests {
			t.Run(pair.name+"/"+tt.name, func(t *testing.T) {
				cfg := configmock.New(t)
				rc := &rcClient{settingsComponent: &writeThroughSettings{cfg: cfg, failOn: pair.allowlistSetting, err: tt.err}}

				statuses := map[string]state.ApplyStatus{}
				rc.mrfUpdateCallback(map[string]state.RawConfig{
					"datadog/2/AGENT_FAILOVER/web/configname":   {Config: []byte(fields.Replace(`{"LIST": ["web"]}`))},
					"datadog/2/AGENT_FAILOVER/api/configname":   {Config: []byte(fields.Replace(`{"LIST": ["api"]}`))},
					"datadog/2/AGENT_FAILOVER/empty/configname": {Config: []byte(fields.Replace(`{"LIST": []}`))},
				}, func(cfgPath string, status state.ApplyStatus) { statuses[cfgPath] = status })

				require.Len(t, statuses, 3)
				for cfgPath, status := range statuses {
					assert.Equal(t, tt.want, status.State, cfgPath)
				}
				if tt.err == nil {
					assert.Equal(t, []string{"api", "web"}, cfg.GetStringSlice(pair.allowlistSetting))
				}
			})
		}
	}
}

// A config that set several settings ends in error when one of its writes fails, whichever write fails,
// and a failed write in one pair does not stop the other pairs.
func TestAgentMRFConfigStatusStaysErrorAcrossSettings(t *testing.T) {
	const cfgPath = "datadog/2/AGENT_FAILOVER/both/configname"

	t.Run("later write fails", func(t *testing.T) {
		// Inactive: the allowlist is written first and acknowledged, then the flag write fails.
		cfg := configmock.New(t)
		rc := &rcClient{settingsComponent: &writeThroughSettings{cfg: cfg, failOn: failoverLogsSetting, err: errors.New("cannot set")}}
		statuses := map[string]state.ApplyStatus{}
		rc.mrfUpdateCallback(map[string]state.RawConfig{
			cfgPath: {Config: []byte(`{"failover_logs": true, "logs_service_allowlist": ["web"]}`)},
		}, func(cfgPath string, status state.ApplyStatus) { statuses[cfgPath] = status })

		assert.Equal(t, state.ApplyStateError, statuses[cfgPath].State)
		assert.False(t, cfg.GetBool(failoverLogsSetting))
	})

	t.Run("earlier write fails", func(t *testing.T) {
		// The logs allowlist write fails, the APM flag of the same config succeeds afterwards.
		cfg := configmock.New(t)
		rc := &rcClient{settingsComponent: &writeThroughSettings{cfg: cfg, failOn: logsServiceAllowlistSetting, err: errors.New("cannot set")}}
		statuses := map[string]state.ApplyStatus{}
		rc.mrfUpdateCallback(map[string]state.RawConfig{
			cfgPath: {Config: []byte(`{"logs_service_allowlist": ["web"], "failover_apm": true}`)},
		}, func(cfgPath string, status state.ApplyStatus) { statuses[cfgPath] = status })

		assert.Equal(t, state.ApplyStateError, statuses[cfgPath].State)
		assert.True(t, cfg.GetBool(failoverAPMSetting), "the APM flag is still applied")
	})

	t.Run("metrics failure does not stop logs or APM", func(t *testing.T) {
		cfg := configmock.New(t)
		rc := &rcClient{settingsComponent: &writeThroughSettings{cfg: cfg, failOn: metricsAllowlistSetting, err: errors.New("cannot set")}}
		statuses := map[string]state.ApplyStatus{}
		rc.mrfUpdateCallback(map[string]state.RawConfig{
			cfgPath: {Config: []byte(`{"metrics_allowlist": ["system.cpu.usage"], "failover_logs": true, "failover_apm": true}`)},
		}, func(cfgPath string, status state.ApplyStatus) { statuses[cfgPath] = status })

		assert.Equal(t, state.ApplyStateError, statuses[cfgPath].State)
		assert.True(t, cfg.GetBool(failoverLogsSetting), "the logs pair is still applied")
		assert.True(t, cfg.GetBool(failoverAPMSetting), "the APM flag is still applied")
	})

	t.Run("service checks failure prevents enabling metrics", func(t *testing.T) {
		cfg := configmock.New(t)
		rc := &rcClient{settingsComponent: &writeThroughSettings{cfg: cfg, failOn: failoverServiceChecksSetting, err: errors.New("cannot set")}}
		statuses := map[string]state.ApplyStatus{}
		rc.mrfUpdateCallback(map[string]state.RawConfig{
			cfgPath: {Config: []byte(`{"failover_metrics": true, "failover_service_checks": false}`)},
		}, func(cfgPath string, status state.ApplyStatus) { statuses[cfgPath] = status })

		assert.Equal(t, state.ApplyStateError, statuses[cfgPath].State)
		assert.False(t, cfg.GetBool(failoverMetricsSetting), "metrics failover must not turn on when disabling service checks failed")
	})

	t.Run("metrics disable failure keeps service checks disabled", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.Set(failoverMetricsSetting, true, model.SourceRC)
		cfg.Set(failoverServiceChecksSetting, false, model.SourceRC)
		rc := &rcClient{settingsComponent: &writeThroughSettings{cfg: cfg, failOn: failoverMetricsSetting, err: errors.New("cannot set")}}
		statuses := map[string]state.ApplyStatus{}
		rc.mrfUpdateCallback(map[string]state.RawConfig{
			cfgPath: {Config: []byte(`{"failover_metrics": false}`)},
		}, func(cfgPath string, status state.ApplyStatus) { statuses[cfgPath] = status })

		assert.Equal(t, state.ApplyStateError, statuses[cfgPath].State)
		assert.True(t, cfg.IsConfigured(failoverServiceChecksSetting), "service checks override must stay set while metrics failover is still active")
		assert.False(t, cfg.GetBool(failoverServiceChecksSetting))
	})
}

// While the callback switches failover_metrics and failover_service_checks, no intermediate state
// may forward service checks when neither the previous nor the new configuration forwards them.
func TestAgentMRFServiceChecksTransitions(t *testing.T) {
	serviceChecksForward := func(cfg model.Reader) bool {
		if cfg.IsConfigured(failoverServiceChecksSetting) {
			return cfg.GetBool(failoverServiceChecksSetting)
		}
		return cfg.GetBool(failoverMetricsSetting)
	}
	boolPtr := func(b bool) *bool { return &b }

	tests := []struct {
		name              string
		fileMetrics       bool
		fileServiceChecks *bool
		rcMetrics         *bool
		rcServiceChecks   *bool
		updates           map[string]string
		wantMetrics       bool
		wantSCConfigured  bool
		wantSCValue       bool
		wantSCForwarded   bool
	}{
		{
			name:             "enable metrics with service checks disabled",
			rcMetrics:        boolPtr(false),
			updates:          map[string]string{"c1": `{"failover_metrics": true, "failover_service_checks": false}`},
			wantMetrics:      true,
			wantSCConfigured: true,
			wantSCValue:      false,
			wantSCForwarded:  false,
		},
		{
			name:             "disable metrics and remove service checks override",
			rcMetrics:        boolPtr(true),
			rcServiceChecks:  boolPtr(false),
			updates:          map[string]string{"c1": `{"failover_metrics": false}`},
			wantMetrics:      false,
			wantSCConfigured: false,
			wantSCValue:      false,
			wantSCForwarded:  false,
		},
		{
			name:             "disable metrics by falling back to file and remove service checks override",
			fileMetrics:      false,
			rcMetrics:        boolPtr(true),
			rcServiceChecks:  boolPtr(false),
			updates:          map[string]string{"c1": `{}`},
			wantMetrics:      false,
			wantSCConfigured: false,
			wantSCValue:      false,
			wantSCForwarded:  false,
		},
		{
			name:             "enable service checks independently of metrics",
			updates:          map[string]string{"c1": `{"failover_service_checks": true}`},
			wantMetrics:      false,
			wantSCConfigured: true,
			wantSCValue:      true,
			wantSCForwarded:  true,
		},
		{
			name:              "fall back to file service checks false when omitted from RC",
			fileServiceChecks: boolPtr(false),
			rcServiceChecks:   boolPtr(true),
			updates:           map[string]string{"c1": `{"failover_metrics": true}`},
			wantMetrics:       true,
			wantSCConfigured:  true,
			wantSCValue:       false,
			wantSCForwarded:   false,
		},
		{
			name: "OR merge across multiple configs prefers true",
			updates: map[string]string{
				"c1": `{"failover_service_checks": false}`,
				"c2": `{"failover_service_checks": true}`,
			},
			wantMetrics:      false,
			wantSCConfigured: true,
			wantSCValue:      true,
			wantSCForwarded:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set(failoverMetricsSetting, tt.fileMetrics, model.SourceFile)
			if tt.fileServiceChecks != nil {
				cfg.Set(failoverServiceChecksSetting, *tt.fileServiceChecks, model.SourceFile)
			}
			if tt.rcMetrics != nil {
				cfg.Set(failoverMetricsSetting, *tt.rcMetrics, model.SourceRC)
			}
			if tt.rcServiceChecks != nil {
				cfg.Set(failoverServiceChecksSetting, *tt.rcServiceChecks, model.SourceRC)
			}
			wasForwarding := serviceChecksForward(cfg)

			var leaked []string
			cfg.OnUpdate(func(setting string, _ model.Source, _, _ any, _ uint64, _ model.Source) {
				if setting != failoverMetricsSetting && setting != failoverServiceChecksSetting {
					return
				}
				if serviceChecksForward(cfg) && !wasForwarding && !tt.wantSCForwarded {
					leaked = append(leaked, "forwarded service checks after "+setting)
				}
			})

			rawUpdates := make(map[string]state.RawConfig, len(tt.updates))
			for id, body := range tt.updates {
				rawUpdates["datadog/2/AGENT_FAILOVER/"+id+"/configname"] = state.RawConfig{Config: []byte(body)}
			}

			rc := &rcClient{settingsComponent: &writeThroughSettings{cfg: cfg}}
			rc.mrfUpdateCallback(rawUpdates, applyEmpty)

			assert.Equal(t, tt.wantMetrics, cfg.GetBool(failoverMetricsSetting))
			assert.Equal(t, tt.wantSCConfigured, cfg.IsConfigured(failoverServiceChecksSetting))
			assert.Equal(t, tt.wantSCValue, cfg.GetBool(failoverServiceChecksSetting))
			assert.Equal(t, tt.wantSCForwarded, serviceChecksForward(cfg))
			assert.Empty(t, leaked, "service checks leaked in an intermediate state")
		})
	}
}
