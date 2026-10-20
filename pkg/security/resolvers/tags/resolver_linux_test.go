// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tags

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/require"
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

func TestCGroupLifecycleIgnoresStaleNotifications(t *testing.T) {
	r := NewResolver(10, &lateTagger{
		readyAt: 1,
		tags:    []string{"image_name:nginx", "image_tag:1.27"},
	}, nil, nil)

	oldEntry := newContainerCGroup()
	replacement := newContainerCGroup()
	r.onCGroupCreated(oldEntry)
	r.onCGroupCreated(replacement)
	r.onCGroupDeleted(oldEntry)

	r.workloadsLock.Lock()
	workload := r.workloads[replacement.GetCGroupID()]
	r.workloadsLock.Unlock()
	if workload == nil || workload.GCroupCacheEntry != replacement {
		t.Fatal("stale deletion removed the replacement workload")
	}

	staleCreation := newContainerCGroup()
	staleCreation.SetAsDeleted()
	r.onCGroupCreated(staleCreation)

	r.workloadsLock.Lock()
	workload = r.workloads[replacement.GetCGroupID()]
	r.workloadsLock.Unlock()
	if workload == nil || workload.GCroupCacheEntry != replacement {
		t.Fatal("stale creation replaced the live workload")
	}
}

type missingCGroupFS struct{}

func (missingCGroupFS) FindCGroupContext(uint32, uint32) (containerutils.ContainerID, utils.CGroupContext, string, error) {
	return "", utils.CGroupContext{}, "", errors.New("not found")
}

func (missingCGroupFS) GetCGroupPids(string) ([]uint32, error) {
	return nil, nil
}

func TestCGroupLifecycleNotificationsPreserveOrder(t *testing.T) {
	cgroupResolver, err := cgroup.NewResolver(&statsd.NoOpClient{}, missingCGroupFS{}, nil)
	require.NoError(t, err)

	deletionStarted := make(chan struct{})
	releaseDeletion := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			close(releaseDeletion)
		})
	}
	t.Cleanup(release)

	require.NoError(t, cgroupResolver.RegisterListener(cgroup.CGroupDeleted, func(*cgroupModel.CacheEntry) {
		close(deletionStarted)
		<-releaseDeletion
	}))

	r := NewResolver(10, &lateTagger{readyAt: math.MaxInt}, cgroupResolver, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	require.NoError(t, r.Start(ctx))

	cgroupID := containerutils.CGroupID("/system.slice/test.service")
	oldEntry := cgroupResolver.AddPID(1001, model.CGroupContext{
		CGroupID:      cgroupID,
		CGroupPathKey: model.PathKey{MountID: 1, Inode: 1001},
		CGroupSource:  model.CGroupSourceEvent,
	})
	require.NotNil(t, oldEntry)

	deleteDone := make(chan struct{})
	go func() {
		defer close(deleteDone)
		cgroupResolver.Delete(1001)
	}()

	select {
	case <-deletionStarted:
	case <-time.After(time.Second):
		t.Fatal("deletion notification did not start")
	}

	replacementResult := make(chan *cgroupModel.CacheEntry, 1)
	go func() {
		replacementResult <- cgroupResolver.AddPID(1002, model.CGroupContext{
			CGroupID:      cgroupID,
			CGroupPathKey: model.PathKey{MountID: 1, Inode: 1002},
			CGroupSource:  model.CGroupSourceEvent,
		})
	}()

	require.Eventually(t, func() bool {
		entry := cgroupResolver.GetCacheEntryByCgroupID(cgroupID)
		return entry != nil && entry != oldEntry
	}, time.Second, time.Millisecond)

	release()

	var replacement *cgroupModel.CacheEntry
	select {
	case replacement = <-replacementResult:
	case <-time.After(time.Second):
		t.Fatal("replacement creation did not complete")
	}
	select {
	case <-deleteDone:
	case <-time.After(time.Second):
		t.Fatal("deletion did not complete")
	}

	r.workloadsLock.Lock()
	workload := r.workloads[cgroupID]
	r.workloadsLock.Unlock()
	require.NotNil(t, workload)
	require.Same(t, replacement, workload.GCroupCacheEntry)
	require.False(t, workload.GCroupCacheEntry.IsDeleted())
}
