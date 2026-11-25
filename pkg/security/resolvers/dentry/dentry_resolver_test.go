// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dentry holds dentry related files
package dentry

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe/erpc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type fakeERPCRequester struct{}

func (fakeERPCRequester) Request(_ *erpc.Request) error {
	return nil
}

type fakePathnamesMap struct {
	entries map[model.PathKey]model.PathLeaf
}

func (m *fakePathnamesMap) Lookup(key, valueOut interface{}) error {
	var pk model.PathKey
	switch k := key.(type) {
	case model.PathKey:
		pk = k
	case []byte:
		if len(k) < model.PathKeySize {
			return errors.New("invalid key buffer")
		}
		pk.Inode = binary.NativeEndian.Uint64(k[0:8])
		pk.MountID = binary.NativeEndian.Uint32(k[8:12])
		pk.PathID = binary.NativeEndian.Uint32(k[12:16])
	default:
		return errors.New("unsupported key type")
	}

	leaf, ok := m.entries[pk]
	if !ok {
		return ErrEntryNotFound
	}
	out, ok := valueOut.(*model.PathLeaf)
	if !ok {
		return errors.New("unsupported valueOut type")
	}
	*out = leaf
	return nil
}

type erpcSegmentEntry struct {
	inode   uint64
	mountID uint32
	name    string
}

func writeERPCSegments(buf []byte, challenge uint32, entries []erpcSegmentEntry) {
	clear(buf)
	offset := 0
	for _, e := range entries {
		binary.NativeEndian.PutUint64(buf[offset:offset+8], e.inode)
		binary.NativeEndian.PutUint32(buf[offset+8:offset+12], e.mountID)
		binary.NativeEndian.PutUint32(buf[offset+12:offset+16], challenge)
		offset += 16
		copy(buf[offset:], e.name)
		offset += len(e.name)
		buf[offset] = 0
		offset++
	}
}

func TestComputeFilenameFromParts(t *testing.T) {
	type testEntry struct {
		name     string
		parts    [][]byte
		expected string
	}

	entries := []testEntry{
		{
			name:     "empty",
			parts:    [][]byte{},
			expected: "/",
		},
		{
			name: "basic",
			parts: [][]byte{
				[]byte("a"),
				[]byte("b"),
				[]byte("c"),
			},
			expected: "/c/b/a",
		},
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			assert.Equal(t, entry.expected, computeFilenameFromParts(entry.parts))
		})
	}
}

func TestERPCWarmsParentForMapFallback(t *testing.T) {
	dr, err := NewResolver(&config.Config{
		DentryCacheSize:            128,
		MapDentryResolutionEnabled: true,
	}, &statsd.NoOpClient{}, nil)
	require.NoError(t, err)

	dr.erpc = fakeERPCRequester{}
	dr.erpcSegment = make([]byte, 4096)
	dr.erpcSegmentSize = len(dr.erpcSegment)

	const mountID = uint32(7)
	const highPathID = uint32(0x00020000)

	leaf1Key := model.PathKey{Inode: 1001, MountID: mountID, PathID: highPathID | 0x0005}
	parentKey := model.PathKey{Inode: 200, MountID: mountID, PathID: highPathID}
	rootKey := model.PathKey{Inode: 1, MountID: mountID, PathID: highPathID}

	writeERPCSegments(dr.erpcSegment, dr.challenge, []erpcSegmentEntry{
		{inode: leaf1Key.Inode, mountID: mountID, name: "leaf1"},
		{inode: parentKey.Inode, mountID: mountID, name: "dir"},
		{inode: rootKey.Inode, mountID: mountID, name: "/"},
	})

	resolved, err := dr.ResolveFromERPC(leaf1Key, true, CallerFile)
	require.NoError(t, err)
	assert.Equal(t, "/dir/leaf1", resolved)

	// Add a second leaf in the kernel map whose parent carries the leaf's nonzero low PathID, while the parent itself was evicted from the kernel map.
	leaf2Key := model.PathKey{Inode: 1002, MountID: mountID, PathID: highPathID | 0x0001}
	var leaf2 model.PathLeaf
	leaf2.Parent = model.PathKey{Inode: parentKey.Inode, MountID: mountID, PathID: leaf2Key.PathID}
	leaf2.SetName("leaf2")
	dr.pathnames = &fakePathnamesMap{
		entries: map[model.PathKey]model.PathLeaf{
			leaf2Key: leaf2,
		},
	}

	resolved2, err := dr.ResolveFromMap(leaf2Key, true, CallerFile)
	require.NoError(t, err)
	assert.Equal(t, "/dir/leaf2", resolved2)

	mapHits := dr.hitsCounters[counterEntry{
		resolutionType: metrics.KernelMapsTag,
		resolution:     metrics.PathResolutionTag,
		caller:         CallerFile,
	}].Load()
	cacheHits := dr.hitsCounters[counterEntry{
		resolutionType: metrics.CacheTag,
		resolution:     metrics.PathResolutionTag,
		caller:         CallerFile,
	}].Load()
	assert.Equal(t, int64(1), mapHits)
	assert.Equal(t, int64(1), cacheHits)
}

func TestResolveFromERPCNilRequesterReturnsError(t *testing.T) {
	dr, err := NewResolver(&config.Config{
		DentryCacheSize:             128,
		ERPCDentryResolutionEnabled: true,
	}, &statsd.NoOpClient{}, nil)
	require.NoError(t, err)

	_, err = dr.ResolveFromERPC(model.PathKey{Inode: 10, MountID: 1, PathID: 1}, true, CallerFile)
	require.Error(t, err)
}

func TestResolveFromMapParentCacheFallbackEdgeCases(t *testing.T) {
	const mountID = uint32(7)
	const highPathID = uint32(0x00030000)

	t.Run("hardlink-fallback-does-not-corrupt-cached-parent", func(t *testing.T) {
		dr, err := NewResolver(&config.Config{
			DentryCacheSize:            128,
			MapDentryResolutionEnabled: true,
		}, &statsd.NoOpClient{}, nil)
		require.NoError(t, err)

		keyA := model.PathKey{Inode: 10, MountID: mountID, PathID: highPathID}
		keyB := model.PathKey{Inode: 20, MountID: mountID, PathID: highPathID}
		keyOther := model.PathKey{Inode: 30, MountID: mountID, PathID: highPathID}
		rootKey := model.PathKey{Inode: 1, MountID: mountID, PathID: highPathID}

		// Warm /a/b/other in userspace cache.
		dr.cacheInode(keyOther, PathEntry{Parent: keyB, Name: "other"})
		dr.cacheInode(keyB, PathEntry{Parent: keyA, Name: "b"})
		dr.cacheInode(keyA, PathEntry{Parent: model.PathKey{}, Name: "a"})

		// Resolve /a/b/hardlink via ResolveFromMap(cache=false, CallerFileHardLink) where b is evicted from pathnames (hitting dr.cache) and a is still in pathnames.
		leafKey := model.PathKey{Inode: 40, MountID: mountID, PathID: highPathID | 0x0009}
		var leaf model.PathLeaf
		leaf.Parent = model.PathKey{Inode: keyB.Inode, MountID: mountID, PathID: leafKey.PathID}
		leaf.SetName("hardlink")

		var leafA model.PathLeaf
		leafA.Parent = rootKey
		leafA.SetName("a")

		var rootLeaf model.PathLeaf
		rootLeaf.SetName("/")

		dr.pathnames = &fakePathnamesMap{
			entries: map[model.PathKey]model.PathLeaf{
				leafKey: leaf,
				keyA:    leafA,
				rootKey: rootLeaf,
			},
		}

		resolved, err := dr.ResolveFromMap(leafKey, false, CallerFileHardLink)
		require.NoError(t, err)
		assert.Equal(t, "/a/b/hardlink", resolved)

		// Verify dr.cache[b] was not corrupted and /a/b/other still resolves from cache.
		otherResolved, err := dr.ResolveFromCache(keyOther, CallerFile)
		require.NoError(t, err)
		assert.Equal(t, "/a/b/other", otherResolved)
	})

	t.Run("non-hardlink-cache-false-callers-do-not-fallback", func(t *testing.T) {
		dr, err := NewResolver(&config.Config{
			DentryCacheSize:            128,
			MapDentryResolutionEnabled: true,
		}, &statsd.NoOpClient{}, nil)
		require.NoError(t, err)

		parentKey := model.PathKey{Inode: 20, MountID: mountID, PathID: highPathID}
		dr.cacheInode(parentKey, PathEntry{Parent: model.PathKey{}, Name: "dir"})

		leafKey := model.PathKey{Inode: 40, MountID: mountID, PathID: highPathID}
		var leaf model.PathLeaf
		leaf.Parent = parentKey
		leaf.SetName("leaf")
		dr.pathnames = &fakePathnamesMap{
			entries: map[model.PathKey]model.PathLeaf{
				leafKey: leaf,
			},
		}

		for _, caller := range []Caller{CallerCGroup, CallerDiscarder} {
			_, err := dr.ResolveFromMap(leafKey, false, caller)
			require.Error(t, err)
		}
	})

	t.Run("bumped-high-path-id-misses-stale-parent-cache", func(t *testing.T) {
		dr, err := NewResolver(&config.Config{
			DentryCacheSize:            128,
			MapDentryResolutionEnabled: true,
		}, &statsd.NoOpClient{}, nil)
		require.NoError(t, err)

		staleParentKey := model.PathKey{Inode: 20, MountID: mountID, PathID: highPathID}
		dr.cacheInode(staleParentKey, PathEntry{Parent: model.PathKey{}, Name: "old-dir"})

		bumpedHighPathID := uint32(0x00040000)
		leafKey := model.PathKey{Inode: 40, MountID: mountID, PathID: bumpedHighPathID | 0x0001}
		var leaf model.PathLeaf
		leaf.Parent = model.PathKey{Inode: 20, MountID: mountID, PathID: leafKey.PathID}
		leaf.SetName("leaf")
		dr.pathnames = &fakePathnamesMap{
			entries: map[model.PathKey]model.PathLeaf{
				leafKey: leaf,
			},
		}

		_, err = dr.ResolveFromMap(leafKey, true, CallerFile)
		var notFound *ErrDentryPathKeyNotFound
		require.ErrorAs(t, err, &notFound)
	})
}
