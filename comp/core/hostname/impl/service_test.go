// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostnameimpl

import (
	"context"
	"testing"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	cfg := mock.New(t)
	t.Cleanup(func() {
		// erase cache
		cache.Cache.Delete(cache.BuildAgentKey("hostname"))
		cfg.SetWithoutSource("hostname", "")
	})
	cfg.SetWithoutSource("hostname", "test-hostname")

	lc := compdef.NewTestLifecycle(t)
	provides, err := NewComponent(Requires{Lc: lc})
	require.NoError(t, err)

	s := provides.Comp
	name, err := s.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-hostname", name)
}

func TestGetWithProvider(t *testing.T) {
	cfg := mock.New(t)
	t.Cleanup(func() {
		// erase cache
		cache.Cache.Delete(cache.BuildAgentKey("hostname"))
		cfg.SetWithoutSource("hostname", "")
	})
	cfg.SetWithoutSource("hostname", "test-hostname2")

	lc := compdef.NewTestLifecycle(t)
	provides, err := NewComponent(Requires{Lc: lc})
	require.NoError(t, err)

	s := provides.Comp
	data, err := s.GetWithProvider(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-hostname2", data.Hostname)
	assert.Equal(t, "configuration", data.Provider)
	assert.True(t, data.FromConfiguration())
}

func TestNewHostnameService(t *testing.T) {
	cfg := mock.New(t)
	t.Cleanup(func() {
		cache.Cache.Delete(cache.BuildAgentKey("hostname"))
		cfg.SetWithoutSource("hostname", "")
	})
	cfg.SetWithoutSource("hostname", "test-hostname")

	s := NewHostnameService()
	name, err := s.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-hostname", name)
}

func TestLifecycleHook(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	_, err := NewComponent(Requires{Lc: lc})
	require.NoError(t, err)
	lc.AssertHooksNumber(1)
}
