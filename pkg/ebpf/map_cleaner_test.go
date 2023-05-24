// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"testing"
	"time"

	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapCleaner(t *testing.T) {
	const numMapEntries = 100

	var (
		key = new(int64)
		val = new(int64)
	)

	err := rlimit.RemoveMemlock()
	require.NoError(t, err)

	m, err := cebpf.NewMap(&cebpf.MapSpec{
		Type:       cebpf.Hash,
		KeySize:    8,
		ValueSize:  8,
		MaxEntries: numMapEntries,
	})
	require.NoError(t, err)

	cleaner, err := NewMapCleaner(m, key, val)
	require.NoError(t, err)
	for i := 0; i < numMapEntries; i++ {
		*key = int64(i)
		err := m.Put(key, val)
		assert.Nilf(t, err, "can't put key=%d: %s", i, err)
	}

	// Clean all the even entries
	cleaner.Clean(100*time.Millisecond, func(now int64, k, v interface{}) bool {
		key := k.(*int64)
		return *key%2 == 0
	})

	time.Sleep(1 * time.Second)
	cleaner.Stop()

	for i := 0; i < numMapEntries; i++ {
		*key = int64(i)
		err := m.Lookup(key, val)

		// If the entry is even, it should have been deleted
		// otherwise it should be present
		if i%2 == 0 {
			assert.NotNilf(t, err, "entry key=%d should not be present", i)
		} else {
			assert.Nil(t, err)
		}
	}
}
