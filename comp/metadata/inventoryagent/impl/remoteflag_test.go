// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/agentmetadata"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
)

func newTestFlagHandler(t *testing.T) (*fullConfigFlagHandler, config.Component) {
	conf := config.NewMock(t)
	return newFullConfigFlagHandler(conf, logmock.New(t)), conf
}

func TestFullConfigFlagName(t *testing.T) {
	h, _ := newTestFlagHandler(t)
	assert.Equal(t, pkgremoteflags.FlagName("inventory_drop_full_configuration"), h.FlagName())
	assert.Equal(t, []pkgremoteflags.FlagHandler{h}, h.Handlers())
	assert.True(t, h.IsHealthy())
}

func TestFullConfigFlagSetsSetting(t *testing.T) {
	h, conf := newTestFlagHandler(t)
	require.Equal(t, model.SourceDefault, conf.GetSource(fullConfigurationSetting))

	require.NoError(t, h.OnChange(true))
	assert.False(t, conf.GetBool(fullConfigurationSetting))
	assert.Equal(t, model.SourceAgentRuntime, conf.GetSource(fullConfigurationSetting))

	require.NoError(t, h.OnChange(false))
	assert.True(t, conf.GetBool(fullConfigurationSetting))
	assert.Equal(t, model.SourceAgentRuntime, conf.GetSource(fullConfigurationSetting))
}

func TestFullConfigFlagResetsSetting(t *testing.T) {
	resets := map[string]func(h *fullConfigFlagHandler){
		"OnNoConfig":  func(h *fullConfigFlagHandler) { h.OnNoConfig() },
		"SafeRecover": func(h *fullConfigFlagHandler) { h.SafeRecover(assert.AnError, true) },
	}
	for name, reset := range resets {
		t.Run(name, func(t *testing.T) {
			h, conf := newTestFlagHandler(t)
			require.NoError(t, h.OnChange(true))
			require.False(t, conf.GetBool(fullConfigurationSetting))

			reset(h)
			assert.True(t, conf.GetBool(fullConfigurationSetting))
			assert.Equal(t, model.SourceDefault, conf.GetSource(fullConfigurationSetting))

			reset(h) // nothing left to unset
			assert.Equal(t, model.SourceDefault, conf.GetSource(fullConfigurationSetting))
		})
	}
}

func TestFullConfigFlagKeepsExplicitSetting(t *testing.T) {
	for _, source := range []model.Source{
		model.SourceInfraMode,
		model.SourceFile,
		model.SourceFleetPolicies,
		model.SourceRC,
		model.SourceCLI,
	} {
		t.Run(string(source), func(t *testing.T) {
			h, conf := newTestFlagHandler(t)
			conf.Set(fullConfigurationSetting, true, source)

			require.NoError(t, h.OnChange(true))
			assert.True(t, conf.GetBool(fullConfigurationSetting))
			assert.Equal(t, source, conf.GetSource(fullConfigurationSetting))

			h.OnNoConfig()
			assert.True(t, conf.GetBool(fullConfigurationSetting))
			assert.Equal(t, source, conf.GetSource(fullConfigurationSetting))
		})
	}
}

func getMetadata(t *testing.T, ia *inventoryagent) map[string]interface{} {
	t.Helper()
	return ia.getPayload().(*agentmetadata.Payload).Metadata
}

func TestComponentWiresFullConfigurationFlag(t *testing.T) {
	provides := getProvides(t, map[string]any{"inventories_configuration_enabled": true}, nil)
	ia := provides.Comp.(*inventoryagent)

	require.Len(t, provides.Subscriber.Handlers(), 1)
	h := provides.Subscriber.Handlers()[0]
	assert.Equal(t, pkgremoteflags.FlagName(flagDropFullConfiguration), h.FlagName())

	assert.Contains(t, getMetadata(t, ia), "full_configuration")
	require.NoError(t, h.OnChange(true))
	md := getMetadata(t, ia)
	assert.NotContains(t, md, "full_configuration")
	assert.Contains(t, md, "provided_configuration")
	assert.Contains(t, md["agent_runtime_configuration"], fullConfigurationSetting)
	h.OnNoConfig()
	assert.Contains(t, getMetadata(t, ia), "full_configuration")
}
