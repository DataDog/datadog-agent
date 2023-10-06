// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resolvers holds resolvers related files
package resolvers

import (
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/hash"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	ProcessResolver *process.Resolver
	TagsResolver    tags.Resolver
	HashResolver    *hash.Resolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber) (*Resolvers, error) {
	processResolver, err := process.NewResolver(config, statsdClient, scrubber, process.NewResolverOpts())
	if err != nil {
		return nil, err
	}

	tagsResolver := tags.NewResolver(config.Probe)

	hashResolver, err := hash.NewResolver(config.RuntimeSecurity, statsdClient)
	if err != nil {
		return nil, err
	}

	resolvers := &Resolvers{
		ProcessResolver: processResolver,
		TagsResolver:    tagsResolver,
		HashResolver:    hashResolver,
	}
	return resolvers, nil
}

// Snapshot collects data on the current state of the system
func (r *Resolvers) Snapshot() error {
	r.ProcessResolver.Snapshot()
	return nil
}
