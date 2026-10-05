// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// collectStaticConfigServices runs createServices synchronously and returns the
// AD identifiers of the emitted services.
func collectStaticConfigServices(l *StaticConfigListener) []string {
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

func TestStaticConfigListenerSysProbeChecks(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
		expected  []string
		absent    []string
	}{
		{
			name:      "nothing enabled",
			overrides: map[string]interface{}{},
			absent:    []string{"_oom_kill", "_tcp_queue_length"},
		},
		{
			name:      "oom_kill enabled",
			overrides: map[string]interface{}{"system_probe_config.enable_oom_kill": true},
			expected:  []string{"_oom_kill"},
			absent:    []string{"_tcp_queue_length"},
		},
		{
			name:      "tcp_queue_length enabled",
			overrides: map[string]interface{}{"system_probe_config.enable_tcp_queue_length": true},
			expected:  []string{"_tcp_queue_length"},
			absent:    []string{"_oom_kill"},
		},
		{
			name: "both enabled",
			overrides: map[string]interface{}{
				"system_probe_config.enable_oom_kill":         true,
				"system_probe_config.enable_tcp_queue_length": true,
			},
			expected: []string{"_oom_kill", "_tcp_queue_length"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configmock.New(t)
			sysProbeCfg := configmock.NewSystemProbe(t)
			for k, v := range tc.overrides {
				sysProbeCfg.SetInTest(k, v)
			}

			ids := collectStaticConfigServices(&StaticConfigListener{})

			for _, id := range tc.expected {
				assert.Contains(t, ids, id)
			}
			for _, id := range tc.absent {
				assert.NotContains(t, ids, id)
			}
		})
	}
}
