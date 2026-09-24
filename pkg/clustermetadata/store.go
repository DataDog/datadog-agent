// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import "context"

// Store is the cluster workload metadata query contract. The cluster-agent
// runs the coordinator implementation (local caches plus the peer fanout);
// a peer client implements it against another replica's local-answer
// service. Both sides see the same interface.
type Store interface {
	// Lookup returns the tags of one named object. Workload kinds are
	// replicated, so any replica answers them. Pod misses defer to the
	// coordinator: the answer comes from the owning replica or the
	// reduced answers of the whole ring.
	Lookup(ctx context.Context, req LookupRequest) (LookupAnswer, error)

	// LookupOrigin resolves a pod by runtime identifiers (UID, container
	// ID). On the coordinator, a local miss fans out to the peers; first
	// AnswerFound wins, see AnswerKind for the full protocol.
	LookupOrigin(ctx context.Context, req OriginLookupRequest) (LookupAnswer, error)
}
