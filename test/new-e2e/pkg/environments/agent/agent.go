// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awsdocker contains the definition of the AWS Docker environment.
package agentenv

import (
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	remoteComp "github.com/DataDog/test-infra-definitions/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type HostAgentParams struct {
	// PulumiContext is the Pulumi context to use.
	PulumiContext *pulumi.Context
	// CommonEnvironment is the common environment to use.
	CommonEnvironment *config.CommonEnvironment
	// Host is the host where we want to install the agent.
	Host *remoteComp.Host
	// Options is a list of options to configure the agent.
	Options []agentparams.Option
	// Importer is the component that will be used to import the agent from pulumi.
	Importer *agent.HostAgentOutput
}

func NewAgentOnHost(params HostAgentParams) (hostAgent *agent.HostAgent, err error) {
	hostAgent, err = agent.NewHostAgent(params.CommonEnvironment, params.Host, params.Options...)
	if err != nil {
		return nil, err
	}

	if params.Importer == nil {
		// nothing to export to
		return hostAgent, nil
	}

	err = hostAgent.Export(params.PulumiContext, params.Importer)
	if err != nil {
		return nil, err
	}
	return hostAgent, nil
}
