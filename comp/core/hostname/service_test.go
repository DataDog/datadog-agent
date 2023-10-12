// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostname

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	t.Cleanup(func() {
		// erase cache
		cache.Cache.Delete(cache.BuildAgentKey("hostname"))
		config.Datadog.Set("hostname", "")
	})
	config.Datadog.Set("hostname", "test-hostname")
	s := fxutil.Test[Component](t, Module)
	name, err := s.Get(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-hostname", name)
}

func TestGetWithProvider(t *testing.T) {
	t.Cleanup(func() {
		// erase cache)
		cache.Cache.Delete(cache.BuildAgentKey("hostname"))
		config.Datadog.Set("hostname", "")
	})
	config.Datadog.Set("hostname", "test-hostname2")
	s := fxutil.Test[Component](t, Module)
	data, err := s.GetWithProvider(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-hostname2", data.Hostname)
	assert.Equal(t, "configuration", data.Provider)
	assert.True(t, data.FromConfiguration())
}
