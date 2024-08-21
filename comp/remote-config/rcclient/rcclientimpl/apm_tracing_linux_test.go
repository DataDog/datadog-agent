// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build linux

package rcclientimpl

import (
	yamlv2 "gopkg.in/yaml.v2"
	"os"
	"strconv"
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

func TestServiceNameConfig(t *testing.T) {
	data := []struct {
		name string
		cfg  []serviceEnvConfig
	}{
		{
			name: "simple",
			cfg: []serviceEnvConfig{
				{
					ProvidedService: "replacement_name1",
					Service:         "original_name1",
				},
				{
					ProvidedService: "replacement_name2",
					Service:         "original_name2",
				},
				{
					ProvidedService: "replacement_name3",
					Service:         "original_name3",
				},
			},
		},
		{
			name: "none",
			cfg:  nil,
		},
		{
			name: "empty",
			cfg:  []serviceEnvConfig{},
		},
	}
	for _, d := range data {
		t.Run(d.name, func(t *testing.T) {
			// setup the temp file name
			dir := t.TempDir()
			origName := apmServiceNameFilePath
			apmServiceNameFilePath = dir + "/" + d.name
			t.Cleanup(func() {
				apmServiceNameFilePath = origName
			})

			// write the file
			tec := tracingEnabledConfig{
				ServiceEnvConfigs: nil,
			}
			err := writeServiceNameConfig(tec)
			if err != nil {
				t.Fatal(err)
			}

			// read the file
			f, err := os.ReadFile(apmServiceNameFilePath)
			if err != nil {
				t.Fatal(err)
			}
			m := map[string]string{}
			err = yamlv2.Unmarshal(f, &m)
			if err != nil {
				t.Fatal(err)
			}

			// scan the file for entries
			for i := range len(d.cfg) {
				generatedName := "original_name" + strconv.Itoa(i+1)
				expectedName := "replacement_name" + strconv.Itoa(i+1)
				if m[generatedName] != expectedName {
					t.Error("expected", expectedName, "got", m[generatedName], "for", generatedName)
				}
			}
			// check for non-entries
			if _, ok := m["not in there"]; ok {
				t.Error("expected not in there, got", m["not in there"])
			}
		})
	}
	t.Run("check_map", func(t *testing.T) {
		// setup the temp file name
		dir := t.TempDir()
		origName := apmServiceNameFilePath
		apmServiceNameFilePath = dir + "/" + "check_map"
		t.Cleanup(func() {
			apmServiceNameFilePath = origName
		})

		// write the file
		tec := tracingEnabledConfig{
			ServiceEnvConfigs: []serviceEnvConfig{
				{
					ProvidedService: "p1",
					Service:         "s1",
				},
				{
					ProvidedService: "p2",
					Service:         "s2",
				},
				{
					Service: "nope",
				},
			},
		}
		err := writeServiceNameConfig(tec)
		if err != nil {
			t.Fatal(err)
		}

		// read the file
		f, err := os.ReadFile(apmServiceNameFilePath)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		err = yamlv2.Unmarshal(f, &m)
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, m, map[string]string{
			"s2": "p2",
			"s1": "p1",
		})
	})
}
