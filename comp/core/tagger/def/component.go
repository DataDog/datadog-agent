// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger provides the tagger interface for the Datadog Agent
package tagger

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// team: container-platform

// Component is the component type.
type Component interface {
	Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error)
	// TagWithCompleteness returns tags for an entity together with a boolean
	// indicating whether the entity's data is complete. An entity is complete
	// when all expected collectors have reported data for it. For containers,
	// completeness also considers the associated parent entity's completeness
	// (pod in Kubernetes, ECS task in ECS).
	//
	// Note: tags may not be complete immediately after an entity is first seen,
	// because the different workloadmeta collectors discover entity data at
	// different speeds.
	TagWithCompleteness(entityID types.EntityID, cardinality types.TagCardinality) ([]string, bool, error)
	GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error)
	Standard(entityID types.EntityID) ([]string, error)
	List() types.TaggerListResponse
	GetEntity(entityID types.EntityID) (*types.Entity, error)
	// subscriptionID is used for logging and debugging purposes
	Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error)
	GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string
	AgentTags(cardinality types.TagCardinality) ([]string, error)
	// GlobalTags returns the list of static tags that should be applied to all telemetry.
	// This is the set of tags that should be attached when host tags are absent.
	//
	// Cardinality levels:
	//   - LowCardinality: includes all static tags provided from the config and cluster-level tags
	//   - OrchestratorCardinality: includes the above and orch-level tags like TaskARN on ECS Fargate
	//   - HighCardinality: includes the above
	//   - ChecksConfigCardinality: alias defined via `checks_tag_cardinality` setting an above option
	GlobalTags(cardinality types.TagCardinality) ([]string, error)
	EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo)
}
