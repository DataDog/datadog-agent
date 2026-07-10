// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"sync/atomic"
)

// LockedItem is a single-use handle to one locked store entry, returned by Get.
//
// Upsert, Delete, and Release release the item lock and consume the handle; later
// terminal calls are no-ops, so `defer item.Release()` is safe.
type LockedItem[T any] struct {
	store    *Store[T]
	key      string
	kl       *keyLock
	value    T
	consumed atomic.Bool
}

// Value returns a shallow copy of the locked value, or the zero value when the key was
// missing or the handle has been consumed.
//
// Reference-typed fields still alias the stored value. Replace them before Upsert
// instead of mutating shared backing data in place.
func (it *LockedItem[T]) Value() T {
	if it.consumed.Load() {
		var zero T
		return zero
	}
	return it.value
}

// Upsert stores value, releases the lock, and notifies set observers. It is a no-op if
// the handle has already been consumed.
func (it *LockedItem[T]) Upsert(value T, sender SenderID) {
	if !it.consumed.CompareAndSwap(false, true) {
		return
	}

	it.store.storeValue(it.key, value)
	it.store.release(it.key, it.kl)

	it.store.notifySet(it.key, sender)
}

// Delete removes the key, releases the lock, and notifies delete observers when the
// key existed. It is a no-op if the handle has already been consumed.
func (it *LockedItem[T]) Delete(sender SenderID) {
	if !it.consumed.CompareAndSwap(false, true) {
		return
	}

	existed := it.store.removeValue(it.key)
	it.store.release(it.key, it.kl)

	if existed {
		it.store.notifyDelete(it.key, sender)
	}
}

// Release releases the lock without writing or notifying observers. It is a no-op if
// the handle has already been consumed.
func (it *LockedItem[T]) Release() {
	if !it.consumed.CompareAndSwap(false, true) {
		return
	}

	it.store.release(it.key, it.kl)
}
