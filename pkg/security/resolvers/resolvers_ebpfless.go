// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpfless

// Package resolvers holds resolvers related files
package resolvers

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/container"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/hash"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/mount"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/path"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/usergroup"
)

// Opts defines common options
type Opts struct {
	TagsResolver tags.Resolver
}

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
	manager           *manager.Manager
	MountResolver     *mount.Resolver
	ContainerResolver *container.Resolver
	TimeResolver      *time.Resolver
	UserGroupResolver *usergroup.Resolver
	TagsResolver      tags.Resolver
	DentryResolver    *dentry.Resolver
	ProcessResolver   *process.Resolver
	CGroupResolver    *cgroup.Resolver
	PathResolver      path.ResolverInterface
	SBOMResolver      *sbom.Resolver
	HashResolver      *hash.Resolver
	TCResolver        *tc.Resolver
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber, opts Opts) (*Resolvers, error) {
	var tagsResolver tags.Resolver
	if opts.TagsResolver != nil {
		tagsResolver = opts.TagsResolver

		fmt.Printf("EEEEEEEEEEEEEEEE\n")
	} else {
		tagsResolver = tags.NewResolver(config.Probe)
		fmt.Printf("FFFFFFFFFFFFFFFF: %v\n", tagsResolver)
	}

	processOpts := process.NewResolverOpts()
	processOpts.WithEnvsValue(config.Probe.EnvsWithValue)

	processResolver, err := process.NewResolver(config.Probe, statsdClient, scrubber, processOpts)
	if err != nil {
		return nil, err
	}

	resolvers := &Resolvers{
		TagsResolver:    tagsResolver,
		ProcessResolver: processResolver,
	}

	return resolvers, nil
}

// Start the resolvers
func (r *Resolvers) Start(ctx context.Context) error {
	if err := r.ProcessResolver.Start(ctx); err != nil {
		return err
	}

	if err := r.TagsResolver.Start(ctx); err != nil {
		return err
	}

	return nil
}

// Snapshot collects data on the current state of the system to populate user space and kernel space caches.
func (r *Resolvers) Snapshot() error {
	return nil
}

// Close cleans up any underlying resolver that requires a cleanup
func (r *Resolvers) Close() error {
	return nil
}
