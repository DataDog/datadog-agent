// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package remote

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSSHClient_RetrieveRunningConfig_Success(t *testing.T) {
	expectedConfig := `
version 15.1
hostname Router1
interface GigabitEthernet0/1
 ip address 192.168.1.1 255.255.255.0
end`

	session := &mockSSHSession{
		outputs: map[string]string{
			"show running-config": expectedConfig,
		},
	}

	client := &MockSSHClient{
		session: session,
	}

	config, err := client.RetrieveRunningConfig()

	assert.NoError(t, err)
	assert.Equal(t, expectedConfig, config)
	assert.True(t, session.closed, "Session should be closed after use")
}

func TestSSHClient_RetrieveStartupConfig_Success(t *testing.T) {
	expectedConfig := `
version 15.1
hostname Router1
interface GigabitEthernet0/1
 ip address 192.168.1.1 255.255.255.0
end`

	session := &mockSSHSession{
		outputs: map[string]string{
			"show startup-config": expectedConfig,
		},
	}

	client := &MockSSHClient{
		session: session,
	}

	config, err := client.RetrieveStartupConfig()

	assert.NoError(t, err)
	assert.Equal(t, expectedConfig, config)
	assert.True(t, session.closed, "Session should be closed after use")
}

func TestSSHClient_RetrieveConfig_SessionCreationFailure(t *testing.T) {
	client := &MockSSHClient{
		sessionError: fmt.Errorf("failed to create SSH session"),
	}

	_, err := client.RetrieveRunningConfig()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SSH session")
}

func TestSSHClient_RetrieveConfig_CommandExecutionFailure(t *testing.T) {
	session := &mockSSHSession{
		err: fmt.Errorf("command execution failed"),
	}

	client := &MockSSHClient{
		session: session,
	}

	_, err := client.RetrieveRunningConfig()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command execution failed")
	assert.True(t, session.closed, "Session should be closed even on failure")
}

func TestSSHClient_MultipleCommands(t *testing.T) {
	session := &mockSSHSession{
		outputs: map[string]string{
			"show version":    "Cisco IOS Software Version 15.1",
			"show interfaces": "GigabitEthernet0/1 is up, line protocol is up",
			"show ip route":   "Gateway of last resort is not set",
		},
	}

	client := &MockSSHClient{
		session: session,
	}

	commands := []string{"show version", "show interfaces", "show ip route"}
	results, err := client.retrieveConfiguration(commands)

	assert.NoError(t, err)
	assert.Contains(t, results, "Cisco IOS Software Version 15.1")
	assert.Contains(t, results, "GigabitEthernet0/1 is up, line protocol is up")
	assert.Contains(t, results, "Gateway of last resort is not set")
	assert.True(t, session.closed, "Session should be closed after use")
}
