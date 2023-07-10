// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarders

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Config config.Component
	Logger log.Component
}

type forwarders struct {
	eventForwarder       defaultforwarder.Component
	processForwarder     defaultforwarder.Component
	rtProcessForwarder   defaultforwarder.Component
	connectionsForwarder defaultforwarder.Component
}

func newForwarders(deps dependencies) (Component, error) {
	config := deps.Config
	queueBytes := config.GetInt("process_config.process_queue_bytes")
	if queueBytes <= 0 {
		deps.Logger.Warnf("Invalid queue bytes size: %d. Using default value: %d", queueBytes, ddconfig.DefaultProcessQueueBytes)
		queueBytes = ddconfig.DefaultProcessQueueBytes
	}

	eventsAPIEndpoints, err := endpoint.GetEventsAPIEndpoints(config)
	if err != nil {
		return nil, err
	}

	eventForwarderOpts := createParams(deps.Config, deps.Logger, queueBytes, eventsAPIEndpoints)

	processAPIEndpoints, err := endpoint.GetAPIEndpoints(config)
	if err != nil {
		return nil, err
	}

	processForwarderOpts := createParams(deps.Config, deps.Logger, queueBytes, processAPIEndpoints)

	return &forwarders{
		eventForwarder:       defaultforwarder.NewForwarder(deps.Config, deps.Logger, eventForwarderOpts),
		processForwarder:     defaultforwarder.NewForwarder(deps.Config, deps.Logger, processForwarderOpts),
		rtProcessForwarder:   defaultforwarder.NewForwarder(deps.Config, deps.Logger, processForwarderOpts),
		connectionsForwarder: defaultforwarder.NewForwarder(deps.Config, deps.Logger, processForwarderOpts),
	}, nil

}

func createParams(config config.Component, log log.Component, queueBytes int, endpoints []apicfg.Endpoint) defaultforwarder.Params {
	forwarderOpts := defaultforwarder.NewOptionsWithResolvers(config, log, resolver.NewSingleDomainResolvers(apicfg.KeysPerDomains(endpoints)))
	forwarderOpts.DisableAPIKeyChecking = true
	forwarderOpts.RetryQueuePayloadsTotalMaxSize = queueBytes // Allow more in-flight requests than the default
	return defaultforwarder.Params{Options: forwarderOpts}
}

func (f *forwarders) GetEventForwarder() defaultforwarder.Component {
	return f.eventForwarder
}

func (f *forwarders) GetProcessForwarder() defaultforwarder.Component {
	return f.processForwarder
}

func (f *forwarders) GetRTProcessForwarder() defaultforwarder.Component {
	return f.rtProcessForwarder
}

func (f *forwarders) GetConnectionsForwarder() defaultforwarder.Component {
	return f.connectionsForwarder
}

func newMockForwarders(deps dependencies) (Component, error) {
	return newForwarders(deps)
}
