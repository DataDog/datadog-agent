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

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

type pendingWorkload struct {
	*cgroupModel.CacheEntry
	retries int
}

// LinuxResolver represents a default resolver based directly on the underlying tagger
type LinuxResolver struct {
	*DefaultResolver
	*utils.Notifier[Event, *cgroupModel.CacheEntry]
	workloadsWithoutTags chan *pendingWorkload
	cgroupResolver       *cgroup.Resolver
}

// Start the resolver
func (t *LinuxResolver) Start(ctx context.Context) error {
	if err := t.DefaultResolver.Start(ctx); err != nil {
		return err
	}

	if err := t.cgroupResolver.RegisterListener(cgroup.CGroupCreated, func(cgce *cgroupModel.CacheEntry) {
		workload := &pendingWorkload{CacheEntry: cgce, retries: 3}
		t.checkTags(workload)
	}); err != nil {
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

			WORKLOAD:
				// we want to process approximately the number of workloads in the queue
				for workloadCount := len(t.workloadsWithoutTags); workloadCount > 0; workloadCount-- {
					select {
					case workload := <-t.workloadsWithoutTags:
						t.checkTags(workload)
					default:
						break WORKLOAD
					}
				}
			}
		}
	}()

	return nil
}

func needsTagsResolution(cgce *cgroupModel.CacheEntry) bool {
	return len(cgce.ContainerID) != 0 && !cgce.WorkloadSelector.IsReady()
}

// checkTags checks if the tags of a workload were properly set
func (t *LinuxResolver) checkTags(pendingWorkload *pendingWorkload) {
	workload := pendingWorkload.CacheEntry
	// check if the workload tags were found or if it was deleted
	if !workload.Deleted.Load() && needsTagsResolution(workload) {
		// this is an alive cgroup, try to resolve its tags now
		if err := t.fetchTags(workload); err != nil || needsTagsResolution(workload) {
			if pendingWorkload.retries--; pendingWorkload.retries >= 0 {
				// push to the workloadsWithoutTags chan so that its tags can be resolved later
				select {
				case t.workloadsWithoutTags <- pendingWorkload:
				default:
					seclog.Warnf("Failed to requeue workload %s for tags retrieval", workload.ContainerID)
				}
			} else {
				seclog.Debugf("Failed to resolve tags for workload %s", workload.ContainerID)
			}
			return
		}

		t.NotifyListeners(WorkloadSelectorResolved, workload)
	}
}

// fetchTags fetches tags for the provided workload
func (t *LinuxResolver) fetchTags(container *cgroupModel.CacheEntry) error {
	newTags, err := t.ResolveWithErr(container.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", container.ContainerID, err)
	}
	container.SetTags(newTags)
	return nil
}

// NewResolver returns a new tags resolver
func NewResolver(tagger Tagger, cgroupsResolver *cgroup.Resolver) *LinuxResolver {
	resolver := &LinuxResolver{
		Notifier:             utils.NewNotifier[Event, *cgroupModel.CacheEntry](),
		DefaultResolver:      NewDefaultResolver(tagger),
		workloadsWithoutTags: make(chan *pendingWorkload, 100),
		cgroupResolver:       cgroupsResolver,
	}
	return resolver
}
