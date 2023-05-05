// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package flare

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type AgentTaskProvider struct {
	client    *remote.Client
	flareComp flare.Component

	flareTaskProcessed map[string]bool
}

const (
	agentTaskFlareType = "flare"
)

func NewAgentTaskProvider(flareComp flare.Component, name string, agentVersion string) (*AgentTaskProvider, error) {
	c, err := remote.NewUnverifiedGRPCClient(
		name, agentVersion, []data.Product{data.ProductAgentTask}, 1*time.Second,
	)
	if err != nil {
		return nil, err
	}

	return &AgentTaskProvider{
		client:             c,
		flareTaskProcessed: map[string]bool{},
	}, nil
}

func (a *AgentTaskProvider) Start() {
	// TODO fix testing to put a non-nil client
	if a.client != nil {
		a.client.RegisterAgentTaskUpdate(a.agentTaskUpdateCallback)

		a.client.Start()
	}
}

func (a *AgentTaskProvider) agentTaskUpdateCallback(configs map[string]state.AgentTaskConfig) {
	for configID, c := range configs {
		if c.Config.TaskType == agentTaskFlareType {
			// Check that the flare task wasn't already processed
			if !a.flareTaskProcessed[c.Config.UUID] {
				a.flareTaskProcessed[c.Config.UUID] = true
				log.Debugf("Running agent flare task %s for %s", c.Config.UUID, configID)

				filePath, err := a.flareComp.Create(nil, nil)

				log.Warnf("Created a flare with error %s at path %s", err, filePath)
			}
		}
	}
}
