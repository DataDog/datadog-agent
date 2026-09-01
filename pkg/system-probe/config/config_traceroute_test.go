// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestBasicDynamicTestSettingIsRegistered(t *testing.T) {
	coreCfg := mock.New(t)
	require.True(t, coreCfg.IsKnown("network_path.connections_monitoring.basic_tests_enabled"))
}

func TestTracerouteModule(t *testing.T) {
	boolPtr := func(value bool) *bool { return &value }
	tests := []struct {
		name                    string
		npmEnabled              bool
		standardTestsEnabled    bool
		basicTestsEnabled       bool
		tracerouteEnabled       *bool
		expectTracerouteEnabled bool
		expectWarning           bool
	}{
		{name: "disabled by default"},
		{name: "NPM alone does not enable traceroute", npmEnabled: true},
		{name: "standard tests require NPM", standardTestsEnabled: true},
		{name: "basic tests require NPM", basicTestsEnabled: true},
		{name: "standard tests enable traceroute", npmEnabled: true, standardTestsEnabled: true, expectTracerouteEnabled: true},
		{name: "basic tests enable traceroute", npmEnabled: true, basicTestsEnabled: true, expectTracerouteEnabled: true},
		{name: "explicit true enables traceroute", tracerouteEnabled: boolPtr(true), expectTracerouteEnabled: true},
		{name: "explicit false without dynamic tests", npmEnabled: true, tracerouteEnabled: boolPtr(false)},
		{name: "explicit false with standard tests but without NPM does not warn", standardTestsEnabled: true, tracerouteEnabled: boolPtr(false)},
		{name: "explicit false with basic tests but without NPM does not warn", basicTestsEnabled: true, tracerouteEnabled: boolPtr(false)},
		{name: "explicit false overrides standard tests", npmEnabled: true, standardTestsEnabled: true, tracerouteEnabled: boolPtr(false), expectWarning: true},
		{name: "explicit false overrides basic tests", npmEnabled: true, basicTestsEnabled: true, tracerouteEnabled: boolPtr(false), expectWarning: true},
	}

	var logs bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&logs, log.WarnLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "warn")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "debug") })

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coreCfg := mock.New(t)
			sysprobeCfg := mock.NewSystemProbe(t)
			coreCfg.SetInTest("network_path.connections_monitoring.enabled", test.standardTestsEnabled)
			coreCfg.SetInTest("network_path.connections_monitoring.basic_tests_enabled", test.basicTestsEnabled)
			sysprobeCfg.SetInTest("network_config.enabled", test.npmEnabled)
			if test.tracerouteEnabled != nil {
				sysprobeCfg.SetInTest("traceroute.enabled", *test.tracerouteEnabled)
			}
			assert.Equal(t, test.tracerouteEnabled != nil, sysprobeCfg.IsConfigured("traceroute.enabled"))

			logs.Reset()
			cfg, err := load()
			require.NoError(t, err)

			assert.Equal(t, test.expectTracerouteEnabled, cfg.ModuleIsEnabled(TracerouteModule))
			assert.Equal(t, test.tracerouteEnabled != nil || test.expectTracerouteEnabled, sysprobeCfg.IsConfigured("traceroute.enabled"))
			assert.Equal(t, test.expectTracerouteEnabled, sysprobeCfg.GetBool("traceroute.enabled"))
			expectSource := pkgconfigmodel.SourceDefault
			if test.tracerouteEnabled != nil {
				expectSource = pkgconfigmodel.SourceUnknown
			} else if test.expectTracerouteEnabled {
				expectSource = pkgconfigmodel.SourceAgentRuntime
			}
			assert.Equal(t, expectSource, sysprobeCfg.GetSource("traceroute.enabled"))
			assert.Equal(t, test.expectWarning, bytes.Contains(logs.Bytes(), []byte("system-probe traceroute was explicitly disabled")))
		})
	}
}

func TestTracerouteModuleUnsetDefaultTrue(t *testing.T) {
	mock.New(t)
	sysprobeCfg := mock.NewSystemProbe(t)
	sysprobeCfg.SetDefault("traceroute.enabled", true)
	assert.False(t, sysprobeCfg.IsConfigured("traceroute.enabled"))
	assert.True(t, sysprobeCfg.GetBool("traceroute.enabled"))

	cfg, err := load()
	require.NoError(t, err)

	assert.True(t, cfg.ModuleIsEnabled(TracerouteModule))
	assert.True(t, sysprobeCfg.IsConfigured("traceroute.enabled"))
	assert.True(t, sysprobeCfg.GetBool("traceroute.enabled"))
	assert.Equal(t, pkgconfigmodel.SourceAgentRuntime, sysprobeCfg.GetSource("traceroute.enabled"))
}
