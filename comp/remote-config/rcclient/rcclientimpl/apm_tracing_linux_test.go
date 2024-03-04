// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build linux

package rcclientimpl

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestOnAPMTracingUpdate(t *testing.T) {
	mkTemp := func(t *testing.T) func() {
		oldPath := apmTracingFilePath
		f, err := os.CreateTemp("", "test")
		require.NoError(t, err)
		f.Close() // This is required for windows unit tests as windows will not allow this file to be deleted while we have this handle.
		apmTracingFilePath = f.Name()
		return func() {
			apmTracingFilePath = oldPath
		}
	}

	t.Run("Empty update deletes file", func(t *testing.T) {
		defer mkTemp(t)()
		rc := rcClient{}

		rc.onAPMTracingUpdate(map[string]state.RawConfig{}, nil)

		_, err := os.Open(apmTracingFilePath)
		if !os.IsNotExist(err) {
			// file still exists when it shouldn't
			assert.Fail(t, "Empty update did not delete existing config file")
		}
	})

	t.Run("Valid update writes file", func(t *testing.T) {
		defer mkTemp(t)()
		rc := rcClient{}
		callbackCalls := map[string]string{}
		callback := func(id string, status state.ApplyStatus) {
			callbackCalls[id] = status.Error
		}

		hostConfig := state.RawConfig{Config: []byte(`{"infra_target": {"tags":["k:v"]},"lib_config":{"env":"someEnv","tracing_enabled":true}}`)}
		senvConfig := state.RawConfig{Config: []byte(`{"service_target": {"service":"s1", "env":"e1"}}`)}

		updates := map[string]state.RawConfig{
			"host1": hostConfig,
			"srv1":  senvConfig,
		}
		rc.onAPMTracingUpdate(updates, callback)

		assert.Len(t, callbackCalls, 2)
		assert.Empty(t, callbackCalls["host1"])
		assert.Empty(t, callbackCalls["srv1"])
		actualBytes, err := os.ReadFile(apmTracingFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "tracing_enabled: true\nenv: someEnv\nservice_env_configs:\n- service: s1\n  env: e1\n  tracing_enabled: false\n", string(actualBytes))
	})

	t.Run("lowest config-id wins", func(t *testing.T) {
		defer mkTemp(t)()
		rc := rcClient{}
		callbackCalls := map[string]string{}
		callback := func(id string, status state.ApplyStatus) {
			callbackCalls[id] = status.Error
		}

		hostConfig := state.RawConfig{Config: []byte(`{"id":"abc","infra_target": {"tags":["k:v"]},"lib_config":{"env":"someEnv","tracing_enabled":true}}`)}
		hostConfig2 := state.RawConfig{Config: []byte(`{"id":"xyz","infra_target": {"tags":["k:v"]},"lib_config":{"env":"someEnv2","tracing_enabled":true}}`)}

		updates := map[string]state.RawConfig{
			"abc": hostConfig,
			"xyz": hostConfig2,
		}
		rc.onAPMTracingUpdate(updates, callback)

		assert.Len(t, callbackCalls, 2)
		assert.Empty(t, callbackCalls["abc"])
		assert.Equal(t, "DUPLICATE_HOST_CONFIG", callbackCalls["xyz"])
		actualBytes, err := os.ReadFile(apmTracingFilePath)
		assert.NoError(t, err)
		assert.Equal(t, "tracing_enabled: true\nenv: someEnv\nservice_env_configs: []\n", string(actualBytes))
	})

	t.Run("bad updates report failure", func(t *testing.T) {
		defer mkTemp(t)()
		rc := rcClient{}
		calls := map[string]string{}
		callback := func(id string, status state.ApplyStatus) {
			calls[id] = status.Error
		}

		missingTarget := state.RawConfig{Config: []byte(`{}`)}
		badPayload := state.RawConfig{Config: []byte(`{`)}

		updates := map[string]state.RawConfig{
			"missingTarget": missingTarget,
			"badPayload":    badPayload,
		}
		rc.onAPMTracingUpdate(updates, callback)

		assert.Len(t, calls, 2)
		assert.Equal(t, calls["missingTarget"], MissingServiceTarget)
		assert.Equal(t, calls["badPayload"], InvalidAPMTracingPayload)
	})
}
