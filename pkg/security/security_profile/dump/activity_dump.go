// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds activity dump related files
package dump

import (
	"slices"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ActivityDumpStatus defines the state of an activity dump
type ActivityDumpStatus int

const (
	// Stopped means that the ActivityDump is not active
	Stopped ActivityDumpStatus = iota
	// Disabled means that the ActivityDump is ready to be in running state, but we're missing the kernel space filters
	// to start retrieving events from kernel space
	Disabled
	// Paused means that the ActivityDump is ready to be in running state, but the kernel space filters have been configured
	// to prevent from being sent over the perf map
	Paused
	// Running means that the ActivityDump is active
	Running
)

// ActivityDump represents a profile during its event collection phase
type ActivityDump struct {
	Cookie             uint64             // Shouldn't be changed after the activity dump is created
	onNeedNewTracedPid OnNeedNewTracedPid // Shouldn't be changed after the activity dump is created

	Profile    *profile.Profile
	LoadConfig *atomic.Pointer[model.ActivityDumpLoadConfig]

	m                sync.Mutex // Protects the fields below
	state            ActivityDumpStatus
	countedByLimiter bool
}

// WithDumpOption can be used to configure an ActivityDump
type WithDumpOption func(ad *ActivityDump)

// OnNeedNewTracedPid is a callback function used to notify the caller that a new pid should be traced
type OnNeedNewTracedPid func(ad *ActivityDump, pid uint32)

// NewEmptyActivityDump returns a new zero-like instance of an ActivityDump
func NewEmptyActivityDump(pathsReducer *activity_tree.PathsReducer, differentiateArgs bool, dnsMatchMaxDepth int, eventTypes []model.EventType, onNeedNewTracedPid OnNeedNewTracedPid) *ActivityDump {
	ad := &ActivityDump{
		Cookie:             utils.NewCookie(),
		LoadConfig:         atomic.NewPointer(&model.ActivityDumpLoadConfig{}),
		onNeedNewTracedPid: onNeedNewTracedPid,
	}

	ad.Profile = profile.New(
		profile.WithPathsReducer(pathsReducer),
		profile.WithDifferentiateArgs(differentiateArgs),
		profile.WithDNSMatchMaxDepth(dnsMatchMaxDepth),
		profile.WithEventTypes(eventTypes),
	)
	ad.Profile.SetTreeType(ad, "activity_dump")

	return ad
}

// NewActivityDump returns a new instance of an ActivityDump
func NewActivityDump(pathsReducer *activity_tree.PathsReducer, differentiateArgs bool, dnsMatchMaxDepth int, eventTypes []model.EventType, onNeedNewTracedPid OnNeedNewTracedPid, loadConfig *model.ActivityDumpLoadConfig, options ...WithDumpOption) *ActivityDump {
	ad := NewEmptyActivityDump(pathsReducer, differentiateArgs, dnsMatchMaxDepth, eventTypes, onNeedNewTracedPid)

	ad.LoadConfig.Store(loadConfig)

	for _, option := range options {
		option(ad)
	}

	return ad
}

// activity_tree.Owner interface

// MatchesSelector returns true if the provided entry matches the selector of the activity dump
func (ad *ActivityDump) MatchesSelector(entry *model.ProcessCacheEntry) bool {
	if entry == nil {
		return false
	}

	if len(ad.Profile.Metadata.ContainerID) > 0 {
		if ad.Profile.Metadata.ContainerID != entry.ContainerContext.ContainerID {
			return false
		}
	}

	if len(ad.Profile.Metadata.CGroupContext.CGroupID) > 0 {
		if ad.Profile.Metadata.CGroupContext.CGroupID != entry.CGroup.CGroupID {
			return false
		}
	}

	return true
}

// IsEventTypeValid returns true if the provided event type is traced by the activity dump
func (ad *ActivityDump) IsEventTypeValid(event model.EventType) bool {
	return slices.Contains(ad.LoadConfig.Load().TracedEventTypes, event)
}

// NewProcessNodeCallback is a callback function used to propagate the fact that a new process node was added to the
// activity tree
func (ad *ActivityDump) NewProcessNodeCallback(p *activity_tree.ProcessNode) {
	if ad.onNeedNewTracedPid != nil {
		ad.onNeedNewTracedPid(ad, p.Process.Pid)
	}
}

// ActivityDump funcs

// Insert inserts an event into the activity dump
func (ad *ActivityDump) Insert(event *model.Event, resolvers *resolvers.EBPFResolvers) (bool, int64, error) {
	ad.m.Lock()
	defer ad.m.Unlock()

	if ad.state != Running {
		// this activity dump is not running, ignore event
		return false, 0, nil
	}

	if !ad.MatchesSelector(event.ProcessCacheEntry) {
		return false, 0, nil
	}

	imageTag := ad.Profile.GetTagValue("image_tag")
	return ad.Profile.InsertAndGetSize(event, true, imageTag, activity_tree.Runtime, resolvers)
}

// SetState sets the state of the activity dump
func (ad *ActivityDump) SetState(state ActivityDumpStatus) {
	ad.m.Lock()
	defer ad.m.Unlock()
	ad.state = state
}

// GetState returns the state of the activity dump
func (ad *ActivityDump) GetState() ActivityDumpStatus {
	ad.m.Lock()
	defer ad.m.Unlock()
	return ad.state
}

// GetSelectorStr returns the string representation of the activity dump
func (ad *ActivityDump) GetSelectorStr() string {
	ad.m.Lock()
	defer ad.m.Unlock()

	if len(ad.Profile.Metadata.ContainerID) > 0 {
		return string(ad.Profile.Metadata.ContainerID)
	} else if len(ad.Profile.Metadata.CGroupContext.CGroupID) > 0 {
		return string(ad.Profile.Metadata.CGroupContext.CGroupID)
	}

	return "empty-selector"
}

// GetTimeout returns the timeout of the activity dump
func (ad *ActivityDump) GetTimeout() time.Duration {
	return ad.LoadConfig.Load().Timeout
}

// IsCountedByLimiter returns true if the activity dump is counted by the rate limiter
func (ad *ActivityDump) IsCountedByLimiter() bool {
	ad.m.Lock()
	defer ad.m.Unlock()
	return ad.countedByLimiter
}

// SetCountedByLimiter sets the flag indicating this activity dump is counted by the rate limiter
func (ad *ActivityDump) SetCountedByLimiter(countedByLimiter bool) {
	ad.m.Lock()
	defer ad.m.Unlock()
	ad.countedByLimiter = countedByLimiter
}
