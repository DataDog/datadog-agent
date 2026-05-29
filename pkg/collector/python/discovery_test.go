// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func TestDiscoverConfig(t *testing.T) {
	testDiscoverConfig(t)
}

func TestDiscoverConfigNoConfigs(t *testing.T) {
	testDiscoverConfigNoConfigs(t)
}

func TestDiscoverConfigRtloaderError(t *testing.T) {
	testDiscoverConfigRtloaderError(t)
}

func TestDiscoverConfigMalformedResult(t *testing.T) {
	testDiscoverConfigMalformedResult(t)
}

func TestParseDiscoveryResult(t *testing.T) {
	t.Run("null", func(t *testing.T) {
		configs, err := parseDiscoveryResult("fake_check", "null")
		require.NoError(t, err)
		assert.Empty(t, configs)
	})

	t.Run("empty array", func(t *testing.T) {
		configs, err := parseDiscoveryResult("fake_check", "[]")
		require.NoError(t, err)
		assert.Empty(t, configs)
	})

	t.Run("valid array", func(t *testing.T) {
		configs, err := parseDiscoveryResult("fake_check", `[{"host":"127.0.0.1"},{"host":"127.0.0.2"}]`)
		require.NoError(t, err)
		assert.Equal(t, []integration.Data{
			integration.Data(`{"host":"127.0.0.1"}`),
			integration.Data(`{"host":"127.0.0.2"}`),
		}, configs)
	})

	t.Run("malformed", func(t *testing.T) {
		configs, err := parseDiscoveryResult("fake_check", "{")
		require.Error(t, err)
		assert.Nil(t, configs)
	})
}
