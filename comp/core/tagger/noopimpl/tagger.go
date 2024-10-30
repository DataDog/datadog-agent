// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tagger implements the Tagger component. The Tagger is the central
// source of truth for client-side entity tagging. It subscribes to workloadmeta
// to get updates for all the entity kinds (containers, kubernetes pods,
// kubernetes nodes, etc.) and extracts the tags for each of them. Tags are then
// stored in memory (by the TagStore) and can be queried by the tagger.Tag()
// method.

// Package noopimpl provides a noop implementation for the tagger component
package noopimpl

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			NewTaggerClient,
		),
	)

}

type noopTagger struct{}

func (n *noopTagger) Start(context.Context) error {
	return nil
}

func (n *noopTagger) Stop() error {
	return nil
}

func (n *noopTagger) ReplayTagger() tagger.ReplayTagger {
	return nil
}

func (n *noopTagger) GetTaggerTelemetryStore() *telemetry.Store {
	return nil
}

func (n *noopTagger) Tag(types.EntityID, types.TagCardinality) ([]string, error) {
	return nil, nil
}

func (n *noopTagger) LegacyTag(string, types.TagCardinality) ([]string, error) {
	return nil, nil
}

func (n *noopTagger) AccumulateTagsFor(types.EntityID, types.TagCardinality, tagset.TagsAccumulator) error {
	return nil
}

func (n *noopTagger) Standard(types.EntityID) ([]string, error) {
	return nil, nil
}

func (n *noopTagger) List() types.TaggerListResponse {
	return types.TaggerListResponse{}
}

func (n *noopTagger) GetEntity(types.EntityID) (*types.Entity, error) {
	return nil, nil
}

func (n *noopTagger) Subscribe(string, *types.Filter) (types.Subscription, error) {
	return nil, fmt.Errorf("not implemented")
}

func (n *noopTagger) GetEntityHash(types.EntityID, types.TagCardinality) string {
	return ""
}

func (n *noopTagger) AgentTags(types.TagCardinality) ([]string, error) {
	return nil, nil
}

func (n *noopTagger) GlobalTags(types.TagCardinality) ([]string, error) {
	return nil, nil
}

func (n *noopTagger) SetNewCaptureTagger(tagger.Component) {}

func (n *noopTagger) ResetCaptureTagger() {}

func (n *noopTagger) EnrichTags(tagset.TagsAccumulator, taggertypes.OriginInfo) {}

func (n *noopTagger) ChecksCardinality() types.TagCardinality {
	return types.LowCardinality
}

func (n *noopTagger) DogstatsdCardinality() types.TagCardinality {
	return types.LowCardinality
}

// NewTaggerClient returns a new noop tagger client
func NewTaggerClient() tagger.Component {
	return &noopTagger{}
}
