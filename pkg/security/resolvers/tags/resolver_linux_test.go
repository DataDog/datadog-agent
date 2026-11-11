// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tags

import (
	"math"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// lateTagger returns the tags of a container from its readyAt-th lookup on, as
// the tagger of system-probe does while the core agent starts.
type lateTagger struct {
	lookups int
	readyAt int
	tags    []string
}

func (l *lateTagger) Tag(types.EntityID, types.TagCardinality) ([]string, error) {
	l.lookups++
	if l.lookups < l.readyAt {
		return nil, nil
	}
	return l.tags, nil
}

func (l *lateTagger) GlobalTags(types.TagCardinality) ([]string, error) {
	return nil, nil
}

func newContainerCGroup() *cgroupModel.CacheEntry {
	return cgroupModel.NewCacheEntry(model.ContainerContext{ContainerID: "0123456789ab"}, model.CGroupContext{CGroupID: "cgroup"}, 1)
}

// TestTagsRetriedWhileCGroupLives checks that the tags of a container whose
// tags come late are resolved, however many retries it takes.
func TestTagsRetriedWhileCGroupLives(t *testing.T) {
	tagger := &lateTagger{readyAt: 10, tags: []string{"image_name:nginx", "image_tag:1.27"}}
	r := NewResolver(10, tagger, nil, nil)
	var resolved *Workload
	if err := r.RegisterListener(WorkloadSelectorResolved, func(workload *Workload) {
		resolved = workload
	}); err != nil {
		t.Fatalf("RegisterListener: %v", err)
	}

	r.onCGroupCreated(newContainerCGroup())
	for range 20 {
		r.retryWorkloadsWithoutTags()
	}

	if resolved == nil {
		t.Fatalf("tags unresolved after %d lookups", tagger.lookups)
	}
	if resolved.Selector.Image != "nginx" || resolved.Selector.Tag != "1.27" {
		t.Errorf("selector = %+v, want nginx:1.27", resolved.Selector)
	}
}

// TestTagsRetriesEndWithCGroup checks that a deleted cgroup leaves the queue of
// workloads waiting for their tags.
func TestTagsRetriesEndWithCGroup(t *testing.T) {
	r := NewResolver(10, &lateTagger{readyAt: math.MaxInt}, nil, nil)

	cgce := newContainerCGroup()
	r.onCGroupCreated(cgce)
	if n := len(r.workloadsWithoutTags); n != 1 {
		t.Fatalf("%d workloads waiting for their tags, want 1", n)
	}

	cgce.SetAsDeleted()
	r.retryWorkloadsWithoutTags()

	if n := len(r.workloadsWithoutTags); n != 0 {
		t.Errorf("%d workloads waiting for their tags after the deletion of their cgroup, want 0", n)
	}
}

// TestTagsQueueKeepsNewWorkloads checks that a full queue of workloads waiting
// for their tags drops the one that waited longest to take a new one.
func TestTagsQueueKeepsNewWorkloads(t *testing.T) {
	r := NewResolver(2, &lateTagger{readyAt: math.MaxInt}, nil, nil)
	for _, id := range []string{"a", "b", "c"} {
		r.onCGroupCreated(cgroupModel.NewCacheEntry(
			model.ContainerContext{ContainerID: containerutils.ContainerID(id)},
			model.CGroupContext{CGroupID: containerutils.CGroupID(id)}, 1))
	}

	var queued []string
	for len(r.workloadsWithoutTags) > 0 {
		queued = append(queued, string((<-r.workloadsWithoutTags).GCroupCacheEntry.GetContainerID()))
	}
	if len(queued) != 2 || queued[0] != "b" || queued[1] != "c" {
		t.Errorf("queued workloads = %v, want [b c]", queued)
	}
}
