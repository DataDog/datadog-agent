// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"bytes"
	"encoding/json"
	"strings"
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

func TestDumpMapJSON(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a test eBPF map
	spec := &ebpf.MapSpec{
		Type:       ebpf.Hash,
		KeySize:    4,
		ValueSize:  8,
		MaxEntries: 10,
	}

	m, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer m.Close()

	// Put some test data
	key1 := []byte{0x01, 0x02, 0x03, 0x04}
	value1 := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x11, 0x22}
	require.NoError(t, m.Put(key1, value1))

	key2 := []byte{0x10, 0x20, 0x30, 0x40}
	value2 := []byte{0xa1, 0xb2, 0xc3, 0xd4, 0xe5, 0xf6, 0x17, 0x28}
	require.NoError(t, m.Put(key2, value2))

	// Get map info
	info, err := m.Info()
	require.NoError(t, err)

	// Dump the map to JSON
	var buf bytes.Buffer
	err = dumpMapJSON(m, info, &buf)
	require.NoError(t, err)

	// Parse the JSON output
	var entries []mapEntry
	err = json.Unmarshal(buf.Bytes(), &entries)
	require.NoError(t, err)

	// Verify we got 2 entries
	require.Len(t, entries, 2)

	// Verify byte array format
	for _, entry := range entries {
		require.Len(t, entry.Key, 4)
		require.Len(t, entry.Value, 8)

		// Each element should be a hex string like "0xXX"
		for _, k := range entry.Key {
			require.True(t, strings.HasPrefix(k, "0x"))
			require.Len(t, k, 4) // "0xXX" is 4 characters
		}
		for _, v := range entry.Value {
			require.True(t, strings.HasPrefix(v, "0x"))
			require.Len(t, v, 4)
		}
	}

	// Verify one of the entries matches our test data
	foundEntry1 := false
	for _, entry := range entries {
		if entry.Key[0] == "0x01" && entry.Key[1] == "0x02" &&
			entry.Key[2] == "0x03" && entry.Key[3] == "0x04" {
			foundEntry1 = true
			require.Equal(t, "0xaa", entry.Value[0])
			require.Equal(t, "0xbb", entry.Value[1])
			require.Equal(t, "0xcc", entry.Value[2])
			require.Equal(t, "0xdd", entry.Value[3])
			require.Equal(t, "0xee", entry.Value[4])
			require.Equal(t, "0xff", entry.Value[5])
			require.Equal(t, "0x11", entry.Value[6])
			require.Equal(t, "0x22", entry.Value[7])
		}
	}
	require.True(t, foundEntry1, "Expected to find entry with key [0x01, 0x02, 0x03, 0x04]")
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

func TestRunMapListOutput(t *testing.T) {
	require.NoError(t, rlimit.RemoveMemlock())

	// Create a test map
	spec := &ebpf.MapSpec{
		Name:       "test_list_map",
		Type:       ebpf.Hash,
		KeySize:    8,
		ValueSize:  16,
		MaxEntries: 100,
	}

	testMap, err := ebpf.NewMapWithOptions(spec, ebpf.MapOptions{})
	require.NoError(t, err)
	defer testMap.Close()

	// Run map list and capture output
	var buf bytes.Buffer
	err = runMapList(&buf)
	require.NoError(t, err)

	output := buf.String()
	require.NotEmpty(t, output)

	// Output should have lines in bpftool format
	lines := strings.Split(output, "\n")

	// Should have at least some maps (the system will have maps)
	// Each map takes 2 lines, so we should have at least 2 lines
	require.Greater(t, len(lines), 2)

	// Check format of first map entry
	// Format: "<id>: <type>  name <name>  flags 0x<flags>"
	// Second line: "    key <size>B  value <size>B  max_entries <count>"

	foundMapLine := false
	foundSizeLine := false
	for i, line := range lines {
		if strings.Contains(line, ": ") && strings.Contains(line, "name") && strings.Contains(line, "flags") {
			foundMapLine = true
			// Next line should be the size line
			if i+1 < len(lines) && strings.Contains(lines[i+1], "key") &&
				strings.Contains(lines[i+1], "value") && strings.Contains(lines[i+1], "max_entries") {
				foundSizeLine = true
				// Verify it starts with spaces (indentation)
				require.True(t, strings.HasPrefix(lines[i+1], "    "))
			}
		}
	}

	require.True(t, foundMapLine, "Expected to find at least one map line in output")
	require.True(t, foundSizeLine, "Expected to find at least one size line in output")
}

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, cmd := range parent.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	return nil
}
