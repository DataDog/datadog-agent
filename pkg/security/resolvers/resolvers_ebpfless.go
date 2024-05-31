// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package resolvers holds resolvers related files
package resolvers

import (
	"context"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/container"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/hash"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// EBPFLessResolvers holds the list of the event attribute resolvers
type EBPFLessResolvers struct {
	ContainerResolver *container.Resolver
	TagsResolver      tags.Resolver
	ProcessResolver   *process.EBPFLessResolver
	HashResolver      *hash.Resolver
	SBOMResolver      *sbom.Resolver
}

// NewEBPFLessResolvers creates a new instance of EBPFLessResolvers
func NewEBPFLessResolvers(config *config.Config, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber, opts Opts, wmeta optional.Option[workloadmeta.Component]) (*EBPFLessResolvers, error) {
	var tagsResolver tags.Resolver
	if opts.TagsResolver != nil {
		tagsResolver = opts.TagsResolver
	} else {
		tagsResolver = tags.NewResolver(config.Probe)
	}

	processOpts := process.NewResolverOpts()
	processOpts.WithEnvsValue(config.Probe.EnvsWithValue)

	processResolver, err := process.NewEBPFLessResolver(config.Probe, statsdClient, scrubber, processOpts)
	if err != nil {
		return nil, err
	}

	var sbomResolver *sbom.Resolver
	if config.RuntimeSecurity.SBOMResolverEnabled {
		sbomResolver, err = sbom.NewSBOMResolver(config.RuntimeSecurity, statsdClient, wmeta)
		if err != nil {
			return nil, err
		}
	}

	cgroupsResolver, err := cgroup.NewResolver(tagsResolver, sbomResolver)
	if err != nil {
		return nil, err
	}
	hashResolver, err := hash.NewResolver(config.RuntimeSecurity, statsdClient, cgroupsResolver)
	if err != nil {
		return nil, err
	}

	resolvers := &EBPFLessResolvers{
		TagsResolver:    tagsResolver,
		ProcessResolver: processResolver,
		HashResolver:    hashResolver,
		SBOMResolver:    sbomResolver,
	}

	return resolvers, nil
}

// Start the resolvers
func (r *EBPFLessResolvers) Start(ctx context.Context) error {
	if err := r.ProcessResolver.Start(ctx); err != nil {
		return err
	}

	if err := r.TagsResolver.Start(ctx); err != nil {
		return err
	}

	return nil
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *EBPFLessResolvers) Snapshot() error {
	return nil
}

// Close cleans up any underlying resolver that requires a cleanup
func (r *EBPFLessResolvers) Close() error {
	return nil
}
