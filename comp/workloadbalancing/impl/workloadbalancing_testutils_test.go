// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancingimpl

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

// failingHostname stands in for a host whose name cannot be resolved.
type failingHostname struct{}

func (failingHostname) Get(context.Context) (string, error) {
	return "", errors.New("cannot resolve the hostname")
}

func (failingHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{}, errors.New("cannot resolve the hostname")
}

func (failingHostname) GetSafe(context.Context) string {
	return "unknown host"
}

func newTestWorkloadBalancingComponent(t *testing.T, agentConfigs map[string]interface{}, logger log.Component) Provides {
	if logger == nil {
		logger = logmock.New(t)
	}
	agentConfigComponent := config.NewMockWithOverrides(t, agentConfigs)

	requires := Requires{
		Logger:      logger,
		AgentConfig: agentConfigComponent,
		Hostname:    hostnameimpl.NewHostnameService(),
	}

	provides, err := NewComponent(requires)
	require.NoError(t, err)
	return provides
}
