// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package empty implements empty functions for the tagger component interface.
package empty

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Tagger struct to embed in other taggers that do not implement some of the tagger component functions
type Tagger struct{}

// GetEntityHash returns the hash for the tags associated with the given entity
// Returns an empty string if the tags lookup fails
func (t *Tagger) GetEntityHash(types.EntityID, types.TagCardinality) string {
	return ""
}

// AgentTags returns the agent tags
// It relies on the container provider utils to get the Agent container ID
func (t *Tagger) AgentTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

// GlobalTags queries global tags that should apply to all data coming from the
// agent.
func (t *Tagger) GlobalTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

// SetNewCaptureTagger sets the tagger to be used when replaying a capture
func (t *Tagger) SetNewCaptureTagger(tagger.Component) {}

// ResetCaptureTagger resets the capture tagger to nil
func (t *Tagger) ResetCaptureTagger() {}

// EnrichTags extends a tag list with origin detection tags
func (t *Tagger) EnrichTags(tagset.TagsAccumulator, taggertypes.OriginInfo) {}

// ChecksCardinality defines the cardinality of tags we should send for check metrics
func (t *Tagger) ChecksCardinality() types.TagCardinality {
	return types.LowCardinality
}

// DogstatsdCardinality defines the cardinality of tags we should send for metrics from
// dogstatsd.
func (t *Tagger) DogstatsdCardinality() types.TagCardinality {
	return types.LowCardinality
}
