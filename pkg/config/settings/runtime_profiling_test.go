// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package settings

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	compsettings "github.com/DataDog/datadog-agent/comp/core/settings"
)

// mockClient is a test double for the Client interface.
type mockClient struct {
	data     map[string]interface{}
	getCalls []string
	setCalls []struct{ key, value string }
	getErr   error
	setErr   error
}

func (m *mockClient) Get(key string) (interface{}, error) {
	m.getCalls = append(m.getCalls, key)
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.data[key], nil
}

func (m *mockClient) Set(key, value string) (bool, error) {
	m.setCalls = append(m.setCalls, struct{ key, value string }{key, value})
	if m.setErr != nil {
		return false, m.setErr
	}
	return true, nil
}

func (m *mockClient) GetWithSources(_ string) (map[string]interface{}, error) {
	return nil, nil
}

func (m *mockClient) List() (map[string]compsettings.RuntimeSettingResponse, error) {
	return nil, nil
}

func (m *mockClient) FullConfig() (string, error)                { return "", nil }
func (m *mockClient) FullConfigWithoutDefaults() (string, error) { return "", nil }
func (m *mockClient) FullConfigBySource() (string, error)        { return "", nil }

func TestExecWithRuntimeProfilingSettings(t *testing.T) {
	t.Run("no profiling options", func(t *testing.T) {
		client := &mockClient{data: map[string]interface{}{}}
		called := false
		err := ExecWithRuntimeProfilingSettings(func() { called = true }, ProfilingOpts{}, client)
		require.NoError(t, err)
		assert.True(t, called)
		assert.Empty(t, client.getCalls)
		assert.Empty(t, client.setCalls)
	})

	t.Run("mutex profiling with zero fraction is a no-op", func(t *testing.T) {
		client := &mockClient{data: map[string]interface{}{}}
		err := ExecWithRuntimeProfilingSettings(func() {}, ProfilingOpts{ProfileMutex: true, ProfileMutexFraction: 0}, client)
		require.NoError(t, err)
		assert.Empty(t, client.getCalls)
		assert.Empty(t, client.setCalls)
	})

	t.Run("blocking profiling with zero rate is a no-op", func(t *testing.T) {
		client := &mockClient{data: map[string]interface{}{}}
		err := ExecWithRuntimeProfilingSettings(func() {}, ProfilingOpts{ProfileBlocking: true, ProfileBlockingRate: 0}, client)
		require.NoError(t, err)
		assert.Empty(t, client.getCalls)
		assert.Empty(t, client.setCalls)
	})

	t.Run("mutex profiling sets and restores", func(t *testing.T) {
		client := &mockClient{data: map[string]interface{}{"runtime_mutex_profile_fraction": "0"}}
		called := false
		err := ExecWithRuntimeProfilingSettings(func() { called = true }, ProfilingOpts{ProfileMutex: true, ProfileMutexFraction: 5}, client)
		require.NoError(t, err)
		assert.True(t, called)
		assert.Equal(t, []string{"runtime_mutex_profile_fraction"}, client.getCalls)
		require.Len(t, client.setCalls, 2)
		assert.Equal(t, "runtime_mutex_profile_fraction", client.setCalls[0].key)
		assert.Equal(t, "5", client.setCalls[0].value)
		// deferred reset restores the original value
		assert.Equal(t, "runtime_mutex_profile_fraction", client.setCalls[1].key)
		assert.Equal(t, "0", client.setCalls[1].value)
	})

	t.Run("blocking profiling sets and restores", func(t *testing.T) {
		client := &mockClient{data: map[string]interface{}{"runtime_block_profile_rate": "0"}}
		called := false
		err := ExecWithRuntimeProfilingSettings(func() { called = true }, ProfilingOpts{ProfileBlocking: true, ProfileBlockingRate: 10}, client)
		require.NoError(t, err)
		assert.True(t, called)
		assert.Equal(t, []string{"runtime_block_profile_rate"}, client.getCalls)
		require.Len(t, client.setCalls, 2)
		assert.Equal(t, "runtime_block_profile_rate", client.setCalls[0].key)
		assert.Equal(t, "10", client.setCalls[0].value)
		assert.Equal(t, "runtime_block_profile_rate", client.setCalls[1].key)
		assert.Equal(t, "0", client.setCalls[1].value)
	})

	t.Run("get error is returned", func(t *testing.T) {
		client := &mockClient{getErr: errors.New("connection refused")}
		err := ExecWithRuntimeProfilingSettings(func() {}, ProfilingOpts{ProfileMutex: true, ProfileMutexFraction: 5}, client)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get current value")
	})

	t.Run("set error is returned", func(t *testing.T) {
		client := &mockClient{
			data:   map[string]interface{}{"runtime_mutex_profile_fraction": "0"},
			setErr: errors.New("server error"),
		}
		err := ExecWithRuntimeProfilingSettings(func() {}, ProfilingOpts{ProfileMutex: true, ProfileMutexFraction: 5}, client)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to set")
	})
}
