// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package collectors

import (
	"time"
)

// TagInfo holds the tag information for a given entity and source. It's meant
// to be created from collectors and read by the store.
type TagInfo struct {
	Source               string    // source collector's name
	Entity               string    // entity name ready for lookup
	HighCardTags         []string  // high cardinality tags that can create a lot of different timeseries (typically one per container, user request, etc.)
	OrchestratorCardTags []string  // orchestrator cardinality tags that have as many combination as pods/tasks
	LowCardTags          []string  // low cardinality tags safe for every pipeline
	StandardTags         []string  // the discovered standard tags (env, version, service) for the entity
	DeleteEntity         bool      // true if the entity is to be deleted from the store
	ExpiryDate           time.Time // keep in cache until expiryDate
}

// CollectorPriority helps resolving dupe tags from collectors
type CollectorPriority int

// List of collector priorities
const (
	NodeRuntime CollectorPriority = iota
	NodeOrchestrator
	ClusterOrchestrator
)

// TagCardinality indicates the cardinality-level of a tag.
// It can be low cardinality (in the host count order of magnitude)
// orchestrator cardinality (tags that change value for each pod, task, etc.)
// high cardinality (typically tags that change value for each web request, each container, etc.)
type TagCardinality int

// List of possible container cardinality
const (
	LowCardinality TagCardinality = iota
	OrchestratorCardinality
	HighCardinality
)
