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
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Workload represents a workload along with its tags
type Workload struct {
	*cgroupModel.CacheEntry
	Tags     []string
	Selector cgroupModel.WorkloadSelector
	retries  int
}

// LinuxResolver represents a default resolver based directly on the underlying tagger
type LinuxResolver struct {
	*DefaultResolver
	*utils.Notifier[Event, *Workload]
	workloadsWithoutTags chan *Workload
	cgroupResolver       *cgroup.Resolver
	workloads            map[containerutils.CGroupID]*Workload
}

// Start the resolver
func (t *LinuxResolver) Start(ctx context.Context) error {
	if err := t.DefaultResolver.Start(ctx); err != nil {
		return err
	}

	if err := t.cgroupResolver.RegisterListener(cgroup.CGroupCreated, func(cgce *cgroupModel.CacheEntry) {
		workload := &Workload{CacheEntry: cgce, retries: 3}
		t.workloads[cgce.CGroupID] = workload
		t.checkTags(workload)
	}); err != nil {
		return err
	}

	if err := t.cgroupResolver.RegisterListener(cgroup.CGroupDeleted, func(cgce *cgroupModel.CacheEntry) {
		if workload, ok := t.workloads[cgce.CGroupID]; ok {
			t.NotifyListeners(WorkloadSelectorDeleted, workload)
			delete(t.workloads, cgce.CGroupID)
		}
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

func needsTagsResolution(workload *Workload) bool {
	return len(workload.ContainerID) != 0 && !workload.Selector.IsReady()
}

// checkTags checks if the tags of a workload were properly set
func (t *LinuxResolver) checkTags(pendingWorkload *Workload) {
	workload := pendingWorkload
	// check if the workload tags were found or if it was deleted
	if !workload.Deleted.Load() && needsTagsResolution(workload) {
		// this is an alive cgroup, try to resolve its tags now
		err := t.fetchTags(workload)
		if err != nil || needsTagsResolution(workload) {
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
func (t *LinuxResolver) fetchTags(workload *Workload) error {
	newTags, err := t.ResolveWithErr(workload.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", workload.ContainerID, err)
	}

	workload.Selector.Image = utils.GetTagValue("image_name", newTags)
	workload.Selector.Tag = utils.GetTagValue("image_tag", newTags)
	if len(workload.Selector.Image) != 0 && len(workload.Selector.Tag) == 0 {
		workload.Selector.Tag = "latest"
	}

	return nil
}

// NewResolver returns a new tags resolver
func NewResolver(tagger Tagger, cgroupsResolver *cgroup.Resolver) *LinuxResolver {
	resolver := &LinuxResolver{
		Notifier:             utils.NewNotifier[Event, *Workload](),
		DefaultResolver:      NewDefaultResolver(tagger),
		workloadsWithoutTags: make(chan *Workload, 100),
		cgroupResolver:       cgroupsResolver,
		workloads:            make(map[containerutils.CGroupID]*Workload),
	}
	return resolver
}
