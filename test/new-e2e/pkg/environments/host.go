// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// Host is an environment that contains a Host, FakeIntake and Agent configured to talk to each other.
type Host struct {
	// Components
	RemoteHost *components.RemoteHost
	FakeIntake *components.FakeIntake
	Agent      *components.RemoteHostAgent
}

var _ e2e.Initializable = &Host{}

// Init initializes the environment
func (e *Host) Init(ctx e2e.Context) error {
	if e.Agent != nil {
		agent, err := client.NewHostAgentClient(ctx.T(), e.RemoteHost, true)
		if err != nil {
			return err
		}
		e.Agent.Client = agent
	}

	return nil
}
