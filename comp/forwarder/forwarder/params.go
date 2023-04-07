// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package forwarder

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
)

type Params struct {
	UseNoopForwarder bool
	// TODO: (components) When the code of the forwarder will be
	// in /comp/forwarder move the content of forwarder.Options inside this struct.
	Options *forwarder.Options
}

func NewParams(config config.Component, log log.Component) Params {
	return Params{Options: forwarder.NewOptions(getMultipleEndpoints(config, log))}
}

func NewParamsWithResolvers(config config.Component, log log.Component) Params {
	keysPerDomain := getMultipleEndpoints(config, log)
	return Params{Options: forwarder.NewOptionsWithResolvers(resolver.NewSingleDomainResolvers(keysPerDomain))}
}

func getMultipleEndpoints(_ config.Component, log log.Component) map[string][]string {
	// Inject the config to make sure we can call GetMultipleEndpoints.
	keysPerDomain, err := pkgconfig.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	return keysPerDomain
}
