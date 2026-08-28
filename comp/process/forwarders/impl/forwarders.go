// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package forwardersimpl implements a component to provide forwarders used by the process agent.
package forwardersimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	defaultforwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	forwarders "github.com/DataDog/datadog-agent/comp/process/forwarders/def"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

type dependencies struct {
	compdef.In

	Config                config.Component
	Logger                log.Component
	ConnectionsForwarders connectionsforwarder.Component
	Lc                    compdef.Lifecycle
	Secrets               secrets.Component
	DelegatedAuth         delegatedauth.Component
}

type forwardersComp struct {
	processForwarder     defaultforwarder.Component
	rtProcessForwarder   defaultforwarder.Component
	connectionsForwarder connectionsforwarder.Component
}

// NewComponent creates a new forwarders component.
func NewComponent(deps dependencies) (forwarders.Component, error) {
	config := deps.Config
	queueBytes := config.GetInt("process_config.process_queue_bytes")
	if queueBytes <= 0 {
		deps.Logger.Warnf("Invalid queue bytes size: %d. Using default value: %d", queueBytes, pkgconfigsetup.DefaultProcessQueueBytes)
		queueBytes = pkgconfigsetup.DefaultProcessQueueBytes
	}

	processAPIEndpoints, err := endpoint.GetAPIEndpoints(config)
	if err != nil {
		return nil, err
	}

	processForwarderOpts, err := createParams(deps.Config, deps.Logger, queueBytes, processAPIEndpoints, deps.DelegatedAuth)
	if err != nil {
		return nil, err
	}

	return &forwardersComp{
		processForwarder:     createForwarder(deps, processForwarderOpts),
		rtProcessForwarder:   createForwarder(deps, processForwarderOpts),
		connectionsForwarder: deps.ConnectionsForwarders,
	}, nil
}

func createForwarder(deps dependencies, options *defaultforwarderimpl.Options) defaultforwarder.Component {
	options.Secrets = deps.Secrets
	return defaultforwarderimpl.NewForwarder(deps.Config, deps.Logger, deps.Lc, false, options).Comp
}

func createParams(config config.Component, log log.Component, queueBytes int, endpoints []apicfg.Endpoint, delegatedAuth delegatedauth.Component) (*defaultforwarderimpl.Options, error) {
	eds := configutils.EndpointDescriptorSetFromKeysPerDomain(apicfg.KeysPerDomains(endpoints))
	resolvers, err := resolver.NewSingleDomainResolvers2(eds)
	if err != nil {
		return nil, err
	}
	if delegatedAuth != nil {
		for _, ed := range eds {
			r, ok := resolvers[ed.BaseURL]
			if !ok {
				continue
			}
			var providers []resolver.CredentialProvider
			for _, keys := range ed.APIKeySet {
				providers = append(providers, delegatedAuth.ProvidersFor(keys.ConfigSettingPath, ed.BaseURL)...)
			}
			if len(providers) > 0 {
				r.SetCredentialProviders(providers)
			}
		}
	}
	forwarderOpts := defaultforwarderimpl.NewOptionsWithResolvers(config, log, resolvers)
	forwarderOpts.DisableAPIKeyChecking = true
	forwarderOpts.RetryQueuePayloadsTotalMaxSize = queueBytes // Allow more in-flight requests than the default
	return forwarderOpts, nil
}

func (f *forwardersComp) GetProcessForwarder() defaultforwarder.Component {
	return f.processForwarder
}

func (f *forwardersComp) GetRTProcessForwarder() defaultforwarder.Component {
	return f.rtProcessForwarder
}

func (f *forwardersComp) GetConnectionsForwarder() connectionsforwarder.Component {
	return f.connectionsForwarder
}
