// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package encryptioncontext

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newKey(seed byte) *[32]byte {
	var key [32]byte
	for index := range key {
		key[index] = seed
	}
	return &key
}

func TestStoreTake(t *testing.T) {
	cases := []struct {
		name                string
		putContextID        string
		advance             time.Duration
		takeContextID       string
		wantErr             error
		wantKeyOnSecondTake error
	}{
		{
			name:                "put then take succeeds and evicts entry",
			putContextID:        "ctx-1",
			takeContextID:       "ctx-1",
			wantKeyOnSecondTake: ErrNotFound,
		},
		{
			name:          "mismatched encryptionContextId fails",
			putContextID:  "ctx-1",
			takeContextID: "ctx-2",
			wantErr:       ErrNotFound,
		},
		{
			name:          "expired entry is not retrievable",
			putContextID:  "ctx-1",
			advance:       6 * time.Second,
			takeContextID: "ctx-1",
			wantErr:       ErrNotFound,
		},
		{
			name:          "expiry at TTL boundary is treated as expired",
			putContextID:  "ctx-1",
			advance:       5 * time.Second,
			takeContextID: "ctx-1",
			wantErr:       ErrNotFound,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			now := time.Unix(1_700_000_000, 0)
			clock := func() time.Time { return now }
			store := NewStoreWithTTL(5*time.Second, clock)

			privateKey := newKey(0x42)
			require.NoError(t, store.Put(testCase.putContextID, privateKey))
			now = now.Add(testCase.advance)

			retrieved, err := store.Take(testCase.takeContextID)
			if testCase.wantErr != nil {
				require.ErrorIs(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, privateKey, retrieved)

			// Subsequent take must miss because Take evicts the entry on success.
			_, err = store.Take(testCase.takeContextID)
			require.ErrorIs(t, err, testCase.wantKeyOnSecondTake)
		})
	}
}

func TestStoreMismatchedTakeDoesNotEvictOriginalEntry(t *testing.T) {
	store := NewStoreWithTTL(time.Minute, time.Now)
	require.NoError(t, store.Put("ctx-1", newKey(0x01)))

	_, err := store.Take("ctx-2")
	require.ErrorIs(t, err, ErrNotFound)

	_, err = store.Take("ctx-1")
	require.NoError(t, err)
}

func TestStoreEvictsAbandonedEntryAfterTTL(t *testing.T) {
	const ttl = 20 * time.Millisecond
	store := NewStoreWithTTL(ttl, time.Now)
	require.NoError(t, store.Put("ctx-1", newKey(0x01)))

	memStore, ok := store.(*memoryStore)
	require.True(t, ok)

	require.Eventually(t, func() bool {
		memStore.mutex.Lock()
		defer memStore.mutex.Unlock()
		_, present := memStore.entries["ctx-1"]
		return !present
	}, time.Second, time.Millisecond, "entry should be evicted by its wall-clock timer even though Take was never called")

	_, err := store.Take("ctx-1")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestStorePutRejectsDuplicateContextID(t *testing.T) {
	store := NewStoreWithTTL(time.Minute, time.Now)
	require.NoError(t, store.Put("ctx-1", newKey(0x01)))
	require.ErrorIs(t, store.Put("ctx-1", newKey(0x02)), ErrAlreadyExists)

	retrieved, err := store.Take("ctx-1")
	require.NoError(t, err)
	require.Equal(t, newKey(0x01), retrieved)
}

func TestStoreConcurrentAccess(t *testing.T) {
	store := NewStoreWithTTL(time.Minute, time.Now)

	const goroutineCount = 100
	var waitGroup sync.WaitGroup
	waitGroup.Add(goroutineCount)
	for index := range goroutineCount {
		go func(index int) {
			defer waitGroup.Done()
			store.Put(string(rune('a'+index%26)), newKey(byte(index)))
		}(index)
	}
	waitGroup.Wait()

	_, err := store.Take("a")
	require.NoError(t, err)
}
