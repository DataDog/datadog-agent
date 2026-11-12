// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import "sync"

// shardTracker records, for an original config's digest, the digests of the
// shard configs a sharding strategy (KSM resource-group sharding, generic
// instance sharding) produced for it. This lets Schedule no-op instead of
// re-sharding on a replay of an already-sharded config, lets Unschedule
// remove every shard a given original config produced, and lets placement
// keep the shards of one config apart.
type shardTracker struct {
	mu      sync.Mutex
	digests map[string][]string // original config digest -> shard digests
	parents map[string]string   // shard digest -> original config digest
}

func newShardTracker() *shardTracker {
	return &shardTracker{digests: make(map[string][]string), parents: make(map[string]string)}
}

// isTracked returns whether configDigest already has shards recorded.
func (t *shardTracker) isTracked(configDigest string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	_, exists := t.digests[configDigest]
	return exists
}

// mark records the shard digests produced for configDigest.
func (t *shardTracker) mark(configDigest string, shardDigests []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.digests[configDigest] = shardDigests
	for _, shardDigest := range shardDigests {
		t.parents[shardDigest] = configDigest
	}
}

// pop atomically returns and clears the shard digests recorded for
// configDigest, if any. The read-then-delete happens under a single lock to
// avoid a TOCTOU race against a concurrent mark/pop.
func (t *shardTracker) pop(configDigest string) ([]string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	shardDigests, exists := t.digests[configDigest]
	if !exists {
		return nil, false
	}

	digestsCopy := make([]string, len(shardDigests))
	copy(digestsCopy, shardDigests)
	delete(t.digests, configDigest)
	for _, shardDigest := range shardDigests {
		delete(t.parents, shardDigest)
	}
	return digestsCopy, true
}

// siblings returns the digests of the other shards produced from the same
// original config as shardDigest, or nil if shardDigest is not a shard. A nil
// tracker tracks no shards.
func (t *shardTracker) siblings(shardDigest string) []string {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	parent, isShard := t.parents[shardDigest]
	if !isShard {
		return nil
	}
	var siblings []string
	for _, digest := range t.digests[parent] {
		if digest != shardDigest {
			siblings = append(siblings, digest)
		}
	}
	return siblings
}

// parentsOf maps each of the given digests that is a shard to the digest of the
// original config it was produced from. Digests that are not shards are omitted.
// A nil tracker tracks no shards.
func (t *shardTracker) parentsOf(digests []string) map[string]string {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	parents := make(map[string]string)
	for _, digest := range digests {
		if parent, isShard := t.parents[digest]; isShard {
			parents[digest] = parent
		}
	}
	return parents
}

// reset clears all tracked shards.
func (t *shardTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.digests = make(map[string][]string)
	t.parents = make(map[string]string)
}
