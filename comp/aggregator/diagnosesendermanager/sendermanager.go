// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package diagnosesendermanager

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
	Log    log.Component
	Config config.Component
}

type diagnoseSenderManager struct {
	senderManager    util.Optional[sender.SenderManager]
	deps             dependencies
	hostnameDetected string
}

func newDiagnoseSenderManager(deps dependencies) (Component, error) {
	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return nil, deps.Log.Errorf("Error while getting hostname, exiting: %v", err)
	}

	return &diagnoseSenderManager{deps: deps, hostnameDetected: hostnameDetected}, nil
}

// LazyGetSenderManager gets an instance of SenderManager lazily.
func (sender *diagnoseSenderManager) LazyGetSenderManager() sender.SenderManager {
	senderManager, found := sender.senderManager.Get()
	if found {
		return senderManager
	}

	// Initializing the aggregator with a flush interval of 0 (to disable the flush goroutines)
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 0
	opts.DontStartForwarders = true
	opts.UseNoopEventPlatformForwarder = true
	opts.UseNoopOrchestratorForwarder = true

	log := sender.deps.Log
	config := sender.deps.Config
	forwarder := defaultforwarder.NewDefaultForwarder(config, log, defaultforwarder.NewOptions(config, log, nil))
	senderManager = aggregator.InitAndStartAgentDemultiplexer(
		log,
		forwarder,
		opts,
		sender.hostnameDetected)

	sender.senderManager.Set(senderManager)
	return senderManager
}
