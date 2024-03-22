// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"os"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func TestMain(m *testing.M) {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "warn"
	}
	log.SetupLogger(seelog.Default, logLevel)
	os.Exit(m.Run())
}

type cleanerSignature func(now int64, k int64, v int64) bool

func TestMapCleaner(t *testing.T) {
	tests := []struct {
		name           string
		cleanerFactory func(*ebpf.Map) cleanerSignature
	}{
		{
			name: "sanity",
			cleanerFactory: func(*ebpf.Map) cleanerSignature {
				return func(now int64, k int64, v int64) bool {
					return k%2 == 0
				}
			},
		},
		{
			name: "key is missing",
			cleanerFactory: func(e *ebpf.Map) cleanerSignature {
				return func(now int64, k int64, v int64) bool {
					// Delete a random key.
					if k == 4 {
						e.Delete(&k)
					}
					return k%2 == 0
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const numMapEntries = 100
			const cleanerInterval = 100 * time.Millisecond

			var (
				key = new(int64)
				val = new(int64)
			)

			require.NoError(t, rlimit.RemoveMemlock())

			m, err := ebpf.NewMap(&ebpf.MapSpec{
				Type:       ebpf.Hash,
				KeySize:    8,
				ValueSize:  8,
				MaxEntries: numMapEntries,
			})
			require.NoError(t, err)

			cleaner, err := NewMapCleaner[int64, int64](m, 10)
			require.NoError(t, err)
			for i := 0; i < numMapEntries; i++ {
				*key = int64(i)
				err := m.Put(key, val)
				assert.Nilf(t, err, "can't put key=%d: %s", i, err)
			}

			// Clean all the even entries
			cleaner.Clean(cleanerInterval, nil, nil, func(now int64, k int64, v int64) bool {
				return k%2 == 0
			})

			// Giving some time to the cleaner to do its job.
			time.Sleep(3 * cleanerInterval)
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
		})
	}
}

func benchmarkBatchCleaner(b *testing.B, numMapEntries, batchSize uint32) {
	var (
		key = new(int64)
		val = new(int64)
	)

	require.NoError(b, rlimit.RemoveMemlock())

	m, err := ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    8,
		ValueSize:  8,
		MaxEntries: numMapEntries,
	})
	require.NoError(b, err)

	cleaner, err := NewMapCleaner[int64, int64](m, batchSize)
	require.NoError(b, err)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for i := uint32(0); i < numMapEntries; i++ {
			*key = int64(i)
			err := m.Put(key, val)
			assert.Nilf(b, err, "can't put key=%d: %s", i, err)
		}

		// Clean all the even entries
		if batchSize == 0 {
			cleaner.cleanWithoutBatches(0, func(now int64, k int64, v int64) bool {
				return k%2 == 0
			})
		} else {
			cleaner.cleanWithBatches(0, func(now int64, k int64, v int64) bool {
				return k%2 == 0
			})
		}
		for i := uint32(0); i < numMapEntries; i++ {
			*key = int64(i)
			err := m.Lookup(key, val)

			// If the entry is even, it should have been deleted
			// otherwise it should be present
			if i%2 == 0 {
				assert.NotNilf(b, err, "entry key=%d should not be present", i)
			} else {
				assert.Nil(b, err)
			}
		}
	}
}

func BenchmarkBatchCleaner1000Entries10PerBatch(b *testing.B) {
	benchmarkBatchCleaner(b, 1000, 10)
}

func BenchmarkBatchCleaner1000Entries100PerBatch(b *testing.B) {
	benchmarkBatchCleaner(b, 1000, 100)
}

func BenchmarkBatchCleaner10000Entries100PerBatch(b *testing.B) {
	benchmarkBatchCleaner(b, 10000, 100)
}

func BenchmarkBatchCleaner10000Entries1000PerBatch(b *testing.B) {
	benchmarkBatchCleaner(b, 10000, 1000)
}

func BenchmarkBatchCleaner100000Entries100PerBatch(b *testing.B) {
	benchmarkBatchCleaner(b, 100000, 100)
}

func BenchmarkBatchCleaner100000Entries1000PerBatch(b *testing.B) {
	benchmarkBatchCleaner(b, 100000, 1000)
}

func BenchmarkCleaner1000Entries(b *testing.B) {
	benchmarkBatchCleaner(b, 1000, 0)
}

func BenchmarkCleaner10000Entries(b *testing.B) {
	benchmarkBatchCleaner(b, 10000, 0)
}

func BenchmarkCleaner100000Entries(b *testing.B) {
	benchmarkBatchCleaner(b, 100000, 0)
}
