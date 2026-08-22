// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
)

func newTestWorkloadBalancingComponent(t *testing.T, agentConfigs map[string]interface{}, haAgent haagent.Component, logger log.Component) Provides {
	if logger == nil {
		logger = logmock.New(t)
	}
	if haAgent == nil {
		haAgent = haagentmock.NewMockHaAgent()
	}
	agentConfigComponent := config.NewMockWithOverrides(t, agentConfigs)

	requires := Requires{
		Logger:      logger,
		AgentConfig: agentConfigComponent,
		Hostname:    hostnameimpl.NewHostnameService(),
		HaAgent:     haAgent,
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)
	return provides
}
