// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// FieldHandlers defines a field handlers
type FieldHandlers struct {
	resolvers *resolvers.Resolvers
}

// ResolveEventTimestamp resolves the monolitic kernel event timestamp to an absolute time
func (fh *FieldHandlers) ResolveEventTimestamp(ev *model.Event, e *model.BaseEvent) int {
	return int(fh.ResolveEventTime(ev).UnixNano())
}

func bestGuessServiceTag(serviceValues []string) string {
	if len(serviceValues) == 0 {
		return ""
	}

	firstGuess := serviceValues[0]

	// first we sort base on len, biggest len first
	sort.Slice(serviceValues, func(i, j int) bool {
		return len(serviceValues[j]) < len(serviceValues[i]) // reverse
	})

	// we then compare [i] and [i + 1] to check if [i + 1] is a prefix of [i]
	for i := 0; i < len(serviceValues)-1; i++ {
		if !strings.HasPrefix(serviceValues[i], serviceValues[i+1]) {
			// if it's not a prefix it means we have multiple disjoints services
			// we then return the first guess, closest in the process tree
			return firstGuess
		}
	}

	// we have a prefix chain, let's return the biggest one
	return serviceValues[0]
}

// GetProcessService returns the service tag based on the process context
func (fh *FieldHandlers) GetProcessService(ev *model.Event) string {
	entry, _ := fh.ResolveProcessCacheEntry(ev)
	if entry == nil {
		return ""
	}

	var serviceValues []string

	// first search in the process context itself
	if entry.EnvsEntry != nil {
		if service := entry.EnvsEntry.Get(ServiceEnvVar); service != "" {
			serviceValues = append(serviceValues, service)
		}
	}

	inContainer := entry.ContainerID != ""

	// while in container check for each ancestor
	for ancestor := entry.Ancestor; ancestor != nil; ancestor = ancestor.Ancestor {
		if inContainer && ancestor.ContainerID == "" {
			break
		}

		if ancestor.EnvsEntry != nil {
			if service := ancestor.EnvsEntry.Get(ServiceEnvVar); service != "" {
				serviceValues = append(serviceValues, service)
			}
		}
	}

	return bestGuessServiceTag(serviceValues)
}

// ResolveContainerID resolves the container ID of the event
func (fh *FieldHandlers) ResolveContainerID(ev *model.Event, e *model.ContainerContext) string {
	if len(e.ID) == 0 {
		if entry, _ := fh.ResolveProcessCacheEntry(ev); entry != nil {
			e.ID = entry.ContainerID
		}
	}
	return e.ID
}

// ResolveContainerCreatedAt resolves the container creation time of the event
func (fh *FieldHandlers) ResolveContainerCreatedAt(ev *model.Event, e *model.ContainerContext) int {
	if e.CreatedAt == 0 {
		if containerContext, _ := fh.ResolveContainerContext(ev); containerContext != nil {
			e.CreatedAt = containerContext.CreatedAt
		}
	}
	return int(e.CreatedAt)
}

// ResolveContainerTags resolves the container tags of the event
func (fh *FieldHandlers) ResolveContainerTags(ev *model.Event, e *model.ContainerContext) []string {
	if len(e.Tags) == 0 && e.ID != "" {
		e.Tags = fh.resolvers.TagsResolver.Resolve(e.ID)
	}
	return e.Tags
}

// ResolveProcessCreatedAt resolves process creation time
func (fh *FieldHandlers) ResolveProcessCreatedAt(ev *model.Event, e *model.Process) int {
	return int(e.ExecTime.UnixNano())
}
