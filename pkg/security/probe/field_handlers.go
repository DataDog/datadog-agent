// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package probe holds probe related files
package probe

import (
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
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
		service := entry.EnvsEntry.Get(ServiceEnvVar)
		if service == "" {
			service = entry.EnvsEntry.Get(WorkloadServiceEnvVar)
		}
		if service != "" {
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

func (fh *FieldHandlers) resolveContainerEnvTags(ev *model.Event, e *model.ContainerContext) {
	if !e.EnvTagsResolved {
		envp := fh.ResolveProcessEnvp(ev, &ev.ProcessContext.Process)
		for _, env := range envp {
			if split := strings.SplitN(env, "=", 2); len(split) > 1 {
				if containerTagKey, found := workloadLabelsAsEnvVars[split[0]]; found {
					containerTag := fmt.Sprintf("%s:%s", containerTagKey, split[1])
					e.Tags = append(e.Tags, containerTag)
				}
			}
		}
		e.EnvTagsResolved = true
	}
}

func (fh *FieldHandlers) resolveContainerRemoteTags(_ *model.Event, e *model.ContainerContext) {
	if !e.RemoteTagsResolved {
		if remoteTags := fh.resolvers.TagsResolver.Resolve(e.ID); len(remoteTags) > 0 {
			e.Tags = utils.ConcatenateTags(remoteTags, e.Tags)
			e.RemoteTagsResolved = true
		}
	}
}

// ResolveContainerTags resolves the container tags of the event
func (fh *FieldHandlers) ResolveContainerTags(ev *model.Event, e *model.ContainerContext) []string {
	if e.ID != "" {
		fh.resolveContainerEnvTags(ev, e)
		fh.resolveContainerRemoteTags(ev, e)
	}

	return e.Tags
}

// ResolveProcessCreatedAt resolves process creation time
func (fh *FieldHandlers) ResolveProcessCreatedAt(_ *model.Event, e *model.Process) int {
	return int(e.ExecTime.UnixNano())
}

// ResolveK8SUsername resolves the k8s username of the event
func (fh *FieldHandlers) ResolveK8SUsername(_ *model.Event, evtCtx *model.UserSessionContext) string {
	if !evtCtx.Resolved {
		if ctx := fh.resolvers.UserSessions.ResolveUserSession(evtCtx.ID); ctx != nil {
			*evtCtx = *ctx
		}
	}
	return evtCtx.K8SUsername
}

// ResolveK8SUID resolves the k8s UID of the event
func (fh *FieldHandlers) ResolveK8SUID(_ *model.Event, evtCtx *model.UserSessionContext) string {
	if !evtCtx.Resolved {
		if ctx := fh.resolvers.UserSessions.ResolveUserSession(evtCtx.ID); ctx != nil {
			*evtCtx = *ctx
		}
	}
	return evtCtx.K8SUID
}

// ResolveK8SGroups resolves the k8s groups of the event
func (fh *FieldHandlers) ResolveK8SGroups(_ *model.Event, evtCtx *model.UserSessionContext) []string {
	if !evtCtx.Resolved {
		if ctx := fh.resolvers.UserSessions.ResolveUserSession(evtCtx.ID); ctx != nil {
			*evtCtx = *ctx
		}
	}
	return evtCtx.K8SGroups
}

// ResolveK8SExtra resolves the k8s extra of the event
func (fh *FieldHandlers) ResolveK8SExtra(_ *model.Event, evtCtx *model.UserSessionContext) map[string][]string {
	if !evtCtx.Resolved {
		if ctx := fh.resolvers.UserSessions.ResolveUserSession(evtCtx.ID); ctx != nil {
			*evtCtx = *ctx
		}
	}
	return evtCtx.K8SExtra
}
