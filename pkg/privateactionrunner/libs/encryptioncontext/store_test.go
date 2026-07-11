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

func newPrivateKey(seed byte) *[32]byte {
	var key [32]byte
	for index := range key {
		key[index] = seed
	}
	return &key
}

func TestStoreGetAndDelete(t *testing.T) {
	cases := []struct {
		name          string
		setContextID  string
		takeContextID string
		wantFound     bool
	}{
		{
			name:          "set then get-and-delete succeeds and evicts entry",
			setContextID:  "ctx-1",
			takeContextID: "ctx-1",
			wantFound:     true,
		},
		{
			name:          "mismatched encryptionContextId fails",
			setContextID:  "ctx-1",
			takeContextID: "ctx-2",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := NewStoreWithTTL(time.Minute)

			privateKey := newPrivateKey(0x42)
			store.Set(testCase.setContextID, privateKey)

			retrieved, found := store.GetAndDelete(testCase.takeContextID)
			require.Equal(t, testCase.wantFound, found)
			if !testCase.wantFound {
				return
			}
			require.Equal(t, privateKey, retrieved)

			// Subsequent get-and-delete must miss because the first call evicts the entry.
			_, found = store.GetAndDelete(testCase.takeContextID)
			require.False(t, found)
		})
	}
}

func TestStoreSetOverwritesExistingContextID(t *testing.T) {
	store := NewStoreWithTTL(time.Minute)
	store.Set("ctx-1", newPrivateKey(0x01))
	store.Set("ctx-1", newPrivateKey(0x02))

	retrieved, found := store.GetAndDelete("ctx-1")
	require.True(t, found)
	require.Equal(t, newPrivateKey(0x02), retrieved)
}

func TestStoreConcurrentAccess(t *testing.T) {
	store := NewStoreWithTTL(time.Minute)

	const goroutineCount = 100
	var waitGroup sync.WaitGroup
	waitGroup.Add(goroutineCount)
	for index := range goroutineCount {
		go func(index int) {
			defer waitGroup.Done()
			store.Set(string(rune('a'+index%26)), newPrivateKey(byte(index)))
		}(index)
	}
	waitGroup.Wait()

	_, found := store.GetAndDelete("a")
	require.True(t, found)
}
