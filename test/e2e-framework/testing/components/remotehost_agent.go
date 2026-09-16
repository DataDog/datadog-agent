// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package components

import (
	"errors"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclientparams"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/outputs"
)

// RemoteHostAgent represents an Agent running directly on a Host
type RemoteHostAgent struct {
	outputs.HostAgentOutput

	Client        agentclient.Agent
	ClientOptions []agentclientparams.Option
}

var _ common.Initializable = (*RemoteHostAgent)(nil)

// Init is called by e2e test Suite after the component is provisioned.
func (a *RemoteHostAgent) Init(ctx common.Context) (err error) {
	options := a.ClientOptions
	// The installation records the pinned agent binary path when it knows
	// it; the client then invokes that binary directly instead of the
	// sudo datadog-agent wrapper.
	if a.HostAgentOutput.AgentBinPath != "" {
		options = append([]agentclientparams.Option{
			agentclientparams.WithAgentBinPath(a.HostAgentOutput.AgentBinPath),
		}, options...)
	}
	a.Client, err = client.NewHostAgentClientWithParams(ctx, a.HostAgentOutput.Host, options...)
	return err
}

// InitFromHost initializes the Agent client with the context of an already
// initialized host. It is used when an Agent is added to a live environment.
func (a *RemoteHostAgent) InitFromHost(host *RemoteHost) error {
	if host == nil || host.context == nil {
		return errors.New("initializing Agent client: host is not initialized")
	}
	return a.Init(host.context)
}
