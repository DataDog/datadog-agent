// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package mock

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestSubscribeAndUpdate(t *testing.T) {
	c := New()
	assert.Zero(t, c.Subscribers("P"))

	var got []map[string]state.RawConfig
	c.Subscribe("P", func(u map[string]state.RawConfig, apply func(string, state.ApplyStatus)) {
		got = append(got, u)
		for path := range u {
			apply(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	})
	c.SubscribeIgnoreExpiration("P", func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
		got = append(got, nil)
	})
	assert.Positive(t, c.Subscribers("P"))
	assert.Zero(t, c.Subscribers("Q"))

	cfgs := map[string]state.RawConfig{"a": {Config: []byte("x")}}
	statuses := map[string]state.ApplyStatus{}
	c.Update("P", cfgs, func(path string, status state.ApplyStatus) { statuses[path] = status })

	assert.Equal(t, map[string]state.ApplyStatus{"a": {State: state.ApplyStateAcknowledged}}, statuses)
	assert.Equal(t, []map[string]state.RawConfig{cfgs, nil}, got)
	assert.Equal(t, cfgs, c.GetConfigs("P"))

	// without subscribers the configs are still recorded and the callback is not called
	c.Update("Q", cfgs, func(string, state.ApplyStatus) { t.Fatal("unexpected apply status") })
	assert.Equal(t, cfgs, c.GetConfigs("Q"))
}

func TestConfigsAreNotShared(t *testing.T) {
	c := New()
	cfgs := map[string]state.RawConfig{"a": {}}
	c.SetConfigs("P", cfgs)

	cfgs["b"] = state.RawConfig{}
	got := c.GetConfigs("P")
	assert.Len(t, got, 1)

	got["c"] = state.RawConfig{}
	assert.Len(t, c.GetConfigs("P"), 1)

	var delivered map[string]state.RawConfig
	c.Subscribe("P", func(u map[string]state.RawConfig, _ func(string, state.ApplyStatus)) { delivered = u })
	c.Update("P", cfgs, nil)
	delivered["d"] = state.RawConfig{}
	assert.Len(t, c.GetConfigs("P"), 2)
}

func TestApplyStatusAndProof(t *testing.T) {
	c := New()
	c.SetConfigs("P", map[string]state.RawConfig{"p": {}})
	c.UpdateApplyStatus("p", state.ApplyStatus{State: state.ApplyStateError, Error: "boom"})
	assert.Equal(t, state.ApplyStatus{State: state.ApplyStateError, Error: "boom"}, c.ApplyStatuses()["p"])

	c.UpdateApplyStatus("unknown", state.ApplyStatus{State: state.ApplyStateAcknowledged})
	assert.NotContains(t, c.ApplyStatuses(), "unknown")

	_, ok := c.GetConfigTUFProof("t")
	assert.False(t, ok)
	c.SetConfigTUFProof(state.ConfigTUFProof{TargetPath: "t", Targets: []byte("x")})
	proof, ok := c.GetConfigTUFProof("t")
	assert.True(t, ok)
	assert.Equal(t, []byte("x"), proof.Targets)
}

func TestComponent(t *testing.T) {
	c := NewComponent()

	called := false
	c.Subscribe(data.Product("P"), func(map[string]state.RawConfig, func(string, state.ApplyStatus)) { called = true })
	require.Positive(t, c.Subscribers("P"))
	c.Update("P", nil, func(string, state.ApplyStatus) {})
	assert.True(t, called)
}

func TestConcurrentUse(_ *testing.T) {
	c := New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			c.Subscribe("P", func(u map[string]state.RawConfig, apply func(string, state.ApplyStatus)) {
				for path := range u {
					apply(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
				}
				c.GetConfigs("P")
			})
		}()
		go func() {
			defer wg.Done()
			c.Update("P", map[string]state.RawConfig{"a": {}}, c.UpdateApplyStatus)
			c.ApplyStatuses()
			c.Subscribers("P")
		}()
	}
	wg.Wait()
}
