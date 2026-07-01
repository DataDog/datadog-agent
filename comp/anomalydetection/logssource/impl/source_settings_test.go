// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logssourceimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	compConfig "github.com/DataDog/datadog-agent/comp/core/config"
)

func TestLogSourceSettings(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
		want      logSourceSettings
	}{
		{
			name: "defaults",
			want: logSourceSettings{
				logsEnabled:             true,
				containerSourcesEnabled: true,
				kubeletSourceEnabled:    true,
			},
		},
		{
			name: "container sources disabled",
			overrides: map[string]interface{}{
				anomalyDetectionLogsContainersEnabledKey: false,
			},
			want: logSourceSettings{
				logsEnabled:             true,
				containerSourcesEnabled: false,
				kubeletSourceEnabled:    true,
			},
		},
		{
			name: "kubelet source disabled",
			overrides: map[string]interface{}{
				anomalyDetectionLogsKubeletEnabledKey: false,
			},
			want: logSourceSettings{
				logsEnabled:             true,
				containerSourcesEnabled: true,
				kubeletSourceEnabled:    false,
			},
		},
		{
			name: "parent logs disabled",
			overrides: map[string]interface{}{
				anomalyDetectionLogsEnabledKey: false,
			},
			want: logSourceSettings{
				logsEnabled:             false,
				containerSourcesEnabled: true,
				kubeletSourceEnabled:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := compConfig.NewMockWithOverrides(t, tt.overrides)
			assert.Equal(t, tt.want, newLogSourceSettings(cfg))
		})
	}
}

func TestLogSourceSettingsShouldStart(t *testing.T) {
	defaultSettings := logSourceSettings{
		logsEnabled:             true,
		containerSourcesEnabled: true,
		kubeletSourceEnabled:    true,
	}

	tests := []struct {
		name                  string
		settings              logSourceSettings
		observerAvailable     bool
		workloadmetaAvailable bool
		analysisEnabled       bool
		recordingEnabled      bool
		want                  bool
	}{
		{
			name:                  "all sources enabled",
			settings:              defaultSettings,
			observerAvailable:     true,
			workloadmetaAvailable: true,
			analysisEnabled:       true,
			want:                  true,
		},
		{
			name: "kubelet only does not require workloadmeta",
			settings: logSourceSettings{
				logsEnabled:             true,
				containerSourcesEnabled: false,
				kubeletSourceEnabled:    true,
			},
			observerAvailable: true,
			analysisEnabled:   true,
			want:              true,
		},
		{
			name: "container only requires workloadmeta",
			settings: logSourceSettings{
				logsEnabled:             true,
				containerSourcesEnabled: true,
				kubeletSourceEnabled:    false,
			},
			observerAvailable:     true,
			workloadmetaAvailable: true,
			analysisEnabled:       true,
			want:                  true,
		},
		{
			name: "container only skips without workloadmeta",
			settings: logSourceSettings{
				logsEnabled:             true,
				containerSourcesEnabled: true,
				kubeletSourceEnabled:    false,
			},
			observerAvailable: true,
			analysisEnabled:   true,
			want:              false,
		},
		{
			name: "both source gates disabled",
			settings: logSourceSettings{
				logsEnabled: true,
			},
			observerAvailable:     true,
			workloadmetaAvailable: true,
			analysisEnabled:       true,
			want:                  false,
		},
		{
			name: "parent logs disabled without recording",
			settings: logSourceSettings{
				containerSourcesEnabled: true,
				kubeletSourceEnabled:    true,
			},
			observerAvailable:     true,
			workloadmetaAvailable: true,
			analysisEnabled:       true,
			want:                  false,
		},
		{
			name:                  "analysis disabled without recording",
			settings:              defaultSettings,
			observerAvailable:     true,
			workloadmetaAvailable: true,
			want:                  false,
		},
		{
			name: "recording starts when analysis and parent logs are disabled",
			settings: logSourceSettings{
				containerSourcesEnabled: true,
				kubeletSourceEnabled:    true,
			},
			observerAvailable:     true,
			workloadmetaAvailable: true,
			recordingEnabled:      true,
			want:                  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.settings.shouldStart(
				tt.observerAvailable,
				tt.workloadmetaAvailable,
				tt.analysisEnabled,
				tt.recordingEnabled,
			))
		})
	}
}
