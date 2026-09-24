// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"context"

	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	cm "github.com/DataDog/datadog-agent/pkg/clustermetadata"
)

// RingTagger wraps the cluster-agent's local tagger so the existing tagger
// gRPC keeps answering complete pod tags once the pod cache is sharded: a
// pod query that misses the local (shard-local) cache falls back to the
// ring coordinator, which asks the peer replicas. The existing consumer
// surface and its clients need no changes of their own.
type RingTagger struct {
	tagger.Component
	store *LocalStore
}

// NewRingTagger wraps a local tagger with the ring fallback. It must only
// wrap the cluster-agent's local tagger: on other agent flavors the store
// does not exist and the local tagger is already complete.
func NewRingTagger(local tagger.Component, store *LocalStore) *RingTagger {
	return &RingTagger{Component: local, store: store}
}

// Tag returns the entity's tags at the given cardinality. Pod misses fall
// back to the ring: the coordinator asks the peer replicas and returns the
// owning replica's tags. Any other answer (absent, retry, peer failure)
// leaves the local result in place.
func (t *RingTagger) Tag(entityID taggertypes.EntityID, cardinality taggertypes.TagCardinality) ([]string, error) {
	tags, err := t.Component.Tag(entityID, cardinality)

	// Only pods can be on another shard; every other kind is replicated.
	if entityID.GetPrefix() != taggertypes.KubernetesPodUID {
		return tags, err
	}
	// A local hit needs no ring: one replica holds the pod, and it is us.
	if err == nil && len(tags) > 0 {
		return tags, err
	}

	answer, ringErr := t.store.LookupOrigin(context.Background(), cm.OriginLookupRequest{
		Key:   cm.OriginKey{PodUID: entityID.GetID()},
		Scope: cm.Scope{Cardinality: cardinality},
	})
	if ringErr != nil || answer.Kind != cm.AnswerFound {
		// The ring did not resolve the pod: surface the local answer —
		// its error or emptiness is the more informative failure.
		return tags, err
	}
	return answer.Tags, nil
}
