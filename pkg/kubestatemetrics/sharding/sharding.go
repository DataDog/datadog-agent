// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sharding assigns KSM resources to shards using rendezvous hashing.
package sharding

import (
	"strconv"

	"github.com/cespare/xxhash/v2"
)

type HashKey string

const (
	// CriterionNamespace includes the Kubernetes namespace in a hash key.
	CriterionNamespace = "namespace"
	// CriterionResource includes the Kubernetes resource in a hash key.
	CriterionResource = "resource"
)

// NewHashKey accepts the criteria [namespace, resource] and an object's
// namespace and API resource type to form a hash key that's ultimately used
// to determine which KSM shard "owns" or "tracks" this object.
//
// NewHashKey(["namespace"], "datadog", "pods") -> "datadog"
// NewHashKey(["resource"], "datadog", "pods") -> "|pods"
// NewHashKey(["namespace", "resource"], "datadog", "pods") -> "datadog|pods"
func NewHashKey(criteria []string, namespace, resource string) HashKey {
	var byNamespace, byResource bool
	for _, criterion := range criteria {
		if criterion == CriterionNamespace {
			byNamespace = true
		}
		if criterion == CriterionResource {
			byResource = true
		}
	}

	switch {
	case byNamespace && byResource:
		return HashKey(namespace + "|" + resource)
	case byNamespace:
		return HashKey(namespace)
	case byResource:
		return HashKey("|" + resource)
	default:
		return ""
	}
}

// score accepts a HashKey and shard ID to calculate a "score"
// used for rendezvous hashing.
func score(key HashKey, shard int) uint64 {
	return xxhash.Sum64String(string(key) + "|" + strconv.Itoa(shard))
}

// ShardResponsibleForKey determines which shard owns a precomputed hash key
// using rendezvous hashing.
//
// The shard with the highest score owns the key. Ties favor the higher shard ID.
func ShardResponsibleForKey(count int, key HashKey) int {
	winningShard := 0
	winningScore := score(key, 0)

	for shard := 1; shard < count; shard++ {
		newScore := score(key, shard)
		if newScore >= winningScore {
			winningShard = shard
			winningScore = newScore
		}
	}
	return winningShard
}
