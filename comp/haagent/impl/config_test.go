// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagentimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"
)

func Test_newHaAgentConfigs(t *testing.T) {
	tests := []struct {
		name           string
		agentConfigs   map[string]interface{}
		expectedConfig *haAgentConfigs
	}{
		{
			name: "ok",
			agentConfigs: map[string]interface{}{
				"ha_agent.enabled": true,
				"ha_agent.group":   "mygroup01",
				"ha_agent.integrations": []integrationConfig{
					{Name: "snmp"},
					{Name: "mysql"},
				},
			},
			expectedConfig: &haAgentConfigs{
				enabled: true,
				group:   "mygroup01",
				isHaIntegrationMap: map[string]bool{
					"snmp":  true,
					"mysql": true,
				},
			},
		},
		{
			name: "invalid integrations",
			agentConfigs: map[string]interface{}{
				"ha_agent.enabled":      true,
				"ha_agent.group":        "mygroup01",
				"ha_agent.integrations": "abc",
			},
			expectedConfig: &haAgentConfigs{
				enabled:            true,
				group:              "mygroup01",
				isHaIntegrationMap: map[string]bool{},
			},
		},
		{
			name: "empty integrationConfig name is skipped",
			agentConfigs: map[string]interface{}{
				"ha_agent.enabled": true,
				"ha_agent.group":   "mygroup01",
				"ha_agent.integrations": []integrationConfig{
					{Name: "snmp"},
					{Name: "mysql"},
					{Name: ""},
				},
			},
			expectedConfig: &haAgentConfigs{
				enabled: true,
				group:   "mygroup01",
				isHaIntegrationMap: map[string]bool{
					"snmp":  true,
					"mysql": true,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logComponent := logmock.New(t)
			agentConfigComponent := fxutil.Test[config.Component](t, fx.Options(
				config.MockModule(),
				fx.Replace(config.MockParams{Overrides: tt.agentConfigs}),
			))
			assert.Equal(t, tt.expectedConfig, newHaAgentConfigs(agentConfigComponent, logComponent))
		})
	}
}
