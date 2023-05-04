// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

type Params struct {
	UseNoopForwarder bool
	// TODO: (components) When the code of the forwarder will be
	// in /comp/forwarder move the content of forwarder.Options inside this struct.
	Options *Options
}

func NewParams(config config.Component, log log.Component) Params {
	return Params{Options: NewOptions(config, getMultipleEndpoints(config, log))}
}

func NewParamsWithResolvers(config config.Component, log log.Component) Params {
	keysPerDomain := getMultipleEndpoints(config, log)
	return Params{Options: NewOptionsWithResolvers(config, resolver.NewSingleDomainResolvers(keysPerDomain))}
}

func getMultipleEndpoints(_ config.Component, log log.Component) map[string][]string {
	// Inject the config to make sure we can call GetMultipleEndpoints.
	keysPerDomain, err := utils.GetMultipleEndpoints(pkgconfig.Datadog)
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	return keysPerDomain
}
