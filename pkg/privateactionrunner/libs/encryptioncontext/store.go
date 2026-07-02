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
	"errors"
	"sync"
	"time"
)

var ErrNotFound = errors.New("encryption context not found")

var ErrAlreadyExists = errors.New("encryption context already exists")

// Store keeps private keys indexed by encryptionContextID.
// Keys are evicted on Take, or automatically after their TTL elapses.
type Store interface {
	Put(encryptionContextID string, privateKey *[32]byte) error
	Take(encryptionContextID string) (*[32]byte, error)
}

type entry struct {
	privateKey *[32]byte
	expiresAt  time.Time
	timer      *time.Timer
}

type memoryStore struct {
	mutex   sync.Mutex
	entries map[string]entry
	ttl     time.Duration
	now     func() time.Time
}

// DefaultTTL is how long a key remains retrievable after Put when using NewStore.
const DefaultTTL = 5 * time.Minute

// NewStore returns an in-memory Store using DefaultTTL.
func NewStore(now func() time.Time) Store {
	return NewStoreWithTTL(DefaultTTL, now)
}

// NewStoreWithTTL returns an in-memory Store. ttl is how long a key remains
// retrievable after Put.
func NewStoreWithTTL(ttl time.Duration, now func() time.Time) Store {
	if now == nil {
		now = time.Now
	}
	return &memoryStore{
		entries: make(map[string]entry),
		ttl:     ttl,
		now:     now,
	}
}

func (store *memoryStore) Put(encryptionContextID string, privateKey *[32]byte) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	if _, ok := store.entries[encryptionContextID]; ok {
		return ErrAlreadyExists
	}
	store.entries[encryptionContextID] = entry{
		privateKey: privateKey,
		expiresAt:  store.now().Add(store.ttl),
		timer:      time.AfterFunc(store.ttl, func() { store.evict(encryptionContextID) }),
	}
	return nil
}

// evict removes encryptionContextID's entry once its wall-clock timer fires,
// guaranteeing removal after ttl.
func (store *memoryStore) evict(encryptionContextID string) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	delete(store.entries, encryptionContextID)
}

func (store *memoryStore) Take(encryptionContextID string) (*[32]byte, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	storedEntry, ok := store.entries[encryptionContextID]
	if !ok {
		return nil, ErrNotFound
	}
	delete(store.entries, encryptionContextID)
	storedEntry.timer.Stop()
	if !store.now().Before(storedEntry.expiresAt) {
		return nil, ErrNotFound
	}
	return storedEntry.privateKey, nil
}
