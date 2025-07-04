// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package forwardersimpl implements a component to provide forwarders used by the process agent.
package forwardersimpl

import (
	"go.uber.org/fx"

	config "github.com/DataDog/datadog-agent/comp/core/config/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl/resolver"
	"github.com/DataDog/datadog-agent/comp/process/forwarders"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newForwarders),
	)
}

type dependencies struct {
	fx.In

	Config config.Component
	Logger log.Component
	Lc     fx.Lifecycle
}

type forwardersComp struct {
	eventForwarder       defaultforwarder.Component
	processForwarder     defaultforwarder.Component
	rtProcessForwarder   defaultforwarder.Component
	connectionsForwarder defaultforwarder.Component
}

func newForwarders(deps dependencies) (forwarders.Component, error) {
	config := deps.Config
	queueBytes := config.GetInt("process_config.process_queue_bytes")
	if queueBytes <= 0 {
		deps.Logger.Warnf("Invalid queue bytes size: %d. Using default value: %d", queueBytes, pkgconfigsetup.DefaultProcessQueueBytes)
		queueBytes = pkgconfigsetup.DefaultProcessQueueBytes
	}

	eventsAPIEndpoints, err := endpoint.GetEventsAPIEndpoints(config)
	if err != nil {
		return nil, err
	}

	eventForwarderOpts, err := createParams(deps.Config, deps.Logger, queueBytes, eventsAPIEndpoints)
	if err != nil {
		return nil, err
	}

	processAPIEndpoints, err := endpoint.GetAPIEndpoints(config)
	if err != nil {
		return nil, err
	}

	processForwarderOpts, err := createParams(deps.Config, deps.Logger, queueBytes, processAPIEndpoints)
	if err != nil {
		return nil, err
	}

	return &forwardersComp{
		eventForwarder:       createForwarder(deps, eventForwarderOpts),
		processForwarder:     createForwarder(deps, processForwarderOpts),
		rtProcessForwarder:   createForwarder(deps, processForwarderOpts),
		connectionsForwarder: createForwarder(deps, processForwarderOpts),
	}, nil
}

func createForwarder(deps dependencies, options *defaultforwarderimpl.Options) defaultforwarder.Component {
	// For now, use a noop forwarder since the new architecture doesn't support standalone forwarder creation
	return defaultforwarderimpl.NoopForwarder{}
}

func createParams(config config.Component, log log.Component, queueBytes int, endpoints []apicfg.Endpoint) (*defaultforwarderimpl.Options, error) {
	resolver, err := resolver.NewSingleDomainResolvers(apicfg.KeysPerDomains(endpoints))
	if err != nil {
		return nil, err
	}
	forwarderOpts := defaultforwarderimpl.NewOptionsWithResolvers(config, log, resolver)
	forwarderOpts.DisableAPIKeyChecking = true
	forwarderOpts.RetryQueuePayloadsTotalMaxSize = queueBytes // Allow more in-flight requests than the default
	return forwarderOpts, nil
}

func (f *forwardersComp) GetEventForwarder() defaultforwarder.Component {
	return f.eventForwarder
}

func (f *forwardersComp) GetProcessForwarder() defaultforwarder.Component {
	return f.processForwarder
}

func (f *forwardersComp) GetRTProcessForwarder() defaultforwarder.Component {
	return f.rtProcessForwarder
}

func (f *forwardersComp) GetConnectionsForwarder() defaultforwarder.Component {
	return f.connectionsForwarder
}
