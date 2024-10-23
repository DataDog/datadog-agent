// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package diagnosesendermanagerimpl defines the sender manager for the local diagnose check
package diagnosesendermanagerimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/diagnosesendermanager"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newDiagnoseSenderManager))
}

type dependencies struct {
	fx.In
	Log        log.Component
	Config     config.Component
	Hostname   hostname.Component
	Compressor compression.Component
	Tagger     tagger.Component
}

type diagnoseSenderManager struct {
	senderManager optional.Option[sender.SenderManager]
	deps          dependencies
}

func newDiagnoseSenderManager(deps dependencies) diagnosesendermanager.Component {
	return &diagnoseSenderManager{deps: deps}
}

// LazyGetSenderManager gets an instance of SenderManager lazily.
func (sender *diagnoseSenderManager) LazyGetSenderManager() (sender.SenderManager, error) {
	senderManager, found := sender.senderManager.Get()
	if found {
		return senderManager, nil
	}

	hostnameDetected, err := sender.deps.Hostname.Get(context.TODO())
	if err != nil {
		return nil, sender.deps.Log.Errorf("Error while getting hostname, exiting: %v", err)
	}

	// Initializing the aggregator with a flush interval of 0 (to disable the flush goroutines)
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.FlushInterval = 0
	opts.DontStartForwarders = true

	log := sender.deps.Log
	config := sender.deps.Config
	forwarder := defaultforwarder.NewDefaultForwarder(config, log, defaultforwarder.NewOptions(config, log, nil))
	orchestratorForwarder := optional.NewOptionPtr[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	eventPlatformForwarder := optional.NewOptionPtr[eventplatform.Forwarder](eventplatformimpl.NewNoopEventPlatformForwarder(sender.deps.Hostname))
	senderManager = aggregator.InitAndStartAgentDemultiplexer(
		log,
		forwarder,
		orchestratorForwarder,
		opts,
		eventPlatformForwarder,
		sender.deps.Compressor,
		sender.deps.Tagger,
		hostnameDetected)

	sender.senderManager.Set(senderManager)
	return senderManager, nil
}
