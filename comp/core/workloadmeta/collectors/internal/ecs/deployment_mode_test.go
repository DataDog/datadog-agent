// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestDetermineDeploymentMode(t *testing.T) {
	tests := []struct {
		name         string
		configValue  string
		isFargate    bool
		expectedMode deploymentMode
	}{
		{
			name:         "explicit daemon mode",
			configValue:  "daemon",
			isFargate:    false,
			expectedMode: deploymentModeDaemon,
		},
		{
			name:         "explicit sidecar mode",
			configValue:  "sidecar",
			isFargate:    false,
			expectedMode: deploymentModeSidecar,
		},
		{
			name:         "auto mode on EC2 defaults to daemon",
			configValue:  "auto",
			isFargate:    false,
			expectedMode: deploymentModeDaemon,
		},
		{
			name:         "auto mode on Fargate defaults to sidecar",
			configValue:  "auto",
			isFargate:    true,
			expectedMode: deploymentModeSidecar,
		},
		{
			name:         "unknown mode on EC2 defaults to daemon",
			configValue:  "unknown",
			isFargate:    false,
			expectedMode: deploymentModeDaemon,
		},
		{
			name:         "unknown mode on Fargate defaults to sidecar",
			configValue:  "unknown",
			isFargate:    true,
			expectedMode: deploymentModeSidecar,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock config
			mockConfig := model.NewConfig("test", "DD", nil)
			mockConfig.Set("ecs_deployment_mode", tt.configValue, model.SourceAgentRuntime)

			// Setup feature flags
			if tt.isFargate {
				env.RegisterFeature(env.ECSFargate)
			} else {
				env.UnregisterFeature(env.ECSFargate)
			}

			// Create collector with mock config
			c := &collector{
				config: config.Component(mockConfig),
			}

			// Test
			mode := c.determineDeploymentMode()
			assert.Equal(t, tt.expectedMode, mode)

			// Cleanup
			if tt.isFargate {
				env.UnregisterFeature(env.ECSFargate)
			}
		})
	}
}

func TestDetectLaunchType(t *testing.T) {
	tests := []struct {
		name               string
		isFargate          bool
		deploymentMode     deploymentMode
		expectedLaunchType workloadmeta.ECSLaunchType
	}{
		{
			name:               "Fargate environment detected",
			isFargate:          true,
			deploymentMode:     deploymentModeSidecar,
			expectedLaunchType: workloadmeta.ECSLaunchTypeFargate,
		},
		{
			name:               "EC2 daemon mode",
			isFargate:          false,
			deploymentMode:     deploymentModeDaemon,
			expectedLaunchType: workloadmeta.ECSLaunchTypeEC2,
		},
		{
			name:               "EC2 sidecar mode",
			isFargate:          false,
			deploymentMode:     deploymentModeSidecar,
			expectedLaunchType: workloadmeta.ECSLaunchTypeEC2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup feature flags
			if tt.isFargate {
				env.RegisterFeature(env.ECSFargate)
			} else {
				env.UnregisterFeature(env.ECSFargate)
			}

			// Create collector
			c := &collector{
				deploymentMode: tt.deploymentMode,
			}

			// Test (context not used when env var is set)
			launchType := c.detectLaunchType(nil)
			assert.Equal(t, tt.expectedLaunchType, launchType)

			// Cleanup
			if tt.isFargate {
				env.UnregisterFeature(env.ECSFargate)
			}
		})
	}
}
