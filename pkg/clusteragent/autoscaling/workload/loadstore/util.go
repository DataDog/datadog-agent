// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package loadstore

import (
	"hash/fnv"
	"time"
)

// Timestamp is a uint32 representing a timestamp.
type Timestamp uint32

// generateHash generates a uint64 hash for an unknown number of strings.
func generateHash(strings ...string) uint64 {
	// Initialize a new FNV-1a hasher
	hasher := fnv.New64a()
	// Iterate over the strings and write each one to the hasher
	for _, str := range strings {
		hasher.Write([]byte(str))
	}
	return hasher.Sum64()
}

// hashEntityToUInt64 generates an uint64 hash for an Entity.
func hashEntityToUInt64(entity *Entity) uint64 {
	return generateHash(entity.EntityName, entity.Namespace, entity.MetricName)
}

// getCurrentTime returns the current time in uint32
func getCurrentTime() Timestamp {
	return timeToTimestamp(time.Now())
}

func timeToTimestamp(t time.Time) Timestamp {
	return Timestamp(t.Unix())
}
