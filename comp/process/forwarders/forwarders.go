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
	eventForwarder defaultforwarder.Component
}

func newForwarders(deps dependencies) (Component, error) {
	queueBytes := deps.Config.GetInt("process_config.process_queue_bytes")
	if queueBytes <= 0 {
		deps.Logger.Warnf("Invalid queue bytes size: %d. Using default value: %d", queueBytes, ddconfig.DefaultProcessQueueBytes)
		queueBytes = ddconfig.DefaultProcessQueueBytes
	}

	eventForwarderOpts, err := createParams(deps.Config, queueBytes)
	if err != nil {
		return nil, err
	}

	return &forwarders{
		eventForwarder: defaultforwarder.NewForwarder(deps.Config, eventForwarderOpts),
	}, nil

}

func createParams(config config.Component, queueBytes int) (defaultforwarder.Params, error) {
	apiEndpoints, err := endpoint.GetEventsAPIEndpoints(config)
	if err != nil {
		return defaultforwarder.Params{}, err
	}
	forwarderOpts := defaultforwarder.NewOptionsWithResolvers(config, resolver.NewSingleDomainResolvers(apicfg.KeysPerDomains(apiEndpoints)))
	forwarderOpts.DisableAPIKeyChecking = true
	forwarderOpts.RetryQueuePayloadsTotalMaxSize = queueBytes // Allow more in-flight requests than the default
	return defaultforwarder.Params{Options: forwarderOpts}, nil
}

func (f *forwarders) GetEventForwarder() defaultforwarder.Component {
	return f.eventForwarder
}

func newMockForwarders(deps dependencies) (Component, error) {
	return newForwarders(deps)
}
