// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"crypto/sha256"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ Deduper = (*LRUDeduper)(nil)

func TestNewLRUDeduperInvalidSize(t *testing.T) {
	_, err := NewLRUDeduper(0, 10, time.Hour)
	assert.Error(t, err)
}

func TestLRUDeduperIdentity(t *testing.T) {
	d, err := NewLRUDeduper(10, 10, time.Hour)
	require.NoError(t, err)

	now := time.Unix(1000, 0)
	id := Identity{MountID: 1, Inode: 2, CTime: 3}

	assert.False(t, d.IdentityFresh(id, now), "unknown identity")
	d.MarkIdentity(id, now)
	assert.True(t, d.IdentityFresh(id, now), "identity hit")
	assert.True(t, d.IdentityFresh(id, now.Add(59*time.Minute)), "before TTL")

	changed := id
	changed.CTime++
	assert.False(t, d.IdentityFresh(changed, now), "ctime change is a new identity")

	assert.False(t, d.IdentityFresh(id, now.Add(time.Hour)), "TTL expired")
	d.MarkIdentity(id, now.Add(time.Hour))
	assert.True(t, d.IdentityFresh(id, now.Add(time.Hour+time.Minute)), "re-marked after TTL")
}

func TestLRUDeduperNoTTL(t *testing.T) {
	d, err := NewLRUDeduper(10, 10, 0)
	require.NoError(t, err)

	now := time.Unix(1000, 0)
	id := Identity{MountID: 1, Inode: 2, CTime: 3}
	d.MarkIdentity(id, now)
	assert.True(t, d.IdentityFresh(id, now.Add(24*365*time.Hour)))
}

func TestLRUDeduperIdentityEviction(t *testing.T) {
	d, err := NewLRUDeduper(2, 10, time.Hour)
	require.NoError(t, err)

	now := time.Unix(1000, 0)
	a := Identity{Inode: 1}
	b := Identity{Inode: 2}
	c := Identity{Inode: 3}
	d.MarkIdentity(a, now)
	d.MarkIdentity(b, now)
	// a becomes the most recently used
	assert.True(t, d.IdentityFresh(a, now))
	d.MarkIdentity(c, now)

	assert.True(t, d.IdentityFresh(a, now))
	assert.False(t, d.IdentityFresh(b, now), "least recently used identity evicted")
	assert.True(t, d.IdentityFresh(c, now))

	identities, _ := d.Sizes()
	assert.Equal(t, 2, identities)
}

func TestLRUDeduperClaimRelease(t *testing.T) {
	for _, hashSetSize := range []int{10, 0} {
		t.Run("hash_set_size_"+strconv.Itoa(hashSetSize), func(t *testing.T) {
			d, err := NewLRUDeduper(10, hashSetSize, time.Hour)
			require.NoError(t, err)

			sum := sha256.Sum256([]byte("content"))
			other := sha256.Sum256([]byte("other"))

			assert.True(t, d.ClaimHash(sum), "first claim")
			assert.False(t, d.ClaimHash(sum), "claimed")
			assert.True(t, d.ClaimHash(other), "different content")

			_, hashes := d.Sizes()
			assert.Equal(t, 2, hashes)

			d.ReleaseHash(sum)
			_, hashes = d.Sizes()
			assert.Equal(t, 1, hashes)
			assert.True(t, d.ClaimHash(sum), "claim after release")
			assert.False(t, d.ClaimHash(other), "other still claimed")

			// releasing an unknown hash is a no-op
			d.ReleaseHash(sha256.Sum256([]byte("unknown")))
			_, hashes = d.Sizes()
			assert.Equal(t, 2, hashes)
		})
	}
}

func TestLRUDeduperHashEviction(t *testing.T) {
	d, err := NewLRUDeduper(10, 2, time.Hour)
	require.NoError(t, err)

	a := sha256.Sum256([]byte("a"))
	b := sha256.Sum256([]byte("b"))
	c := sha256.Sum256([]byte("c"))
	require.True(t, d.ClaimHash(a))
	require.True(t, d.ClaimHash(b))
	// a becomes the most recently used
	require.False(t, d.ClaimHash(a))
	require.True(t, d.ClaimHash(c))

	_, hashes := d.Sizes()
	assert.Equal(t, 2, hashes)
	assert.False(t, d.ClaimHash(a))
	assert.True(t, d.ClaimHash(b), "evicted hash can be claimed again")
}

func TestLRUDeduperConcurrentClaims(t *testing.T) {
	d, err := NewLRUDeduper(100, 100, time.Hour)
	require.NoError(t, err)

	sum := sha256.Sum256([]byte("content"))
	now := time.Unix(1000, 0)

	const goroutines = 64
	var (
		wg      sync.WaitGroup
		start   = make(chan struct{})
		claimed atomic.Int64
	)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			id := Identity{Inode: uint64(i % 8)}
			d.IdentityFresh(id, now)
			d.MarkIdentity(id, now)
			if d.ClaimHash(sum) {
				claimed.Add(1)
			}
			d.Sizes()
		}(i)
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, claimed.Load(), "exactly one caller owns the scan")
	identities, hashes := d.Sizes()
	assert.Equal(t, 8, identities)
	assert.Equal(t, 1, hashes)
}
