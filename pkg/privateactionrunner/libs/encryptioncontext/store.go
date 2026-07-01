// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package encryptioncontext stores ephemeral Curve25519 private keys generated
// by the `com.datadoghq.remoteaction.internal.prepareEncryption` action so
// that a subsequent task on the same runner can retrieve and use them to
// decrypt per-task secret inputs.
package encryptioncontext

import (
	"errors"
	"sync"
	"time"
)

// ErrNotFound is returned when no matching entry exists or it has expired.
var ErrNotFound = errors.New("encryption context not found")

// Store keeps Curve25519 private keys indexed by (boundTaskID, encryptionContextID).
// Keys are evicted on Take or when their TTL expires.
type Store interface {
	Put(boundTaskID, encryptionContextID string, privateKey *[32]byte)
	Take(boundTaskID, encryptionContextID string) (*[32]byte, error)
}

type entry struct {
	privateKey *[32]byte
	expiresAt  time.Time
}

type memoryStore struct {
	mutex   sync.Mutex
	entries map[string]entry
	ttl     time.Duration
	now     func() time.Time
}

// NewStore returns an in-memory Store. ttl is how long a key remains
// retrievable after Put. now is injectable for tests; pass time.Now in
// production.
func NewStore(ttl time.Duration, now func() time.Time) Store {
	if now == nil {
		now = time.Now
	}
	return &memoryStore{
		entries: make(map[string]entry),
		ttl:     ttl,
		now:     now,
	}
}

func entryKey(boundTaskID, encryptionContextID string) string {
	return boundTaskID + "|" + encryptionContextID
}

func (store *memoryStore) Put(boundTaskID, encryptionContextID string, privateKey *[32]byte) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.entries[entryKey(boundTaskID, encryptionContextID)] = entry{
		privateKey: privateKey,
		expiresAt:  store.now().Add(store.ttl),
	}
}

func (store *memoryStore) Take(boundTaskID, encryptionContextID string) (*[32]byte, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	key := entryKey(boundTaskID, encryptionContextID)
	storedEntry, ok := store.entries[key]
	if !ok {
		return nil, ErrNotFound
	}
	delete(store.entries, key)
	if !store.now().Before(storedEntry.expiresAt) {
		return nil, ErrNotFound
	}
	return storedEntry.privateKey, nil
}
