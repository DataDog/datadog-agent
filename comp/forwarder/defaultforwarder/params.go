// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Params contains the parameters to create a forwarder.
type Params struct {
	UseNoopForwarder              bool
	withResolver                  bool
	disableAPIKeyCheckingOverride optional.Option[bool]
	features                      []Features
}

// NewParams initializes a new Params struct
func NewParams(config config.Component, log log.Component) Params {
	return Params{withResolver: false}
}

// NewParamsWithResolvers initializes a new Params struct with resolvers
func NewParamsWithResolvers(config config.Component, log log.Component) Params {
	return Params{withResolver: true}
}

func (p *Params) CreationOptions(config config.Component, log log.Component) *Options {
	var options *Options
	if !p.withResolver {
		options = NewOptions(config, log, getMultipleEndpoints(config, log))
	} else {
		keysPerDomain := getMultipleEndpoints(config, log)
		options = NewOptionsWithResolvers(config, log, resolver.NewSingleDomainResolvers(keysPerDomain))
	}
	if disableAPIKeyChecking, ok := p.disableAPIKeyCheckingOverride.Get(); ok {
		options.DisableAPIKeyChecking = disableAPIKeyChecking
	}
	for _, feature := range p.features {
		options.EnabledFeatures = SetFeature(options.EnabledFeatures, feature)
	}
	return options
}

func (p *Params) DisableAPIKeyChecking() {
	p.disableAPIKeyCheckingOverride.Set(true)
}

func (p *Params) SetFeature(feature Features) {
	p.features = append(p.features, feature)
}

func getMultipleEndpoints(config config.Component, log log.Component) map[string][]string {
	// Inject the config to make sure we can call GetMultipleEndpoints.
	keysPerDomain, err := utils.GetMultipleEndpoints(config)
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	return keysPerDomain
}
