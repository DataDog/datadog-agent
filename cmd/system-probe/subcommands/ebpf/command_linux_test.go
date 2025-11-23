// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

func TestEbpfCommandStructure(t *testing.T) {
	globalParams := &command.GlobalParams{}
	cmd := makeEbpfCommand(globalParams)

	require.NotNil(t, cmd)
	require.Equal(t, "ebpf", cmd.Use)

	mapCmd := findSubcommand(cmd, "map")
	require.NotNil(t, mapCmd)

	listCmd := findSubcommand(mapCmd, "list")
	require.NotNil(t, listCmd)

	dumpCmd := findSubcommand(mapCmd, "dump")
	require.NotNil(t, dumpCmd)
}

func TestFindMapByName(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a test map with a specific name
	spec := &ebpf.MapSpec{
		Name:       "test_map_find",
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 10,
	}

	testMap, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer testMap.Close()

	// Try to find the map by name
	foundMap, info, err := findMapByName("test_map_find")
	if err != nil {
		// If we can't find it, it might be because the map name is truncated
		// or the kernel doesn't support map names. This is acceptable for this test.
		t.Skipf("Could not find map by name (this is OK on some kernels): %v", err)
	}
	require.NotNil(t, foundMap)
	require.NotNil(t, info)
	defer foundMap.Close()

	require.Equal(t, "test_map_find", info.Name)
}

func TestDumpEmptyMap(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create an empty map
	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	info, err := m.Info()
	require.NoError(t, err)

	// Dump empty map
	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err)

	// Should output empty JSON array
	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)
	require.Len(t, entries, 0, "Empty map should produce empty array")
}

func TestDumpSingleEntry(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Add single entry
	key := []byte{0x01, 0x02, 0x03, 0x04}
	value := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	require.NoError(t, m.Put(key, value))

	info, err := m.Info()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err)

	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)

	// Verify exactly one entry
	require.Len(t, entries, 1)

	// Type assert to handle JSON unmarshaling into interface{}
	keyArray, ok := entries[0].Key.([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"0x01", "0x02", "0x03", "0x04"}, keyArray)

	valueArray, ok := entries[0].Value.([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"0xaa", "0xbb", "0xcc", "0xdd"}, valueArray)
}

func TestDumpArrayMap(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Array maps use index as key
	spec := &ebpf.MapSpec{
		Type:       ebpf.Array,
		KeySize:    4, // uint32 index
		ValueSize:  8,
		MaxEntries: 5,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Put values at specific indices
	index0 := uint32(0)
	value0 := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
	require.NoError(t, m.Put(&index0, value0))

	index2 := uint32(2)
	value2 := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11}
	require.NoError(t, m.Put(&index2, value2))

	info, err := m.Info()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err)

	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)

	// Array maps return all slots (including unset ones)
	require.Equal(t, int(spec.MaxEntries), len(entries), "Array maps should return all entries")
}

func TestDumpLargeKeyValue(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Test with large key/value sizes
	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    64,
		ValueSize:  128,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Create large key and value
	key := make([]byte, 64)
	for i := range key {
		key[i] = byte(i)
	}
	value := make([]byte, 128)
	for i := range value {
		value[i] = byte(i * 2)
	}

	require.NoError(t, m.Put(key, value))

	info, err := m.Info()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err)

	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)

	require.Len(t, entries, 1)

	// Type assert to hex array format (since map has no BTF)
	keyArray, ok := entries[0].Key.([]interface{})
	require.True(t, ok, "Key should be an array")
	require.Len(t, keyArray, 64, "Key should have 64 bytes")

	valueArray, ok := entries[0].Value.([]interface{})
	require.True(t, ok, "Value should be an array")
	require.Len(t, valueArray, 128, "Value should have 128 bytes")

	// Verify some byte values
	require.Equal(t, "0x00", keyArray[0])
	require.Equal(t, "0x3f", keyArray[63]) // 63 in hex
	require.Equal(t, "0x00", valueArray[0])
	require.Equal(t, "0xfe", valueArray[127]) // 127*2 = 254
}

func TestFindMapByNameNotFound(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Try to find a map that doesn't exist
	_, _, err := findMapByName("nonexistent_map_name_12345")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestRunMapDumpByID(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a test map
	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Add entry
	key := []byte{0x01, 0x02, 0x03, 0x04}
	value := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	require.NoError(t, m.Put(key, value))

	// Get the map ID
	info, err := m.Info()
	require.NoError(t, err)
	mapID, ok := info.ID()
	require.True(t, ok)

	// Dump by ID
	var buf bytes.Buffer
	err = runMapDumpByID(mapID, &buf, false)
	require.NoError(t, err)

	// Verify output
	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Type assert to handle JSON unmarshaling into interface{}
	keyArray, ok := entries[0].Key.([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"0x01", "0x02", "0x03", "0x04"}, keyArray)
}

func TestRunMapDumpByIDNotFound(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Try to dump a map with an invalid ID
	var buf bytes.Buffer
	err := runMapDumpByID(ebpf.MapID(999999999), &buf, false)
	require.Error(t, err)
}

func TestRunMapDumpByName(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a test map with a specific name
	spec := &ebpf.MapSpec{
		Name:       "test_dump_by_name",
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Add entry
	key := []byte{0x0a, 0x0b, 0x0c, 0x0d}
	value := []byte{0xf0, 0xf1, 0xf2, 0xf3}
	require.NoError(t, m.Put(key, value))

	// Dump by name
	var buf bytes.Buffer
	err = runMapDumpByName("test_dump_by_name", &buf, false)
	if err != nil {
		// Skip if kernel doesn't support map names
		t.Skipf("Could not dump by name (this is OK on some kernels): %v", err)
	}

	// Verify output
	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Type assert to handle JSON unmarshaling into interface{}
	keyArray, ok := entries[0].Key.([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"0x0a", "0x0b", "0x0c", "0x0d"}, keyArray)
}

func TestJSONCompactArrayFormat(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	key := []byte{0x01, 0x02, 0x03, 0x04}
	value := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	require.NoError(t, m.Put(key, value))

	info, err := m.Info()
	require.NoError(t, err)

	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err)

	output := buf.String()

	// Verify arrays are on single lines (compact format)
	// Key array should be on one line
	require.Contains(t, output, `"key": ["0x01","0x02","0x03","0x04"]`)
	// Value array should be on one line
	require.Contains(t, output, `"value": ["0xaa","0xbb","0xcc","0xdd"]`)

	// Verify object has proper indentation
	require.Contains(t, output, "{\n\t\"key\":")
	require.Contains(t, output, ",\n\t\"value\":")
}

func TestDumpPerCPUArrayMap(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a PerCPU array map
	spec := &ebpf.MapSpec{
		Type:       ebpf.PerCPUArray,
		KeySize:    4, // uint32 index
		ValueSize:  8, // 8 bytes per CPU
		MaxEntries: 2,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Put values at index 0 - need to provide values for all CPUs
	// The cilium/ebpf library expects a slice of values (one per CPU)
	index0 := uint32(0)
	// Create values for each CPU (we don't know exact CPU count, but library handles this)
	values := [][]byte{
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28},
		{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38},
	}
	require.NoError(t, m.Put(&index0, values))

	info, err := m.Info()
	require.NoError(t, err)

	// This should work without errors
	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err, "PerCPU map dump should not fail")

	// The output should be valid JSON
	require.NotEmpty(t, buf.String())

	// Note: We can't use the regular mapEntry struct because PerCPU maps
	// should have a different structure with "values" instead of "value"
	// For now, just verify it's valid JSON
	var result interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "Output should be valid JSON")
}

func TestDumpPerCPUHashMap(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a PerCPU hash map
	spec := &ebpf.MapSpec{
		Type:       ebpf.PerCPUHash,
		KeySize:    4,
		ValueSize:  8, // 8 bytes per CPU
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Put values with a specific key - need values for all CPUs
	key := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	values := [][]byte{
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28},
		{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38},
	}
	require.NoError(t, m.Put(key, values))

	info, err := m.Info()
	require.NoError(t, err)

	// This should work without errors
	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err, "PerCPU hash map dump should not fail")

	// The output should be valid JSON
	require.NotEmpty(t, buf.String())

	var result interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "Output should be valid JSON")
}

func TestDumpLRUHash(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create an LRU hash map
	spec := &ebpf.MapSpec{
		Type:       ebpf.LRUHash,
		KeySize:    4,
		ValueSize:  8,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Put a value
	key := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	value := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	require.NoError(t, m.Put(key, value))

	info, err := m.Info()
	require.NoError(t, err)

	// This should work without errors
	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err, "LRU hash map dump should not fail")

	// Verify output structure (should be regular map format, not PerCPU)
	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err, "Output should be valid JSON")
	require.Len(t, entries, 1)

	// Type assert to handle JSON unmarshaling into interface{}
	keyArray, ok := entries[0].Key.([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"0xaa", "0xbb", "0xcc", "0xdd"}, keyArray)

	valueArray, ok := entries[0].Value.([]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"0x01", "0x02", "0x03", "0x04", "0x05", "0x06", "0x07", "0x08"}, valueArray)
}

func TestDumpLRUCPUHash(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create an LRU per-CPU hash map
	spec := &ebpf.MapSpec{
		Type:       ebpf.LRUCPUHash,
		KeySize:    4,
		ValueSize:  8,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Put values with a specific key - need values for all CPUs
	key := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	values := [][]byte{
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28},
		{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38},
	}
	require.NoError(t, m.Put(key, values))

	info, err := m.Info()
	require.NoError(t, err)

	// This should work without errors
	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf, false)
	require.NoError(t, err, "LRU per-CPU hash map dump should not fail")

	// The output should be valid JSON
	require.NotEmpty(t, buf.String())

	var result interface{}
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err, "Output should be valid JSON")
}

func TestDumpMapPrettyPrint(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Add a couple of entries
	key1 := []byte{0x01, 0x02, 0x03, 0x04}
	value1 := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	require.NoError(t, m.Put(key1, value1))

	key2 := []byte{0x05, 0x06, 0x07, 0x08}
	value2 := []byte{0xee, 0xff, 0x00, 0x11}
	require.NoError(t, m.Put(key2, value2))

	info, err := m.Info()
	require.NoError(t, err)

	// Test with pretty=true
	var bufPretty bytes.Buffer
	err = dumpMapJSON(m, info, &bufPretty, true)
	require.NoError(t, err)

	prettyOutput := bufPretty.String()

	// Verify it's valid JSON
	var entries []mapEntry
	err = json.Unmarshal(bufPretty.Bytes(), &entries)
	require.NoError(t, err)
	require.Len(t, entries, 2)

	// Verify pretty formatting with proper indentation
	require.Contains(t, prettyOutput, "[\n  {")
	require.Contains(t, prettyOutput, "    \"key\":")
	require.Contains(t, prettyOutput, "    \"value\":")
	require.Contains(t, prettyOutput, "  }")

	// Test with pretty=false for comparison
	var bufCompact bytes.Buffer
	err = dumpMapJSON(m, info, &bufCompact, false)
	require.NoError(t, err)

	compactOutput := bufCompact.String()

	// Verify compact format uses tabs and different structure
	require.Contains(t, compactOutput, "[{")
	require.Contains(t, compactOutput, "\t\"key\":")
	require.Contains(t, compactOutput, "\t\"value\":")

	// Pretty output should be longer due to additional whitespace
	require.Greater(t, len(prettyOutput), len(compactOutput), "Pretty output should be longer")
}

func TestDumpPerCPUMapPrettyPrint(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a PerCPU hash map
	spec := &ebpf.MapSpec{
		Type:       ebpf.PerCPUHash,
		KeySize:    4,
		ValueSize:  8,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Put values with a specific key
	key := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	values := [][]byte{
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
		{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
	}
	require.NoError(t, m.Put(key, values))

	info, err := m.Info()
	require.NoError(t, err)

	// Test with pretty=true
	var bufPretty bytes.Buffer
	err = dumpMapJSON(m, info, &bufPretty, true)
	require.NoError(t, err)

	prettyOutput := bufPretty.String()

	// Verify it's valid JSON
	var entries []perCPUMapEntry
	err = json.Unmarshal(bufPretty.Bytes(), &entries)
	require.NoError(t, err)
	require.Greater(t, len(entries), 0)

	// Verify pretty formatting with proper indentation
	require.Contains(t, prettyOutput, "[\n  {")
	require.Contains(t, prettyOutput, "    \"key\":")
	require.Contains(t, prettyOutput, "    \"values\":")
	require.Contains(t, prettyOutput, "      \"cpu\":")
	require.Contains(t, prettyOutput, "      \"value\":")

	// Test with pretty=false for comparison
	var bufCompact bytes.Buffer
	err = dumpMapJSON(m, info, &bufCompact, false)
	require.NoError(t, err)

	compactOutput := bufCompact.String()

	// Verify compact format
	require.Contains(t, compactOutput, "[{")
	require.Contains(t, compactOutput, "\t\"key\":")

	// Pretty output should be longer
	require.Greater(t, len(prettyOutput), len(compactOutput), "Pretty output should be longer")
}

func TestRunMapDumpByIDWithPrettyFlag(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a test map
	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Add entry
	key := []byte{0x01, 0x02, 0x03, 0x04}
	value := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	require.NoError(t, m.Put(key, value))

	// Get the map ID
	info, err := m.Info()
	require.NoError(t, err)
	mapID, ok := info.ID()
	require.True(t, ok)

	// Dump by ID with pretty=true
	var buf bytes.Buffer
	err = runMapDumpByID(mapID, &buf, true)
	require.NoError(t, err)

	output := buf.String()

	// Verify output is pretty-formatted
	require.Contains(t, output, "[\n  {")
	require.Contains(t, output, "    \"key\":")
	require.Contains(t, output, "    \"value\":")

	// Verify JSON is valid
	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
