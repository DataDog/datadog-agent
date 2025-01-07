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

// hashEntityToUInt64 generates an uint64 hash for an Entity.
func hashEntityToUInt64(entity *Entity) uint64 {
	// Initialize a new FNV-1a hasher
	hasher := fnv.New64a()
	// Convert and write the entity's SourceID (string) to the hasher
	hasher.Write([]byte(entity.SourceID))
	// Convert and write the entity's host (string) to the hasher
	hasher.Write([]byte(entity.Host))
	// Convert and write the entity's namespace (string) to the hasher
	hasher.Write([]byte(entity.Namespace))
	// Convert and write the entity's LoadName (string) to the hasher
	hasher.Write([]byte(entity.LoadName))
	return hasher.Sum64()
}

// getCurrentTime returns the current time in uint32
func getCurrentTime() Timestamp {
	return timeToTimestamp(time.Now())
}

func timeToTimestamp(t time.Time) Timestamp {
	return Timestamp(t.Unix())
}
