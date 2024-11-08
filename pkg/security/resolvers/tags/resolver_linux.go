// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags holds tags related files
package tags

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/remote"
	taggerTelemetry "github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Listener is used to propagate tags events
type Listener func(workload *cgroupModel.CacheEntry)

// LinuxResolver represents a default resolver based directly on the underlying tagger
type LinuxResolver struct {
	*DefaultResolver
	workloadsWithoutTags chan *cgroupModel.CacheEntry
	cgroupResolver       *cgroup.Resolver
}

// Start the resolver
func (t *LinuxResolver) Start(ctx context.Context) error {
	if err := t.DefaultResolver.Start(ctx); err != nil {
		return err
	}

	if err := t.cgroupResolver.RegisterListener(cgroup.CGroupCreated, t.checkTags); err != nil {
		return err
	}

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		delayerTick := time.NewTicker(10 * time.Second)
		defer delayerTick.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-delayerTick.C:
				select {
				case workload := <-t.workloadsWithoutTags:
					t.checkTags(workload)
				default:
				}

			}
		}
	}()

	return nil
}

// checkTags checks if the tags of a workload were properly set
func (t *LinuxResolver) checkTags(workload *cgroupModel.CacheEntry) {
	// check if the workload tags were found
	if workload.NeedsTagsResolution() {
		// this is a container, try to resolve its tags now
		if err := t.fetchTags(workload); err != nil || workload.NeedsTagsResolution() {
			// push to the workloadsWithoutTags chan so that its tags can be resolved later
			select {
			case t.workloadsWithoutTags <- workload:
			default:
			}
			return
		}
	}

	// notify listeners
	t.listenersLock.Lock()
	defer t.listenersLock.Unlock()
	for _, l := range t.listeners[WorkloadSelectorResolved] {
		l(workload)
	}
}

// fetchTags fetches tags for the provided workload
func (t *LinuxResolver) fetchTags(container *cgroupModel.CacheEntry) error {
	newTags, err := t.ResolveWithErr(string(container.ContainerID))
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", container.ContainerID, err)
	}
	container.SetTags(newTags)
	return nil
}

// NewResolver returns a new tags resolver
func NewResolver(config *config.Config, telemetry telemetry.Component, cgroupsResolver *cgroup.Resolver) Resolver {
	defaultResolver := NewDefaultResolver(config, telemetry)
	workloadsWithoutTags := make(chan *cgroupModel.CacheEntry, 100)
	resolver := &LinuxResolver{
		DefaultResolver:      defaultResolver,
		workloadsWithoutTags: workloadsWithoutTags,
		cgroupResolver:       cgroupsResolver,
	}
	if config.RemoteTaggerEnabled {
		options, err := remote.NodeAgentOptionsForSecurityResolvers(pkgconfigsetup.Datadog())
		if err != nil {
			log.Errorf("unable to configure the remote tagger: %s", err)
		} else {
			resolver.tagger = remote.NewTagger(options, pkgconfigsetup.Datadog(), taggerTelemetry.NewStore(telemetry), types.NewMatchAllFilter())
		}
	}
	return resolver
}
