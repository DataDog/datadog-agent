// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package demultiplexer

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
	Log             log.Component
	SharedForwarder defaultforwarder.Component

	Params Params
}

type demultiplexer struct {
	*aggregator.AgentDemultiplexer
}

type provides struct {
	fx.Out
	Comp Component

	// Implement also diagnosesendermanager.Component to make sure
	// either demultiplexer.Component nor diagnosesendermanager.Component is created.
	SenderManager diagnosesendermanager.Component
}

func newDemultiplexer(deps dependencies) (provides, error) {
	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return provides{}, deps.Log.Errorf("Error while getting hostname, exiting: %v", err)
	}

	agentDemultiplexer := aggregator.InitAndStartAgentDemultiplexer(
		deps.Log,
		deps.SharedForwarder,
		deps.Params.Options,
		hostnameDetected)
	demultiplexer := demultiplexer{
		AgentDemultiplexer: agentDemultiplexer,
	}

	return provides{
		Comp:          demultiplexer,
		SenderManager: demultiplexer,
	}, nil
}

// LazyGetSenderManager gets an instance of SenderManager lazily.
func (demux demultiplexer) LazyGetSenderManager() sender.SenderManager {
	return demux
}
