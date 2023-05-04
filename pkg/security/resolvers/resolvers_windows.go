// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package resolvers

import (
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	ProcessResolver *process.ProcessResolver
	TagsResolver    *tags.Resolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, statsdClient statsd.ClientInterface) (*Resolvers, error) {

	processResolver, err := process.NewResolver(config, statsdClient, process.NewResolverOpts())
	if err != nil {
		return nil, err
	}

	tagsResolver := tags.NewResolver(config.Probe)

	resolvers := &Resolvers{
		ProcessResolver: processResolver,
		TagsResolver:    tagsResolver,
	}
	return resolvers, nil
}
