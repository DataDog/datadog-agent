// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package checks

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/config"
)

// Containers will never be enabled on environments other than linux or windows, so
// we must make sure that the build tags in this file match.

func TestContainerCheck(t *testing.T) {
	scfg := &sysconfig.Config{}

	// Make sure the container check can be enabled if the process check is disabled
	t.Run("containers enabled; rt enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", false)
		config.SetFeatures(t, config.Docker)

		enabledChecks := getEnabledChecks(scfg)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertContainsCheck(t, enabledChecks, RTContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, ProcessCheckName)
	})

	// Make sure that disabling RT disables the rt container check
	t.Run("containers enabled; rt disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)
		cfg.Set("process_config.disable_realtime_checks", true)
		config.SetFeatures(t, config.Docker)

		enabledChecks := getEnabledChecks(scfg)
		assertContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure the container check cannot be enabled if we cannot access containers
	t.Run("cannot access containers", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", false)
		cfg.Set("process_config.container_collection.enabled", true)

		enabledChecks := getEnabledChecks(scfg)

		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})

	// Make sure the container and process check are mutually exclusive
	t.Run("mutual exclusion", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.Set("process_config.process_collection.enabled", true)
		cfg.Set("process_config.container_collection.enabled", true)
		config.SetFeatures(t, config.Docker)

		enabledChecks := getEnabledChecks(scfg)
		assertContainsCheck(t, enabledChecks, ProcessCheckName)
		assertNotContainsCheck(t, enabledChecks, ContainerCheckName)
		assertNotContainsCheck(t, enabledChecks, RTContainerCheckName)
	})
}

func TestDisableRealTime(t *testing.T) {
	tests := []struct {
		name            string
		disableRealtime bool
		expectedChecks  []string
	}{
		{
			name:            "true",
			disableRealtime: true,
			expectedChecks:  []string{ContainerCheckName},
		},
		{
			name:            "false",
			disableRealtime: false,
			expectedChecks:  []string{ContainerCheckName, RTContainerCheckName},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)

			mockConfig := config.Mock(t)
			mockConfig.Set("process_config.disable_realtime_checks", tc.disableRealtime)
			mockConfig.Set("process_config.process_discovery.enabled", false) // Not an RT check so we don't care
			config.SetFeatures(t, config.Docker)

			enabledChecks := getEnabledChecks(&sysconfig.Config{})
			assert.EqualValues(tc.expectedChecks, enabledChecks)
		})
	}
}
