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

// basePeerTags is the base set of peer tag precursors (tags from which peer tags
// are derived) we aggregate on when peer tag aggregation is enabled.
var basePeerTags = func() []string {
	r := semantics.DefaultRegistry()
	var precursors []string
	for _, concept := range peerTagConcepts {
		for _, info := range r.GetAttributePrecedence(concept) {
			precursors = append(precursors, info.Name)
		}
	}
	sort.Strings(precursors)
	return precursors
}()

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
