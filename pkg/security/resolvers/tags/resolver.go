// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags holds tags related files
package tags

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Event defines the tags event type
type Event int

const (
	// WorkloadSelectorResolved is used to notify that a new cgroup with a resolved workload selector is ready
	WorkloadSelectorResolved Event = iota
	// WorkloadSelectorDeleted is used to notify that a cgroup with a resolved workload selector is deleted
	WorkloadSelectorDeleted
)

// Tagger defines a Tagger for the Tags Resolver
type Tagger interface {
	Tag(entity types.EntityID, cardinality types.TagCardinality) ([]string, error)
	GlobalTags(cardinality types.TagCardinality) ([]string, error)
}

// Resolver represents a cache resolver
type Resolver interface {
	Start(ctx context.Context) error
	Stop() error
	Resolve(id interface{}) []string
	ResolveWithErr(id interface{}) ([]string, error)
	GetValue(id interface{}, tag string) string
}

// DefaultResolver represents a default resolver based directly on the underlying tagger
type DefaultResolver struct {
	tagger Tagger
}

// Resolve returns the tags for the given id
func (t *DefaultResolver) Resolve(id interface{}) []string {
	tags, _ := t.ResolveWithErr(id)
	return tags
}

// ResolveWithErr returns the tags for the given id
func (t *DefaultResolver) ResolveWithErr(id interface{}) ([]string, error) {
	return t.resolveWorkloadTags(id)
}

// resolveWorkloadTags resolves tags for a workload ID, handling both container and cgroup workloads
func (t *DefaultResolver) resolveWorkloadTags(id interface{}) ([]string, error) {
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
		// CGroup resolution is only supported on Linux
		return nil, fmt.Errorf("cgroup resolution not supported on this platform")
	default:
		return nil, fmt.Errorf("unknown workload id type: %T", id)
	}
}

// GetTagsOfContainer returns the tags for the given container id
// exported to share the code with other non-resolver users of tagger
func GetTagsOfContainer(tagger Tagger, containerID containerutils.ContainerID) ([]string, error) {
	if tagger == nil {
		return nil, nil
	}

	entityID := types.NewEntityID(types.ContainerID, string(containerID))
	return tagger.Tag(entityID, types.OrchestratorCardinality)
}

// GetValue return the tag value for the given id and tag name
func (t *DefaultResolver) GetValue(id interface{}, tag string) string {
	return utils.GetTagValue(tag, t.Resolve(id))
}

// Start the resolver
func (t *DefaultResolver) Start(_ context.Context) error {
	return nil
}

// Stop the resolver
func (t *DefaultResolver) Stop() error {
	return nil
}

// NewDefaultResolver returns a new default tags resolver
func NewDefaultResolver(tagger Tagger) *DefaultResolver {
	if tagger == nil {
		seclog.Errorf("initializing tags resolver with nil tagger")
	}

	return &DefaultResolver{
		tagger: tagger,
	}
}
