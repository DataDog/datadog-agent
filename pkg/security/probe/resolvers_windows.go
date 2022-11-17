// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package probe

import "github.com/DataDog/datadog-agent/pkg/security/config"

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	probe *Probe
	//ProcessResolver   *ProcessResolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, probe *Probe) (*Resolvers, error) {

	resolvers := &Resolvers{
		probe: probe,
	}

	//processResolver, err := NewProcessResolver(probe, resolvers, NewProcessResolverOpts(probe.config.EnvsWithValue))
	//if err != nil {
	//	return nil, err
	//}

	//resolvers.ProcessResolver = processResolver

	return resolvers, nil
}
