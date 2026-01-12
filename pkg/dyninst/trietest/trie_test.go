// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package trietest

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

var maxOps = flag.Int(
	"max-ops",
	intFromEnvDefault("TRIE_MAX_OPS", 10_000),
	"max operations (set via TRIE_MAX_OPS)",
)

func TestTrieVsHashSetProperties(t *testing.T) {
	t.Run("properties", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			testProperties(t, generateOps(t))
		})
	})
	t.Run("full", func(t *testing.T) {
		ops := make([]operation, 0, 2*int(maxSize)+1)
		for i, n := 0, cap(ops); i < n; i++ {
			ops = append(ops, operation{
				op:  opInsert,
				key: key{addr: uint64(i / 2)},
			})
		}
		ts, hs := testProperties(t, ops)
		require.Equal(t, int(maxSize), ts.len(), "Trie len mismatch")
		require.Equal(t, int(maxSize), hs.len(), "HashSet len mismatch")

		ts.clear()
		require.Equalf(t, ts.len(), 0, "Len mismatch after clear")
		hs.clear()
		require.Equalf(t, hs.len(), 0, "Len mismatch after clear")
	})
}

// Property: Trie and HashSet should behave identically for any sequence of
// operations.
func generateOps(t *rapid.T) []operation {
	// Create a smaller pool of keys to increase collision probability.
	keyPoolSize := rapid.IntRange(*maxOps/5, *maxOps).Draw(t, "keyPoolSize")
	keyPool := rapid.SliceOfN(keyGenerator(), keyPoolSize, keyPoolSize).
		Draw(t, "keyPool")

	// Generate operations using keys from the pool.
	operations := rapid.SliceOfN(rapid.Custom(func(t *rapid.T) operation {
		return operation{
			op:  operationType(rapid.IntRange(0, 1).Draw(t, "op")),
			key: rapid.SampledFrom(keyPool).Draw(t, "key"),
		}
	}), *maxOps/5, *maxOps).Draw(t, "operations")
	operations = append(operations, operation{op: opClear})
	rand.Shuffle(len(operations), func(i, j int) {
		operations[i], operations[j] = operations[j], operations[i]
	})

	return operations
}

func testProperties(t require.TestingT, operations []operation) (*trieSet, *hashSet) {
	trie := newTrieSet()
	hashSet := newHashSet()

	// Apply operations and verify consistency.
	for i, op := range operations {
		switch op.op {
		case opInsert:
			trieInserted, trieFull := trie.insert(op.key)
			hashSetInserted, hashSetFull := hashSet.insert(op.key)

			require.Equalf(t, hashSetInserted, trieInserted,
				"Insert mismatch at step %d on %s", i, op)
			require.Equalf(t, hashSetFull, trieFull,
				"Full mismatch at step %d on %s", i, op)
			require.Equalf(t, hashSet.len(), trie.len(),
				"Len mismatch at step %d on %s", i, op)

		case opClear:
			trie.clear()
			hashSet.clear()
			require.Equalf(t, hashSet.len(), trie.len(),
				"Len mismatch at step %d on %s", i, op)

		case opContains:
			trieContains := trie.contains(op.key)
			hashSetContains := hashSet.contains(op.key)

			require.Equalf(t, hashSetContains, trieContains,
				"Contains mismatch at step %d on %s", i, op)
		}
	}

	// Final consistency check: verify all unique keys behave the same way.
	allKeys := make(map[key]struct{})
	for _, op := range operations {
		allKeys[op.key] = struct{}{}
	}

	for key := range allKeys {
		trieContains := trie.contains(key)
		hashSetContains := hashSet.contains(key)
		require.Equalf(t, hashSetContains, trieContains,
			"Final consistency check failed for key %+v", key)
	}

	return trie, hashSet
}

type operationType int

const (
	opInsert operationType = iota
	opContains
	opClear
)

type operation struct {
	op  operationType
	key key
}

func (t operationType) String() string {
	switch t {
	case opInsert:
		return "Insert"
	case opContains:
		return "Contains"
	case opClear:
		return "Clear"
	default:
		return fmt.Sprintf("operationType(%d)", t)
	}
}

func (op operation) String() string {
	return fmt.Sprintf("%s(Key{Addr: %#x, TypeID: %#x})",
		op.op, op.key.addr, op.key.typeID)
}

func keyGenerator() *rapid.Generator[key] {
	return rapid.Custom(func(t *rapid.T) key {
		return key{
			addr:   rapid.Uint64().Draw(t, "addr"),
			typeID: rapid.Uint32().Draw(t, "typeID"),
		}
	})
}

func intFromEnvDefault(env string, def int) int {
	if val := os.Getenv(env); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return def
}
