// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package connectionsforwarderimpl implements the connectionsforwarder component interface
package connectionsforwarderimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/runner/endpoint"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
)

// Requires defines the dependencies for the connectionsforwarder component
type Requires struct {
	Lifecycle compdef.Lifecycle

	Config  config.Component
	Logger  log.Component
	Secrets secrets.Component
}

// Provides defines the output of the connectionsforwarder component
type Provides struct {
	Comp connectionsforwarder.Component
}

// NewComponent creates a new connectionsforwarder component
func NewComponent(reqs Requires) (Provides, error) {
	queueBytes := reqs.Config.GetInt("process_config.process_queue_bytes")
	if queueBytes <= 0 {
		reqs.Logger.Warnf("Invalid queue bytes size: %d. Using default value: %d", queueBytes, pkgconfigsetup.DefaultProcessQueueBytes)
		queueBytes = pkgconfigsetup.DefaultProcessQueueBytes
	}

	processAPIEndpoints, err := endpoint.GetAPIEndpoints(reqs.Config)
	if err != nil {
		return Provides{}, err
	}

	processForwarderOpts, err := createParams(reqs.Config, reqs.Logger, queueBytes, processAPIEndpoints)
	if err != nil {
		return Provides{}, err
	}
	processForwarderOpts.Secrets = reqs.Secrets

	forwarder := defaultforwarder.NewDefaultForwarder(reqs.Config, reqs.Logger, processForwarderOpts)
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error { return forwarder.Start() },
		OnStop:  func(context.Context) error { forwarder.Stop(); return nil },
	})

	return Provides{
		Comp: forwarder,
	}, nil
}

func createParams(config config.Component, log log.Component, queueBytes int, endpoints []apicfg.Endpoint) (*defaultforwarder.Options, error) {
	r, err := resolver.NewSingleDomainResolvers(apicfg.KeysPerDomains(endpoints))
	if err != nil {
		return nil, err
	}
	forwarderOpts := defaultforwarder.NewOptionsWithResolvers(config, log, r)
	forwarderOpts.DisableAPIKeyChecking = true
	forwarderOpts.RetryQueuePayloadsTotalMaxSize = queueBytes // Allow more in-flight requests than the default
	return forwarderOpts, nil
}
