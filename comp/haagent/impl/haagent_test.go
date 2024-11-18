package haagentimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Enabled(t *testing.T) {
	tests := []struct {
		name            string
		configs         map[string]interface{}
		expectedEnabled bool
	}{
		{
			name: "enabled",
			configs: map[string]interface{}{
				"ha_agent.enabled": true,
			},
			expectedEnabled: true,
		},
		{
			name: "disabled",
			configs: map[string]interface{}{
				"ha_agent.enabled": false,
			},
			expectedEnabled: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			haAgent := newTestHaAgentComponent(t, tt.configs)
			assert.Equal(t, tt.expectedEnabled, haAgent.Enabled())
		})
	}
}

func Test_GetGroup(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"ha_agent.group": "my-group-01",
	}
	haAgent := newTestHaAgentComponent(t, agentConfigs)
	assert.Equal(t, "my-group-01", haAgent.GetGroup())
}

func Test_IsLeader_SetLeader(t *testing.T) {
	agentConfigs := map[string]interface{}{
		"hostname": "my-agent-hostname",
	}
	haAgent := newTestHaAgentComponent(t, agentConfigs)

	haAgent.SetLeader("another-agent")
	assert.False(t, haAgent.IsLeader())

	haAgent.SetLeader("my-agent-hostname")
	assert.True(t, haAgent.IsLeader())
}
