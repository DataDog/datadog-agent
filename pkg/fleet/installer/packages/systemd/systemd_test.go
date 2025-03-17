// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package systemd offers an interface over systemd
package systemd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	expectedAgentExpUnit = `[Unit]
Description=Datadog Agent Experiment
After=network.target
OnFailure=datadog-agent.service
JobTimeoutSec=3000
Wants=datadog-agent-installer-exp.service datadog-agent-trace-exp.service datadog-agent-process-exp.service datadog-agent-sysprobe-exp.service datadog-agent-security-exp.service
Conflicts=datadog-agent.service
Before=datadog-agent.service

[Service]
Type=oneshot
PIDFile=/opt/datadog-packages/datadog-agent/experiment/run/agent.pid
User=dd-agent
EnvironmentFile=-/etc/datadog-agent/environment
Environment="DD_FLEET_POLICIES_DIR=/etc/datadog-agent/managed/datadog-agent/experiment"
ExecStart=/opt/datadog-packages/datadog-agent/experiment/bin/agent/agent run -p /opt/datadog-packages/datadog-agent/experiment/run/agent.pid
ExecStart=/bin/false
ExecStop=/bin/false
RuntimeDirectory=datadog

[Install]
WantedBy=multi-user.target
`

	expectedProcessAgentExpUnit = `[Unit]
Description=Datadog Process Agent Experiment
After=network.target
BindsTo=datadog-agent-exp.service

[Service]
Type=simple
PIDFile=/opt/datadog-packages/datadog-agent/experiment/run/process-agent.pid
User=dd-agent
Restart=on-failure
EnvironmentFile=-/etc/datadog-agent/environment
Environment="DD_FLEET_POLICIES_DIR=/etc/datadog-agent/managed/datadog-agent/experiment"
ExecStart=/opt/datadog-packages/datadog-agent/experiment/embedded/bin/process-agent --cfgpath=/etc/datadog-agent/datadog.yaml --sysprobe-config=/etc/datadog-agent/system-probe.yaml --pid=/opt/datadog-packages/datadog-agent/experiment/run/process-agent.pid
# Since systemd 229, should be in [Unit] but in order to support systemd <229,
# it is also supported to have it here.
StartLimitInterval=10
StartLimitBurst=5

[Install]
WantedBy=multi-user.target
`
)

func TestGenerateExperimentUnit(t *testing.T) {
	agentExpUnit, err := generateExperimentUnit(context.Background(), "datadog-agent-exp.service")
	assert.NoError(t, err)
	assert.Equal(t, string(expectedAgentExpUnit), string(agentExpUnit), "generated content for datadog-agent-exp.service does not match expected content")

	processAgentExpUnit, err := generateExperimentUnit(context.Background(), "datadog-agent-process-exp.service")
	assert.NoError(t, err)
	assert.Equal(t, string(expectedProcessAgentExpUnit), string(processAgentExpUnit), "generated content for datadog-agent-process-exp.service does not match expected content")
}
