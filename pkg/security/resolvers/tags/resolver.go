// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags holds tags related files
package tags

import (
	"context"
	"errors"
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

// WorkloadType represents the type of workload resolved by the tags resolver
type WorkloadType int

const (
	// WorkloadTypeUnknown indicates the workload type could not be determined
	WorkloadTypeUnknown WorkloadType = iota
	// WorkloadTypePodSandbox indicates a Kubernetes sandbox/pause container
	WorkloadTypePodSandbox
	// WorkloadTypeContainer indicates a regular container
	WorkloadTypeContainer
	// WorkloadTypeCGroup indicates a systemd service cgroup
	WorkloadTypeCGroup
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
	Resolve(id containerutils.WorkloadID) []string
	ResolveWithErr(id containerutils.WorkloadID) (WorkloadType, []string, error)
	GetValue(id containerutils.WorkloadID, tag string) string
}

// DefaultResolver represents a default resolver based directly on the underlying tagger
type DefaultResolver struct {
	tagger Tagger
}

// Resolve returns the tags for the given id
func (t *DefaultResolver) Resolve(id containerutils.WorkloadID) []string {
	_, tags, _ := t.ResolveWithErr(id)
	return tags
}

// ResolveWithErr returns the tags for the given id
func (t *DefaultResolver) ResolveWithErr(id containerutils.WorkloadID) (WorkloadType, []string, error) {
	return t.resolveWorkloadTags(id)
}

// resolveWorkloadTags resolves tags for a workload ID, handling both container and cgroup workloads
func (t *DefaultResolver) resolveWorkloadTags(id containerutils.WorkloadID) (WorkloadType, []string, error) {
	if id == nil {
		return WorkloadTypeUnknown, nil, errors.New("nil workload id")
	}

	switch v := id.(type) {
	case containerutils.ContainerID:
		if len(v) == 0 {
			return WorkloadTypeUnknown, nil, errors.New("empty container id")
		}
		// Resolve as a container ID
		return GetTagsOfContainer(t.tagger, v)
	case containerutils.CGroupID:
		if len(v) == 0 {
			return WorkloadTypeUnknown, nil, errors.New("empty cgroup id")
		}
		// CGroup resolution is only supported on Linux
		return WorkloadTypeUnknown, nil, errors.New("cgroup resolution not supported on this platform")
	default:
		return WorkloadTypeUnknown, nil, fmt.Errorf("unknown workload id type: %T", id)
	}
}

// GetTagsOfContainer returns the tags for the given container ID.
// If no tags are found for the container ID, it falls back to checking if this is a
// sandbox/pause container by querying the KubernetesPodSandbox entity type.
// This is exported to share the code with other non-resolver users of the tagger.
func GetTagsOfContainer(tagger Tagger, containerID containerutils.ContainerID) (WorkloadType, []string, error) {
	if tagger == nil {
		return WorkloadTypeUnknown, nil, nil
	}

	// First, try to get tags using the container ID
	entityID := types.NewEntityID(types.ContainerID, string(containerID))
	tags, err := tagger.Tag(entityID, types.OrchestratorCardinality)
	if err != nil {
		return WorkloadTypeUnknown, nil, err
	}
	if len(tags) != 0 {
		return WorkloadTypeContainer, tags, nil
	}

	// If no tags found for the container ID, it might be a sandbox/pause container.
	// Sandbox containers are indexed separately under the KubernetesPodSandbox entity type
	// because they are filtered out from regular container collection in WorkloadMeta.
	sandboxEntityID := types.NewEntityID(types.KubernetesPodSandbox, string(containerID))
	sandboxTags, err := tagger.Tag(sandboxEntityID, types.OrchestratorCardinality)
	if err != nil {
		return WorkloadTypeUnknown, nil, err
	}
	if len(sandboxTags) == 0 {
		return WorkloadTypeUnknown, nil, nil
	}
	return WorkloadTypePodSandbox, sandboxTags, nil
}

// GetValue return the tag value for the given id and tag name
func (t *DefaultResolver) GetValue(id containerutils.WorkloadID, tag string) string {
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
