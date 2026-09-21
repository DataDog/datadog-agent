// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import "context"

// Store is the cluster workload metadata query contract.
//
// Note: This interface is implemented relative to a specific DCA replica.
type Store interface {
	// Lookup returns the tags of one named object, if the object is known to this DCA.
	Lookup(ctx context.Context, req LookupRequest) (LookupAnswer, error)

	// LookupOrigin resolves a pod by runtime identifiers (UID, container ID).
	LookupOrigin(ctx context.Context, req OriginLookupRequest) (LookupAnswer, error)

	// Subscribe streams metadata changes for one node. The stream
	// first delivers a burst of the node's current state, then changes. When
	// the owning replica changes, the stream ends with an error and the
	// consumer must resubscribe to the new owning replica.
	Subscribe(ctx context.Context, node string, scope Scope) (<-chan NodeEvent, func(), error)

	// Snapshot returns this replica's shard of an enumeration. Callers are responsible for merging snapshots across replicas.
	Snapshot(ctx context.Context, kind string, namespace string, scope Scope) (ShardSnapshot, error)

	// Ring returns the current membership view.
	Ring(ctx context.Context) (RingInfo, error)
}
