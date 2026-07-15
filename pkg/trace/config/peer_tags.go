// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

// peerTagConcepts is the ordered list of semantic concepts whose source keys are
// collected as peer tag precursors for stats aggregation.
var peerTagConcepts = []semantics.Concept{
	semantics.ConceptDDBaseService,
	semantics.ConceptPeerService,
	semantics.ConceptPeerHostname,
	semantics.ConceptPeerDBName,
	semantics.ConceptPeerDBSystem,
	semantics.ConceptPeerCassandraContactPts,
	semantics.ConceptPeerCouchbaseSeedNodes,
	semantics.ConceptPeerMessagingDestination,
	semantics.ConceptPeerMessagingSystem,
	semantics.ConceptPeerKafkaBootstrapSrvs,
	semantics.ConceptPeerRPCService,
	semantics.ConceptPeerRPCSystem,
	semantics.ConceptPeerAWSS3Bucket,
	semantics.ConceptPeerAWSSQSQueue,
	semantics.ConceptPeerAWSDynamoDBTable,
	semantics.ConceptPeerAWSKinesisStream,
}

// PeerTagsCache is a snapshot of the peer-tag attribute key set together with
// the semantic registry content hash it was derived from. Callers that need
// to avoid recomputing on every read (e.g. the Concentrator's hot path)
// should hold a *PeerTagsCache and compare its ContentHash against
// semantics.DefaultRegistry().ContentHash() to decide when to rebuild via
// AgentConfig.PeerTagsCache.
type PeerTagsCache struct {
	// ContentHash is the registry content hash that Keys was derived from.
	ContentHash string
	// Keys is the sorted, deduped peer-tag attribute key set, or nil if
	// PeerTagsAggregation is disabled on the AgentConfig.
	Keys []string
}

// PeerTagsCache builds and returns a fresh PeerTagsCache snapshot from the
// live semantic registry combined with the operator-configured PeerTags.
// The returned ContentHash is the registry's ContentHash() at the time of the call.
// Returns a snapshot with nil Keys when PeerTagsAggregation is disabled.
func (c *AgentConfig) PeerTagsCache() *PeerTagsCache {
	r := semantics.DefaultRegistry()
	cache := &PeerTagsCache{ContentHash: r.ContentHash()}
	if !c.PeerTagsAggregation {
		return cache
	}
	cache.Keys = preparePeerTags(append(basePeerTags(r), c.PeerTags...))
	return cache
}

// basePeerTags returns the sorted list of peer-tag precursor attribute keys
// derived from r. Internal helper for PeerTagsCache and the package's tests.
func basePeerTags(r semantics.Registry) []string {
	var precursors []string
	for _, concept := range peerTagConcepts {
		for _, info := range r.GetAttributePrecedence(concept) {
			precursors = append(precursors, info.Name)
		}
	}
	sort.Strings(precursors)
	return precursors
}

func preparePeerTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	var deduped []string
	seen := make(map[string]struct{})
	for _, t := range tags {
		if _, ok := seen[t]; !ok {
			seen[t] = struct{}{}
			deduped = append(deduped, t)
		}
	}
	sort.Strings(deduped)
	return deduped
}
