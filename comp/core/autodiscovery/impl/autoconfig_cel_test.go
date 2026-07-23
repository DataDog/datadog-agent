// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver && cel && test

package autodiscoveryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adtypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/common/types"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

func TestProcessNewConfigCELCompileErrorStoredInStats(t *testing.T) {
	_, ac := getResolveTestConfig(t)

	// Reset errorStats to ensure a clean state
	errorStats = newAcErrorStats()

	invalidTpl := integration.Config{
		Name: "bad-cel-check",
		CELSelector: workloadfilter.Rules{
			Containers: []string{`this is not valid CEL !!!`},
		},
	}

	changes := ac.processNewConfig(invalidTpl)

	// processNewConfig should return empty changes when config cannot be initialized
	assert.Empty(t, changes.Schedule)
	assert.Empty(t, changes.Unschedule)

	// The CEL compile error should be stored in errorStats
	errors := GetConfigErrors()
	assert.Contains(t, errors, "bad-cel-check")
}

func TestInitializeConfigurationADIDInjection(t *testing.T) {
	_, ac := getResolveTestConfig(t)

	t.Run("ad_identifiers + process CEL injects cel://process", func(t *testing.T) {
		config := integration.Config{
			Name:          "redis",
			ADIdentifiers: []string{"redis"},
			CELSelector: workloadfilter.Rules{
				Processes: []string{`process.cmdline.contains("redis-server")`},
			},
		}
		err := ac.initializeConfiguration(&config)
		require.NoError(t, err)
		assert.Equal(t, []string{"redis", string(adtypes.CelProcessIdentifier)}, config.ADIdentifiers)
	})

	t.Run("no ADIDs + process CEL injects cel://process", func(t *testing.T) {
		config := integration.Config{
			Name: "redis",
			CELSelector: workloadfilter.Rules{
				Processes: []string{`process.cmdline.contains("redis-server")`},
			},
		}
		err := ac.initializeConfiguration(&config)
		require.NoError(t, err)
		assert.Equal(t, []string{string(adtypes.CelProcessIdentifier)}, config.ADIdentifiers)
	})

	t.Run("no ADIDs + container CEL injects cel://container", func(t *testing.T) {
		config := integration.Config{
			Name: "redis",
			CELSelector: workloadfilter.Rules{
				Containers: []string{`container.image.reference.contains("redis")`},
			},
		}
		err := ac.initializeConfiguration(&config)
		require.NoError(t, err)
		assert.Equal(t, []string{string(adtypes.CelContainerIdentifier)}, config.ADIdentifiers)
	})

	t.Run("ad_identifiers + container CEL does not inject cel://container", func(t *testing.T) {
		config := integration.Config{
			Name:          "redis",
			ADIdentifiers: []string{"redis"},
			CELSelector: workloadfilter.Rules{
				Containers: []string{`container.image.reference.contains("redis")`},
			},
		}
		err := ac.initializeConfiguration(&config)
		require.NoError(t, err)
		// cel://container should NOT be injected when ad_identifiers are present
		assert.Equal(t, []string{"redis"}, config.ADIdentifiers)
	})

	t.Run("ad_identifiers + both container and process CEL injects only cel://process", func(t *testing.T) {
		config := integration.Config{
			Name:          "redis",
			ADIdentifiers: []string{"redis"},
			CELSelector: workloadfilter.Rules{
				Containers: []string{`container.image.reference.contains("redis")`},
				Processes:  []string{`process.cmdline.contains("redis-server")`},
			},
		}
		err := ac.initializeConfiguration(&config)
		require.NoError(t, err)
		// Only cel://process should be injected (container respects existing ad_identifiers)
		assert.Equal(t, []string{"redis", string(adtypes.CelProcessIdentifier)}, config.ADIdentifiers)
	})

	t.Run("no ADIDs + both container and process CEL injects both", func(t *testing.T) {
		config := integration.Config{
			Name: "redis",
			CELSelector: workloadfilter.Rules{
				Containers: []string{`container.image.reference.contains("redis")`},
				Processes:  []string{`process.cmdline.contains("redis-server")`},
			},
		}
		err := ac.initializeConfiguration(&config)
		require.NoError(t, err)
		assert.Contains(t, config.ADIdentifiers, string(adtypes.CelContainerIdentifier))
		assert.Contains(t, config.ADIdentifiers, string(adtypes.CelProcessIdentifier))
		assert.Len(t, config.ADIdentifiers, 2)
	})

	t.Run("no CEL rules leaves ADIDs unchanged", func(t *testing.T) {
		config := integration.Config{
			Name:          "redis",
			ADIdentifiers: []string{"redis"},
		}
		err := ac.initializeConfiguration(&config)
		require.NoError(t, err)
		assert.Equal(t, []string{"redis"}, config.ADIdentifiers)
	})
}
