// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// collectEnvironmentServices runs createServices synchronously and returns the AD
// identifiers of the emitted services.
func collectEnvironmentServices(l *EnvironmentListener) []string {
	ch := make(chan Service, 16)
	l.newService = ch
	l.createServices()
	close(ch)

	var ids []string
	for svc := range ch {
		ids = append(ids, svc.GetServiceID())
	}
	return ids
}

func TestEnvironmentListenerSysProbeChecks(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]any
		expected  []string
		absent    []string
	}{
		{
			name:      "nothing enabled",
			overrides: map[string]any{},
			absent:    []string{"_oom_kill", "_tcp_queue_length"},
		},
		{
			name:      "oom_kill enabled",
			overrides: map[string]any{"system_probe_config.enable_oom_kill": true},
			expected:  []string{"_oom_kill"},
			absent:    []string{"_tcp_queue_length"},
		},
		{
			name:      "tcp_queue_length enabled",
			overrides: map[string]any{"system_probe_config.enable_tcp_queue_length": true},
			expected:  []string{"_tcp_queue_length"},
			absent:    []string{"_oom_kill"},
		},
		{
			name: "both enabled",
			overrides: map[string]any{
				"system_probe_config.enable_oom_kill":         true,
				"system_probe_config.enable_tcp_queue_length": true,
			},
			expected: []string{"_oom_kill", "_tcp_queue_length"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sysProbeCfg := sysprobeconfigmock.NewMockWithOverrides(t, tc.overrides)
			l := &EnvironmentListener{sysProbeConfig: option.New(sysProbeCfg)}

			ids := collectEnvironmentServices(l)

			for _, id := range tc.expected {
				assert.Contains(t, ids, id)
			}
			for _, id := range tc.absent {
				assert.NotContains(t, ids, id)
			}
		})
	}
}

// TestEnvironmentListenerNoSysProbeConfig ensures the listener does not panic and
// emits no system-probe based service when the sysprobeconfig component is absent.
func TestEnvironmentListenerNoSysProbeConfig(t *testing.T) {
	l := &EnvironmentListener{sysProbeConfig: option.None[sysprobeconfig.Component]()}

	ids := collectEnvironmentServices(l)

	assert.NotContains(t, ids, "_oom_kill")
	assert.NotContains(t, ids, "_tcp_queue_length")
}
