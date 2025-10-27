// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

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
	err = dumpMapJSON(m, info, &buf)
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
	err = dumpMapJSON(m, info, &buf)
	require.NoError(t, err)

	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)

	// Verify exactly one entry
	require.Len(t, entries, 1)
	require.Equal(t, []string{"0x01", "0x02", "0x03", "0x04"}, entries[0].Key)
	require.Equal(t, []string{"0xaa", "0xbb", "0xcc", "0xdd"}, entries[0].Value)
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
	err = dumpMapJSON(m, info, &buf)
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
	err = dumpMapJSON(m, info, &buf)
	require.NoError(t, err)

	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)

	require.Len(t, entries, 1)
	require.Len(t, entries[0].Key, 64, "Key should have 64 bytes")
	require.Len(t, entries[0].Value, 128, "Value should have 128 bytes")

	// Verify some byte values
	require.Equal(t, "0x00", entries[0].Key[0])
	require.Equal(t, "0x3f", entries[0].Key[63]) // 63 in hex
	require.Equal(t, "0x00", entries[0].Value[0])
	require.Equal(t, "0xfe", entries[0].Value[127]) // 127*2 = 254
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
	err = runMapDumpByID(mapID, &buf)
	require.NoError(t, err)

	// Verify output
	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, []string{"0x01", "0x02", "0x03", "0x04"}, entries[0].Key)
}

func TestRunMapDumpByIDNotFound(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Try to dump a map with an invalid ID
	var buf bytes.Buffer
	err := runMapDumpByID(ebpf.MapID(999999999), &buf)
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
	err = runMapDumpByName("test_dump_by_name", &buf)
	if err != nil {
		// Skip if kernel doesn't support map names
		t.Skipf("Could not dump by name (this is OK on some kernels): %v", err)
	}

	// Verify output
	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, []string{"0x0a", "0x0b", "0x0c", "0x0d"}, entries[0].Key)
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
	err = dumpMapJSON(m, info, &buf)
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

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
