// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package encryptioncontext stores ephemeral private keys generated
// by the `com.datadoghq.remoteaction.internal.prepareEncryption` action so
// that a subsequent task on the same runner can retrieve and use them to
// decrypt per-task secret inputs.
package encryptioncontext

import (
	"crypto/hpke"
	"time"

	"github.com/jellydator/ttlcache/v3"
)

// DefaultTTL is how long a key remains retrievable after being stored,
// when using NewStore.
const DefaultTTL = 5 * time.Minute

// Store keeps private keys indexed by encryptionContextID, evicted after
// their TTL elapses or once explicitly deleted.
type Store struct {
	cache *ttlcache.Cache[string, hpke.PrivateKey]
}

// NewStore returns an in-memory Store using DefaultTTL.
func NewStore() *Store {
	return NewStoreWithTTL(DefaultTTL)
}

// NewStoreWithTTL returns an in-memory Store. ttl is how long a key remains
// retrievable after being stored.
func NewStoreWithTTL(ttl time.Duration) *Store {
	return &Store{
		cache: ttlcache.New(ttlcache.WithTTL[string, hpke.PrivateKey](ttl)),
	}
}

// Set stores privateKey under encryptionContextID, overwriting any existing entry.
func (store *Store) Set(encryptionContextID string, privateKey hpke.PrivateKey) {
	store.cache.Set(encryptionContextID, privateKey, ttlcache.DefaultTTL)
}

// GetAndDelete retrieves and evicts the entry for encryptionContextID.
// Returns (nil, false) if no live entry exists.
func (store *Store) GetAndDelete(encryptionContextID string) (hpke.PrivateKey, bool) {
	item, found := store.cache.GetAndDelete(encryptionContextID)
	if !found {
		return nil, false
	}
	return item.Value(), true
}

// Start runs the eviction loop until Stop is called. Blocking; call in a goroutine.
func (store *Store) Start() {
	store.cache.Start()
}

// Stop terminates the eviction loop started by Start.
func (store *Store) Stop() {
	store.cache.Stop()
}
