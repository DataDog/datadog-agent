// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
)

// DefaultHashSetSize is a suggested bound for the sha256 set of an LRUDeduper. An entry costs
// roughly 100 bytes (key, LRU list element and map overhead), so the default is about 10 MB.
const DefaultHashSetSize = 100000

// LRUDeduper is the Deduper used by the pipeline. It is safe for concurrent use.
//
// The identity level is an LRU of Identity -> last check time, bounded by identityCacheSize.
// An identity is fresh until recheckTTL has elapsed since it was last marked.
//
// The content level is a set of sha256 sums. A claimed sum stays in the set after its scan
// completes: the contract has no "scan done" call, and none is needed since a sum that is being
// scanned and a sum that was scanned both mean "don't scan again". A dropped or failed scan
// removes it with ReleaseHash. The set is an LRU bounded by hashSetSize: evicting a sum only
// costs a rescan of that content the next time it is read, and it bounds memory against a host
// executing an unbounded number of distinct binaries. hashSetSize <= 0 makes it unbounded.
type LRUDeduper struct {
	recheckTTL time.Duration

	identityMu sync.Mutex
	identities *simplelru.LRU[Identity, time.Time]

	hashMu sync.Mutex
	// hashes is used when the set is bounded, unboundedHashes otherwise
	hashes          *simplelru.LRU[[32]byte, struct{}]
	unboundedHashes map[[32]byte]struct{}
}

// NewLRUDeduper returns a new LRUDeduper. identityCacheSize must be positive. hashSetSize <= 0
// makes the sha256 set unbounded. recheckTTL <= 0 means identities never expire (they can still
// be evicted by the LRU).
func NewLRUDeduper(identityCacheSize int, hashSetSize int, recheckTTL time.Duration) (*LRUDeduper, error) {
	if identityCacheSize <= 0 {
		return nil, errors.New("identity cache size must be positive")
	}
	identities, err := simplelru.NewLRU[Identity, time.Time](identityCacheSize, nil)
	if err != nil {
		return nil, err
	}

	d := &LRUDeduper{
		recheckTTL: recheckTTL,
		identities: identities,
	}

	if hashSetSize > 0 {
		d.hashes, err = simplelru.NewLRU[[32]byte, struct{}](hashSetSize, nil)
		if err != nil {
			return nil, err
		}
	} else {
		d.unboundedHashes = make(map[[32]byte]struct{})
	}

	return d, nil
}

// IdentityFresh implements Deduper
func (d *LRUDeduper) IdentityFresh(id Identity, now time.Time) bool {
	d.identityMu.Lock()
	defer d.identityMu.Unlock()

	lastChecked, ok := d.identities.Get(id)
	if !ok {
		return false
	}
	if d.recheckTTL > 0 && now.Sub(lastChecked) >= d.recheckTTL {
		return false
	}
	return true
}

// MarkIdentity implements Deduper
func (d *LRUDeduper) MarkIdentity(id Identity, now time.Time) {
	d.identityMu.Lock()
	defer d.identityMu.Unlock()
	d.identities.Add(id, now)
}

// ClaimHash implements Deduper
func (d *LRUDeduper) ClaimHash(sum [32]byte) bool {
	d.hashMu.Lock()
	defer d.hashMu.Unlock()

	if d.hashes != nil {
		// Get also refreshes the recency of an already known sum
		if _, ok := d.hashes.Get(sum); ok {
			return false
		}
		d.hashes.Add(sum, struct{}{})
		return true
	}

	if _, ok := d.unboundedHashes[sum]; ok {
		return false
	}
	d.unboundedHashes[sum] = struct{}{}
	return true
}

// ReleaseHash implements Deduper
func (d *LRUDeduper) ReleaseHash(sum [32]byte) {
	d.hashMu.Lock()
	defer d.hashMu.Unlock()

	if d.hashes != nil {
		d.hashes.Remove(sum)
		return
	}
	delete(d.unboundedHashes, sum)
}

// Sizes implements Deduper
func (d *LRUDeduper) Sizes() (identities int, hashes int) {
	d.identityMu.Lock()
	identities = d.identities.Len()
	d.identityMu.Unlock()

	d.hashMu.Lock()
	if d.hashes != nil {
		hashes = d.hashes.Len()
	} else {
		hashes = len(d.unboundedHashes)
	}
	d.hashMu.Unlock()

	return identities, hashes
}
