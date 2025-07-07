// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags holds tags related files
package tags

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const systemdSystemDir = "/usr/lib/systemd/system"

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
	versionResolver      func(servicePath string) string
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
	// Container or cgroup workloads need tags resolution if they don't have a ready selector
	return (len(workload.ContainerID) != 0 || len(workload.CGroupID) != 0) && !workload.Selector.IsReady()
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
					workloadID := t.getWorkloadID(workload)
					seclog.Warnf("Failed to requeue workload %v for tags retrieval", workloadID)
				}
			} else {
				workloadID := t.getWorkloadID(workload)
				seclog.Debugf("Failed to resolve tags for workload %v", workloadID)
			}
			return
		}

		t.NotifyListeners(WorkloadSelectorResolved, workload)
	}
}

// getWorkloadID returns the workload ID for a workload
func (t *LinuxResolver) getWorkloadID(workload *Workload) interface{} {
	if len(workload.ContainerID) != 0 {
		return workload.ContainerID
	}
	return workload.CGroupID
}

// fetchTags fetches tags for the provided workload
func (t *LinuxResolver) fetchTags(workload *Workload) error {
	workloadID := t.getWorkloadID(workload)
	newTags, err := t.ResolveWithErr(workloadID)
	if err != nil {
		return fmt.Errorf("failed to resolve %v: %w", workloadID, err)
	}

	workload.Tags = newTags

	// For container workloads, try to extract image information
	if len(workload.ContainerID) != 0 {
		workload.Selector.Image = utils.GetTagValue("image_name", newTags)
		workload.Selector.Tag = utils.GetTagValue("image_tag", newTags)
		if len(workload.Selector.Image) != 0 && len(workload.Selector.Tag) == 0 {
			workload.Selector.Tag = "latest"
		}
	} else {
		// For cgroup workloads, set service information as the selector
		serviceName := utils.GetTagValue("service", newTags)
		if len(serviceName) != 0 {
			workload.Selector.Image = serviceName
			workload.Selector.Tag = utils.GetTagValue("version", newTags)
		}
	}

	return nil
}

// NewResolver returns a new tags resolver
func NewResolver(tagger Tagger, cgroupsResolver *cgroup.Resolver, versionResolver func(servicePath string) string) *LinuxResolver {
	resolver := &LinuxResolver{
		Notifier:             utils.NewNotifier[Event, *Workload](),
		DefaultResolver:      NewDefaultResolver(tagger),
		workloadsWithoutTags: make(chan *Workload, 100),
		cgroupResolver:       cgroupsResolver,
		versionResolver:      versionResolver,
		workloads:            make(map[containerutils.CGroupID]*Workload),
	}
	return resolver
}

// ResolveWithErr overrides the default implementation to use Linux-specific workload resolution
func (t *LinuxResolver) ResolveWithErr(id interface{}) ([]string, error) {
	return t.resolveWorkloadTags(id)
}

// resolveWorkloadTags overrides the default implementation to handle CGroup resolution on Linux
func (t *LinuxResolver) resolveWorkloadTags(id interface{}) ([]string, error) {
	if id == nil {
		return nil, fmt.Errorf("nil workload id")
	}

	switch v := id.(type) {
	case containerutils.ContainerID:
		if len(v) == 0 {
			return nil, fmt.Errorf("empty container id")
		}
		// Resolve as a container ID
		return GetTagsOfContainer(t.tagger, v)
	case containerutils.CGroupID:
		if len(v) == 0 {
			return nil, fmt.Errorf("empty cgroup id")
		}
		// Generate systemd service tags for cgroup workloads
		tags := t.getCGroupTags(v)
		return tags, nil
	default:
		return nil, fmt.Errorf("unknown workload id type: %T", id)
	}
}

// getCGroupTags generates tags for cgroup workloads (systemd services) with version resolution
func (t *LinuxResolver) getCGroupTags(cgroupID containerutils.CGroupID) []string {
	if len(cgroupID) == 0 {
		return nil
	}

	systemdService := filepath.Base(string(cgroupID))
	serviceVersion := ""
	servicePath := filepath.Join(systemdSystemDir, systemdService)

	// Try to resolve version using version resolver
	if t.versionResolver != nil {
		serviceVersion = t.versionResolver(servicePath)
	}

	tags := []string{
		"service:" + systemdService,
	}
	if len(serviceVersion) != 0 {
		tags = append(tags, "version:"+serviceVersion)
	}

	return tags
}
