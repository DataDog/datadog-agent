// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package probe

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type FieldHandlers struct {
	resolvers *resolvers.Resolvers
}

// ResolveEventTime resolves the monolitic kernel event timestamp to an absolute time
func (fh *FieldHandlers) ResolveEventTime(ev *model.Event) time.Time {
	ev.Timestamp = time.Now()
	return ev.Timestamp
}

// GetProcessService returns the service tag based on the process context
func (fh *FieldHandlers) GetProcessService(ev *model.Event) string {
	return ""
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (fh *FieldHandlers) ResolveEventTimestamp(ev *model.Event) int {
	return int(fh.ResolveEventTime(ev).UnixNano())
}

// ResolveProcessCacheEntry queries the ProcessResolver to retrieve the ProcessContext of the event
func (fh *FieldHandlers) ResolveProcessCacheEntry(ev *model.Event) (*model.ProcessCacheEntry, bool) {
	if ev.ProcessCacheEntry != nil {
		return ev.ProcessCacheEntry, true
	}
	return nil, false
}

// ResolveContainerContext queries the cgroup resolver to retrieve the ContainerContext of the event
func (fh *FieldHandlers) ResolveContainerContext(ev *model.Event) (*model.ContainerContext, bool) {
	return nil, false
}
