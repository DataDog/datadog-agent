// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hash

import (
	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// ResolverOpts defines hash resolver options
type ResolverOpts struct{}

// Resolver represents a cache for mountpoints and the corresponding file systems
type Resolver struct{}

// NewResolver returns a new instance of the hash resolver
func NewResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface) (*Resolver, error) {
	return &Resolver{}, nil
}

// ComputeHashesFromEvent calls ComputeHashes using the provided event
func (resolver *Resolver) ComputeHashesFromEvent(event *model.Event, file *model.FileEvent) []string {
	return nil
}

// ComputeHashes computes the hashes of the provided file event.
// Disclaimer: This resolver considers that the FileEvent has already been resolved
func (resolver *Resolver) ComputeHashes(eventType model.EventType, process *model.Process, file *model.FileEvent) []string {
	return nil
}
