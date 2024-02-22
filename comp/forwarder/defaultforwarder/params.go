// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

// Params contains the parameters to create a forwarder.
type Params struct {
	UseNoopForwarder bool
	// TODO: (components) When the code of the forwarder will be
	// in /comp/forwarder move the content of forwarder.Options inside this struct.
	Options *Options
}

// NewParams initializes a new Params struct
func NewParams(config config.Component, log log.Component) Params {
	return Params{Options: NewOptions(config, log, getMultipleEndpoints(config, log))}
}

// NewParamsWithResolvers initializes a new Params struct with resolvers
func NewParamsWithResolvers(config config.Component, log log.Component) Params {
	keysPerDomain := getMultipleEndpoints(config, log)
	return Params{Options: NewOptionsWithResolvers(config, log, resolver.NewSingleDomainResolvers(keysPerDomain))}
}

func getMultipleEndpoints(config config.Component, log log.Component) map[string][]string {
	// Inject the config to make sure we can call GetMultipleEndpoints.
	keysPerDomain, err := utils.GetMultipleEndpoints(config)
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	return keysPerDomain
}
