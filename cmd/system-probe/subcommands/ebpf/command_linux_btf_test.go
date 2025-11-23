// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestBTFMapDumping tests that maps with BTF type information are dumped with structured data
func TestBTFMapDumping(t *testing.T) {
	// BTF was introduced in Linux kernel 4.18
	kversion, err := kernel.HostVersion()
	require.NoError(t, err)
	if minVersion := kernel.VersionCode(4, 18, 0); kversion < minVersion {
		t.Skipf("BTF not supported on kernels < %s (current: %s)", minVersion, kversion)
	}

	require.NoError(t, rlimit.RemoveMemlock())

	// Get eBPF config which has the correct BPFDir
	cfg := ddebpf.NewConfig()
	bpfDir := filepath.Join(cfg.BPFDir, "co-re")

	// Load the eBPF object file
	bc, err := bytecode.GetReader(bpfDir, "btf_test.o")
	require.NoError(t, err, "failed to load btf_test.o")
	defer bc.Close()

	// Load collection spec from the eBPF object file
	spec, err := ebpf.LoadCollectionSpecFromReader(bc)
	require.NoError(t, err, "failed to load collection spec")

	// Create the collection to instantiate the maps
	coll, err := ebpf.NewCollection(spec)
	require.NoError(t, err, "failed to create collection")
	defer coll.Close()

	// Test int_map (Hash map with u32 key, u64 value)
	t.Run("int_map", func(t *testing.T) {
		m := coll.Maps["int_map"]
		require.NotNil(t, m, "int_map not found")

		// Insert test data
		key := uint32(42)
		value := uint64(12345)
		keyBytes := make([]byte, 4)
		valueBytes := make([]byte, 8)
		binary.LittleEndian.PutUint32(keyBytes, key)
		binary.LittleEndian.PutUint64(valueBytes, value)
		err := m.Put(keyBytes, valueBytes)
		require.NoError(t, err, "failed to insert into int_map")

		// Get map info
		info, err := m.Info()
		require.NoError(t, err, "failed to get map info")

		// Dump the map
		var buf bytes.Buffer
		err = dumpMapJSON(m, info, &buf, false)
		require.NoError(t, err, "failed to dump int_map")
		output := buf.String()

		// Verify BTF output (should not contain hex format)
		require.NotContains(t, output, `"0x`, "output should not contain hex format when BTF is available")

		// Parse JSON - output is an array of mapEntry
		var entries []mapEntry
		err = json.Unmarshal(buf.Bytes(), &entries)
		require.NoError(t, err, "failed to parse JSON output")
		require.NotEmpty(t, entries, "entries should not be empty")

		// Key should be 42
		keyVal, ok := entries[0].Key.(float64)
		require.True(t, ok, "key should be a number")
		require.Equal(t, float64(42), keyVal)

		// Value should be 12345
		valueVal, ok := entries[0].Value.(float64)
		require.True(t, ok, "value should be a number")
		require.Equal(t, float64(12345), valueVal)
	})

	// Test struct_map (Hash map with struct key and value)
	t.Run("struct_map", func(t *testing.T) {
		m := coll.Maps["struct_map"]
		require.NotNil(t, m, "struct_map not found")

		// Insert test data: conn_key{netns: 1000, port: 8080, pad: 0}
		// conn_stats{packets: 100, bytes: 5000}
		key := make([]byte, 16) // 8 + 2 + 2 + 4 padding
		binary.LittleEndian.PutUint64(key[0:8], 1000)
		binary.LittleEndian.PutUint16(key[8:10], 8080)
		binary.LittleEndian.PutUint16(key[10:12], 0)

		value := make([]byte, 16) // 8 + 8
		binary.LittleEndian.PutUint64(value[0:8], 100)
		binary.LittleEndian.PutUint64(value[8:16], 5000)

		err := m.Put(key, value)
		require.NoError(t, err, "failed to insert into struct_map")

		// Get map info
		info, err := m.Info()
		require.NoError(t, err, "failed to get map info")

		// Dump the map
		var buf bytes.Buffer
		err = dumpMapJSON(m, info, &buf, false)
		require.NoError(t, err, "failed to dump struct_map")
		output := buf.String()

		// Verify BTF output
		require.NotContains(t, output, `"0x`, "output should not contain hex format when BTF is available")

		// Parse JSON - output is an array of mapEntry
		var entries []mapEntry
		err = json.Unmarshal(buf.Bytes(), &entries)
		require.NoError(t, err, "failed to parse JSON output")
		require.NotEmpty(t, entries, "entries should not be empty")

		// Key should be a struct with netns, port, pad fields
		keyStruct, ok := entries[0].Key.(map[string]interface{})
		require.True(t, ok, "key should be a struct")
		require.Equal(t, float64(1000), keyStruct["netns"])
		require.Equal(t, float64(8080), keyStruct["port"])

		// Value should be a struct with packets, bytes fields
		valueStruct, ok := entries[0].Value.(map[string]interface{})
		require.True(t, ok, "value should be a struct")
		require.Equal(t, float64(100), valueStruct["packets"])
		require.Equal(t, float64(5000), valueStruct["bytes"])
	})

	// Test array_map (Array map with u32 key, u64 value)
	t.Run("array_map", func(t *testing.T) {
		m := coll.Maps["array_map"]
		require.NotNil(t, m, "array_map not found")

		// Insert test data at index 2
		key := uint32(2)
		value := uint64(9999)
		keyBytes := make([]byte, 4)
		valueBytes := make([]byte, 8)
		binary.LittleEndian.PutUint32(keyBytes, key)
		binary.LittleEndian.PutUint64(valueBytes, value)
		err := m.Put(keyBytes, valueBytes)
		require.NoError(t, err, "failed to insert into array_map")

		// Get map info
		info, err := m.Info()
		require.NoError(t, err, "failed to get map info")

		// Dump the map
		var buf bytes.Buffer
		err = dumpMapJSON(m, info, &buf, false)
		require.NoError(t, err, "failed to dump array_map")
		output := buf.String()

		// Verify BTF output
		require.NotContains(t, output, `"0x`, "output should not contain hex format when BTF is available")

		// Parse JSON - output is an array of mapEntry
		var entries []mapEntry
		err = json.Unmarshal(buf.Bytes(), &entries)
		require.NoError(t, err, "failed to parse JSON output")
		require.Len(t, entries, 5, "array_map should have 5 entries")

		// Check entry at index 2
		require.Equal(t, float64(2), entries[2].Key)
		require.Equal(t, float64(9999), entries[2].Value)
	})

	// Test enum_map (Hash map with u32 key, enum value)
	t.Run("enum_map", func(t *testing.T) {
		m := coll.Maps["enum_map"]
		require.NotNil(t, m, "enum_map not found")

		// Insert test data with enum value STATE_CONNECTED = 1
		key := uint32(10)
		enumValue := uint32(1) // STATE_CONNECTED
		keyBytes := make([]byte, 4)
		valueBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(keyBytes, key)
		binary.LittleEndian.PutUint32(valueBytes, enumValue)
		err := m.Put(keyBytes, valueBytes)
		require.NoError(t, err, "failed to insert into enum_map")

		// Get map info
		info, err := m.Info()
		require.NoError(t, err, "failed to get map info")

		// Dump the map
		var buf bytes.Buffer
		err = dumpMapJSON(m, info, &buf, false)
		require.NoError(t, err, "failed to dump enum_map")
		output := buf.String()

		// Verify BTF output
		require.NotContains(t, output, `"0x`, "output should not contain hex format when BTF is available")

		// Parse JSON - output is an array of mapEntry
		var entries []mapEntry
		err = json.Unmarshal(buf.Bytes(), &entries)
		require.NoError(t, err, "failed to parse JSON output")
		require.NotEmpty(t, entries, "entries should not be empty")

		// Check that the value is formatted (enum can be number, string, or object)
		valueStr := entries[0].Value
		if str, ok := valueStr.(string); ok {
			// If it's a string, it should contain "STATE_CONNECTED" or "1"
			require.True(t, strings.Contains(str, "STATE_CONNECTED") || str == "1",
				"enum value should be STATE_CONNECTED or 1")
		} else if num, ok := valueStr.(float64); ok {
			// If it's a number, it should be 1
			require.Equal(t, float64(1), num)
		} else if obj, ok := valueStr.(map[string]interface{}); ok {
			// If it's an object, just verify it's not empty (BTF may format enums as objects)
			require.NotEmpty(t, obj, "enum value object should not be empty")
		} else {
			t.Fatalf("unexpected enum value type: %T", valueStr)
		}
	})

	// Test percpu_hash_map (PerCPU hash map with u32 key, u64 value)
	t.Run("percpu_hash_map", func(t *testing.T) {
		m := coll.Maps["percpu_hash_map"]
		require.NotNil(t, m, "percpu_hash_map not found")

		// Get number of CPUs
		numCPUs, err := kernel.PossibleCPUs()
		require.NoError(t, err)

		// Insert test data - per-CPU values as slice of slices
		key := uint32(5)
		values := make([][]byte, numCPUs)
		for i := 0; i < numCPUs; i++ {
			values[i] = make([]byte, 8)
			binary.LittleEndian.PutUint64(values[i], uint64(1000+i))
		}

		keyBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(keyBytes, key)
		err = m.Put(keyBytes, values)
		require.NoError(t, err, "failed to insert into percpu_hash_map")

		// Get map info
		info, err := m.Info()
		require.NoError(t, err, "failed to get map info")

		// Dump the map
		var buf bytes.Buffer
		err = dumpMapJSON(m, info, &buf, false)
		require.NoError(t, err, "failed to dump percpu_hash_map")
		output := buf.String()

		// Verify BTF output
		require.NotContains(t, output, `"0x`, "output should not contain hex format when BTF is available")

		// Parse JSON - output is an array of perCPUMapEntry
		var entries []perCPUMapEntry
		err = json.Unmarshal(buf.Bytes(), &entries)
		require.NoError(t, err, "failed to parse JSON output")
		require.NotEmpty(t, entries, "entries should not be empty")

		// Check key
		keyVal, ok := entries[0].Key.(float64)
		require.True(t, ok, "key should be a number")
		require.Equal(t, float64(5), keyVal)

		// Values should be an array of per-CPU values
		require.Len(t, entries[0].Values, numCPUs, "should have one entry per CPU")
	})

	// Test percpu_array_map (PerCPU array map with u32 key, u64 value)
	t.Run("percpu_array_map", func(t *testing.T) {
		m := coll.Maps["percpu_array_map"]
		require.NotNil(t, m, "percpu_array_map not found")

		// Get number of CPUs
		numCPUs, err := kernel.PossibleCPUs()
		require.NoError(t, err)

		// Insert test data at index 3 - per-CPU values as slice of slices
		key := uint32(3)
		values := make([][]byte, numCPUs)
		for i := 0; i < numCPUs; i++ {
			values[i] = make([]byte, 8)
			binary.LittleEndian.PutUint64(values[i], uint64(2000+i))
		}

		keyBytes := make([]byte, 4)
		binary.LittleEndian.PutUint32(keyBytes, key)
		err = m.Put(keyBytes, values)
		require.NoError(t, err, "failed to insert into percpu_array_map")

		// Get map info
		info, err := m.Info()
		require.NoError(t, err, "failed to get map info")

		// Dump the map
		var buf bytes.Buffer
		err = dumpMapJSON(m, info, &buf, false)
		require.NoError(t, err, "failed to dump percpu_array_map")
		output := buf.String()

		// Verify BTF output
		require.NotContains(t, output, `"0x`, "output should not contain hex format when BTF is available")

		// Parse JSON - output is an array of perCPUMapEntry
		var entries []perCPUMapEntry
		err = json.Unmarshal(buf.Bytes(), &entries)
		require.NoError(t, err, "failed to parse JSON output")
		require.Len(t, entries, 5, "percpu_array_map should have 5 entries")

		// Check entry at index 3
		keyVal, ok := entries[3].Key.(float64)
		require.True(t, ok, "key should be a number")
		require.Equal(t, float64(3), keyVal)

		// Values should be an array of per-CPU values
		require.Len(t, entries[3].Values, numCPUs, "should have one entry per CPU")
	})
}
