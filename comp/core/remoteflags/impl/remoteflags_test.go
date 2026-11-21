// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteflagsimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/remoteflags"
)

const (
	testFlag = "bound_test_flag"
	testKey  = "agent_telemetry.errortracking.enabled"
)

// The configuration component is what the client writes the flags to.
var _ remoteflags.ConfigSetter = config.Component(nil)

// boundSubscriber binds testFlag to testKey the way a component does.
type boundSubscriber struct{}

func (boundSubscriber) Handlers() []remoteflags.FlagHandler {
	return []remoteflags.FlagHandler{boundHandler{}}
}

type boundHandler struct{}

func (boundHandler) FlagName() remoteflags.FlagName           { return testFlag }
func (boundHandler) ConfigurationField() string               { return testKey }
func (boundHandler) OnChange(remoteflags.FlagValue) error     { return nil }
func (boundHandler) OnNoConfig()                              {}
func (boundHandler) SafeRecover(error, remoteflags.FlagValue) {}
func (boundHandler) IsHealthy() bool                          { return true }

func newTestClient(t *testing.T, yaml string) (*remoteflags.Client, config.Component) {
	t.Helper()
	cfg := config.NewMockFromYAML(t, yaml)
	comp := NewComponent(Requires{
		Lc:          compdef.NewTestLifecycle(t),
		Config:      cfg,
		Subscribers: []remoteflags.RemoteFlagSubscriber{boundSubscriber{}},
	})
	return comp.Comp.GetClient(), cfg
}

func deliver(client *remoteflags.Client, payload string) {
	client.OnUpdate(map[string]state.RawConfig{"flags": {Config: []byte(payload)}}, func(string, state.ApplyStatus) {})
}

const (
	flagOn  = `{"flags":[{"name":"bound_test_flag","enabled":true}]}`
	flagOff = `{"flags":[{"name":"bound_test_flag","enabled":false}]}`
	// otherFlag makes the bound flag absent from the received configuration.
	otherFlag = `{"flags":[{"name":"another_flag","enabled":true}]}`
)

// With nothing set locally, the flag drives the key under the Remote Config
// source and gives it back when it goes away.
func TestBoundFlagDrivesTheKey(t *testing.T) {
	client, cfg := newTestClient(t, "")
	require.False(t, cfg.GetBool(testKey))

	deliver(client, flagOn)
	assert.True(t, cfg.GetBool(testKey))
	assert.Equal(t, model.SourceRC, cfg.GetSource(testKey))

	deliver(client, flagOff)
	assert.False(t, cfg.GetBool(testKey))
	assert.Equal(t, model.SourceRC, cfg.GetSource(testKey))

	deliver(client, flagOn)
	deliver(client, otherFlag)
	assert.False(t, cfg.GetBool(testKey))
	assert.Equal(t, model.SourceDefault, cfg.GetSource(testKey))
}

// An explicit local value is never overwritten, whichever way the flag goes,
// even when a payload asks to override it.
func TestBoundFlagNeverOverridesLocalValue(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		payload string
		want    bool
	}{
		{name: "local false, flag true", yaml: "agent_telemetry:\n  errortracking:\n    enabled: false\n", payload: flagOn, want: false},
		{name: "local false, flag true with override_local", yaml: "agent_telemetry:\n  errortracking:\n    enabled: false\n", payload: `{"flags":[{"name":"bound_test_flag","enabled":true,"override_local":true}]}`, want: false},
		{name: "local true, flag false", yaml: "agent_telemetry:\n  errortracking:\n    enabled: true\n", payload: flagOff, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, cfg := newTestClient(t, tt.yaml)

			deliver(client, tt.payload)
			assert.Equal(t, tt.want, cfg.GetBool(testKey))
			assert.Equal(t, model.SourceFile, cfg.GetSource(testKey))

			deliver(client, otherFlag)
			assert.Equal(t, tt.want, cfg.GetBool(testKey))
			assert.Equal(t, model.SourceFile, cfg.GetSource(testKey))
		})
	}
}

// Known limit: the Remote Config source outranks fleet policies in the
// configuration, so a fleet policy that sets the key after the flag was
// applied does not take effect until the flag gives the key back.
func TestBoundFlagOutranksLaterFleetPolicy(t *testing.T) {
	client, cfg := newTestClient(t, "")
	deliver(client, flagOn)
	require.True(t, cfg.GetBool(testKey))

	cfg.Set(testKey, false, model.SourceFleetPolicies)
	assert.True(t, cfg.GetBool(testKey))
	assert.Equal(t, model.SourceRC, cfg.GetSource(testKey))

	// The flag going away is the way out.
	deliver(client, otherFlag)
	assert.False(t, cfg.GetBool(testKey))
	assert.Equal(t, model.SourceFleetPolicies, cfg.GetSource(testKey))
}

// A fleet policy in place before the flag arrives is respected.
func TestBoundFlagRespectsEarlierFleetPolicy(t *testing.T) {
	client, cfg := newTestClient(t, "")
	cfg.Set(testKey, false, model.SourceFleetPolicies)

	deliver(client, flagOn)

	assert.False(t, cfg.GetBool(testKey))
	assert.Equal(t, model.SourceFleetPolicies, cfg.GetSource(testKey))
}
