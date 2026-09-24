// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
)

// EntityKey identifies a Kubernetes object by kind, namespace and name.
// Kind strings are lowercase Kubernetes kinds ("pod", "deployment", "hpa").
type EntityKey struct {
	Kind      string
	Namespace string
	Name      string
}

// OriginKey identifies a pod by runtime identifiers, as sent by dogstatsd
type OriginKey struct {
	// PodUID is the Kubernetes pod UID. Either PodUID or ContainerID is set.
	PodUID string
	// ContainerID is a runtime container ID.
	ContainerID string
	// ContainerName narrows a PodUID to one container when set.
	ContainerName string
}

// Scope bounds what a query may return.
type Scope struct {
	Consumer    string
	Cardinality taggertypes.TagCardinality
}

// LookupRequest asks for the tags of EntityKey.
type LookupRequest struct {
	Key   EntityKey
	Scope Scope
}

// OriginLookupRequest asks for the tags of a pod identified by OriginKey.
type OriginLookupRequest struct {
	Key   OriginKey
	Scope Scope
}

// AnswerKind classifies a LookupAnswer.
type AnswerKind int

const (
	// AnswerFound: the entity is known and has tags.
	AnswerFound AnswerKind = iota
	// AnswerAbsent: an authoritative, synced replica does not know the entity.
	AnswerAbsent
	// AnswerNotMine: peer-forwarding answer; not authoritative for this key.
	AnswerNotMine
	// AnswerNotReady: authoritative for the key, but the informer cache has
	// not synced (e.g. the replica just absorbed the node after a peer died).
	// Retry after a short backoff.
	AnswerNotReady
)

// LookupAnswer is the response to Lookup and LookupOrigin.
type LookupAnswer struct {
	Kind AnswerKind
	Tags []string
}

// NodeEvent is one change for a node-scoped stream subscription.
// ShardSnapshot is the per-shard answer to an enumeration query.
// Consumers are responsible for merging snapshots from multiple DCA replicas.
// RingMember describes one metadata-serving replica.
