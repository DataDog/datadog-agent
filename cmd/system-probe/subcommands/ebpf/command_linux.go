// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/cilium/ebpf"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
)

func makeEbpfCommand(globalParams *command.GlobalParams) *cobra.Command {
	ebpfCmd := &cobra.Command{
		Use:   "ebpf",
		Short: "Inspect eBPF objects",
	}
	ebpfCmd.AddCommand(makeMapCommand(globalParams))
	return ebpfCmd
}

func makeMapCommand(globalParams *command.GlobalParams) *cobra.Command {
	mapCmd := &cobra.Command{
		Use:   "map",
		Short: "Operations on eBPF maps",
	}
	mapCmd.AddCommand(makeMapListCommand(globalParams))
	mapCmd.AddCommand(makeMapDumpCommand(globalParams))
	return mapCmd
}

func makeMapListCommand(_ *command.GlobalParams) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all eBPF maps",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMapList(os.Stdout)
		},
	}
}

func runMapList(w io.Writer) error {
	var id ebpf.MapID

	for {
		var err error
		id, err = ebpf.MapGetNextID(id)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return fmt.Errorf("error enumerating maps: %w", err)
		}

		m, err := ebpf.NewMapFromID(id)
		if err != nil {
			continue
		}

		info, err := m.Info()
		m.Close()
		if err != nil {
			continue
		}

		mapID, _ := info.ID()
		fmt.Fprintf(w, "%d: %s  name %s  flags 0x%x\n",
			mapID, info.Type, info.Name, info.Flags)
		fmt.Fprintf(w, "    key %dB  value %dB  max_entries %d\n",
			info.KeySize, info.ValueSize, info.MaxEntries)
	}

	return nil
}

func makeMapDumpCommand(_ *command.GlobalParams) *cobra.Command {
	return &cobra.Command{
		Use:   "dump {id <id> | name <name>}",
		Short: "Dump contents of an eBPF map",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			specifier := args[0]
			value := args[1]

			switch specifier {
			case "id":
				mapID, err := strconv.ParseUint(value, 10, 32)
				if err != nil {
					return fmt.Errorf("invalid map id: %w", err)
				}
				return runMapDumpByID(ebpf.MapID(mapID), os.Stdout)
			case "name":
				return runMapDumpByName(value, os.Stdout)
			default:
				return fmt.Errorf("invalid specifier %q, use 'id' or 'name'", specifier)
			}
		},
	}
}

func runMapDumpByID(id ebpf.MapID, w io.Writer) error {
	m, err := ebpf.NewMapFromID(id)
	if err != nil {
		return fmt.Errorf("failed to open map: %w", err)
	}
	defer m.Close()

	info, err := m.Info()
	if err != nil {
		return err
	}

	return dumpMapJSON(m, info, w)
}

func runMapDumpByName(name string, w io.Writer) error {
	m, info, err := findMapByName(name)
	if err != nil {
		return err
	}
	defer m.Close()

	return dumpMapJSON(m, info, w)
}

func findMapByName(name string) (*ebpf.Map, *ebpf.MapInfo, error) {
	var id ebpf.MapID

	for {
		var err error
		id, err = ebpf.MapGetNextID(id)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("map %q not found", name)
		}
		if err != nil {
			return nil, nil, err
		}

		m, err := ebpf.NewMapFromID(id)
		if err != nil {
			continue
		}

		info, err := m.Info()
		if err != nil {
			m.Close()
			continue
		}

		if info.Name == name {
			return m, info, nil
		}

		m.Close()
	}
}

type mapEntry struct {
	Key   []string `json:"key"`
	Value []string `json:"value"`
}

func dumpMapJSON(m *ebpf.Map, info *ebpf.MapInfo, w io.Writer) error {
	iter := m.Iterate()
	keyBuf := make([]byte, info.KeySize)
	valueBuf := make([]byte, info.ValueSize)

	var entries []mapEntry

	for iter.Next(&keyBuf, &valueBuf) {
		// Convert bytes to array of hex strings like bpftool
		keyHex := make([]string, len(keyBuf))
		for i, b := range keyBuf {
			keyHex[i] = fmt.Sprintf("0x%02x", b)
		}

		valueHex := make([]string, len(valueBuf))
		for i, b := range valueBuf {
			valueHex[i] = fmt.Sprintf("0x%02x", b)
		}

		entries = append(entries, mapEntry{
			Key:   keyHex,
			Value: valueHex,
		})
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("iteration error: %w", err)
	}

	// Custom JSON formatting to match bpftool: compact arrays, indented objects
	fmt.Fprintf(w, "[")
	for i, entry := range entries {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, "{\n\t\"key\": ")

		// Marshal key array compactly
		keyJSON, err := json.Marshal(entry.Key)
		if err != nil {
			return fmt.Errorf("failed to marshal key: %w", err)
		}
		fmt.Fprintf(w, "%s", keyJSON)

		fmt.Fprintf(w, ",\n\t\"value\": ")

		// Marshal value array compactly
		valueJSON, err := json.Marshal(entry.Value)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
		fmt.Fprintf(w, "%s", valueJSON)

		fmt.Fprintf(w, "\n}")
	}
	fmt.Fprintf(w, "]\n")

	return nil
}
