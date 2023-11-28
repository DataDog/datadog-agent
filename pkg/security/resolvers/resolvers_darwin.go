// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resolvers holds resolvers related files
package resolvers

import (
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
)

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	ProcessResolver *process.Resolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, _ statsd.ClientInterface, scrubber *procutil.DataScrubber) (*Resolvers, error) {
	processResolver, err := process.NewResolver(config, scrubber)
	if err != nil {
		return nil, err
	}

	resolvers := &Resolvers{
		ProcessResolver: processResolver,
	}
	return resolvers, nil
}

// Snapshot collects data on the current state of the system
func (r *Resolvers) Snapshot() error {
	r.ProcessResolver.Snapshot()
	return nil
}
