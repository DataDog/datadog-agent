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
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	var prettyPrint bool

	cmd := &cobra.Command{
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
				return runMapDumpByID(ebpf.MapID(mapID), os.Stdout, prettyPrint)
			case "name":
				return runMapDumpByName(value, os.Stdout, prettyPrint)
			default:
				return fmt.Errorf("invalid specifier %q, use 'id' or 'name'", specifier)
			}
		},
	}

	cmd.Flags().BoolVar(&prettyPrint, "pretty", false, "pretty-print JSON output with indentation")

	return cmd
}

func runMapDumpByID(id ebpf.MapID, w io.Writer, pretty bool) error {
	m, err := ebpf.NewMapFromID(id)
	if err != nil {
		return fmt.Errorf("failed to open map: %w", err)
	}
	defer m.Close()

	info, err := m.Info()
	if err != nil {
		return err
	}

	return dumpMapJSON(m, info, w, pretty)
}

func runMapDumpByName(name string, w io.Writer, pretty bool) error {
	m, info, err := findMapByName(name)
	if err != nil {
		return err
	}
	defer m.Close()

	return dumpMapJSON(m, info, w, pretty)
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
	Key   interface{} `json:"key"`
	Value interface{} `json:"value"`
}

type perCPUValue struct {
	CPU   int         `json:"cpu"`
	Value interface{} `json:"value"`
}

type perCPUMapEntry struct {
	Key    interface{}   `json:"key"`
	Values []perCPUValue `json:"values"`
}

// isPerCPUMap checks if the map type is a PerCPU variant
func isPerCPUMap(mapType ebpf.MapType) bool {
	return mapType == ebpf.PerCPUArray ||
		mapType == ebpf.PerCPUHash ||
		mapType == ebpf.LRUCPUHash
}

// bpfMapInfo mirrors the kernel's bpf_map_info structure fields we need
type bpfMapInfo struct {
	mapType               uint32
	id                    uint32
	keySize               uint32
	valueSize             uint32
	maxEntries            uint32
	mapFlags              uint32
	name                  [16]byte
	ifindex               uint32
	btfVmlinuxValueTypeID uint32
	netnsDev              uint64
	netnsIno              uint64
	btfID                 uint32
	btfKeyTypeID          uint32
	btfValueTypeID        uint32
	btfVmlinuxIDUnused    uint32
	mapExtra              uint64
}

// getBTFTypeIDsFromSyscall directly calls BPF_OBJ_GET_INFO_BY_FD to get BTF type IDs
func getBTFTypeIDsFromSyscall(m *ebpf.Map) (btfID uint32, keyTypeID btf.TypeID, valueTypeID btf.TypeID, err error) {
	// Get raw FD from map
	fd := m.FD()

	// Prepare bpf_map_info structure
	var info bpfMapInfo
	infoLen := uint32(unsafe.Sizeof(info))

	// Call BPF_OBJ_GET_INFO_BY_FD syscall
	attr := struct {
		bpfFd   uint32
		infoLen uint32
		info    uint64
	}{
		bpfFd:   uint32(fd),
		infoLen: infoLen,
		info:    uint64(uintptr(unsafe.Pointer(&info))),
	}

	_, _, errno := unix.Syscall(
		unix.SYS_BPF,
		unix.BPF_OBJ_GET_INFO_BY_FD,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
	)

	if errno != 0 {
		return 0, 0, 0, fmt.Errorf("BPF_OBJ_GET_INFO_BY_FD failed: %v", errno)
	}

	if info.btfID == 0 {
		return 0, 0, 0, fmt.Errorf("map has no BTF information")
	}

	return info.btfID, btf.TypeID(info.btfKeyTypeID), btf.TypeID(info.btfValueTypeID), nil
}

// getBTFInfoForMap retrieves BTF spec and key/value types for a map
func getBTFInfoForMap(m *ebpf.Map) (spec *btf.Spec, keyType, valueType btf.Type, err error) {
	// Get BTF ID and type IDs from syscall
	btfID, keyTypeID, valueTypeID, err := getBTFTypeIDsFromSyscall(m)
	if err != nil {
		return nil, nil, nil, err
	}

	// Load BTF handle from kernel
	handle, err := btf.NewHandleFromID(btf.ID(btfID))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load BTF handle: %w", err)
	}
	defer handle.Close()

	// Get BTF spec
	spec, err = handle.Spec(nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get BTF spec: %w", err)
	}

	// Resolve key and value types
	keyType, err = spec.TypeByID(keyTypeID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve key type: %w", err)
	}

	valueType, err = spec.TypeByID(valueTypeID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve value type: %w", err)
	}

	return spec, keyType, valueType, nil
}

func dumpMapJSON(m *ebpf.Map, info *ebpf.MapInfo, w io.Writer, pretty bool) error {
	// Try to get BTF info
	spec, keyType, valueType, err := getBTFInfoForMap(m)
	useBTF := (err == nil)

	var dumper *BTFDumper
	if useBTF {
		dumper = NewBTFDumper(spec)
		log.Debugf("Using BTF formatting for map %s", info.Name)
	} else {
		log.Debugf("BTF unavailable for map %s: %v, using hex format", info.Name, err)
	}

	// Detect if this is a PerCPU map type
	isPerCPU := isPerCPUMap(info.Type)

	// Allocate buffers
	keyBuf := make([]byte, info.KeySize)

	iter := m.Iterate()

	if isPerCPU {
		return dumpPerCPUMapJSON(iter, keyBuf, info.ValueSize, useBTF, dumper, keyType, valueType, w, pretty)
	}

	valueBuf := make([]byte, info.ValueSize)
	return dumpRegularMapJSON(iter, keyBuf, valueBuf, useBTF, dumper, keyType, valueType, w, pretty)
}

// writePrettyJSON formats and writes the entries as pretty-printed JSON
func writePrettyJSON(w io.Writer, entries interface{}) error {
	output, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal entries: %w", err)
	}
	fmt.Fprintf(w, "%s\n", output)
	return nil
}

func dumpRegularMapJSON(iter *ebpf.MapIterator, keyBuf, valueBuf []byte, useBTF bool, dumper *BTFDumper, keyType, valueType btf.Type, w io.Writer, pretty bool) error {
	var entries []mapEntry

	for iter.Next(&keyBuf, &valueBuf) {
		entry := mapEntry{}

		if useBTF {
			// Try to use BTF formatter for key
			formattedKey, err := dumper.DumpValue(keyBuf, keyType)
			if err != nil {
				log.Warnf("BTF format key failed: %v, using hex", err)
				entry.Key = bytesToHexArray(keyBuf)
			} else {
				entry.Key = formattedKey
			}

			// Try to use BTF formatter for value
			formattedValue, err := dumper.DumpValue(valueBuf, valueType)
			if err != nil {
				log.Warnf("BTF format value failed: %v, using hex", err)
				entry.Value = bytesToHexArray(valueBuf)
			} else {
				entry.Value = formattedValue
			}
		} else {
			// Fallback to hex format
			entry.Key = bytesToHexArray(keyBuf)
			entry.Value = bytesToHexArray(valueBuf)
		}

		entries = append(entries, entry)
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("iteration error: %w", err)
	}

	// Use pretty-printing if requested
	if pretty {
		return writePrettyJSON(w, entries)
	}

	// Custom JSON formatting to match bpftool: compact arrays, indented objects
	fmt.Fprintf(w, "[")
	for i, entry := range entries {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, "{\n\t\"key\": ")

		// Marshal key (can be BTF formatted or hex)
		keyJSON, err := json.Marshal(entry.Key)
		if err != nil {
			return fmt.Errorf("failed to marshal key: %w", err)
		}
		fmt.Fprintf(w, "%s", keyJSON)

		fmt.Fprintf(w, ",\n\t\"value\": ")

		// Marshal value (can be BTF formatted or hex)
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

func dumpPerCPUMapJSON(iter *ebpf.MapIterator, keyBuf []byte, valueSize uint32, useBTF bool, dumper *BTFDumper, keyType, valueType btf.Type, w io.Writer, pretty bool) error {
	numCPUs, err := kernel.PossibleCPUs()
	if err != nil {
		return fmt.Errorf("failed to get number of CPUs: %w", err)
	}

	// For PerCPU maps, Next() expects a slice of byte slices (one per CPU)
	valueBuf := make([][]byte, numCPUs)
	for i := range valueBuf {
		valueBuf[i] = make([]byte, valueSize)
	}

	var entries []perCPUMapEntry

	for iter.Next(&keyBuf, valueBuf) {
		entry := perCPUMapEntry{}

		// Format key with BTF if available
		if useBTF {
			formattedKey, err := dumper.DumpValue(keyBuf, keyType)
			if err != nil {
				log.Warnf("BTF format key failed: %v, using hex", err)
				entry.Key = bytesToHexArray(keyBuf)
			} else {
				entry.Key = formattedKey
			}
		} else {
			entry.Key = bytesToHexArray(keyBuf)
		}

		// Format per-CPU values
		perCPUValues := make([]perCPUValue, numCPUs)
		for cpu := 0; cpu < numCPUs; cpu++ {
			if useBTF {
				formattedValue, err := dumper.DumpValue(valueBuf[cpu], valueType)
				if err != nil {
					log.Warnf("BTF format value (CPU %d) failed: %v, using hex", cpu, err)
					perCPUValues[cpu] = perCPUValue{
						CPU:   cpu,
						Value: bytesToHexArray(valueBuf[cpu]),
					}
				} else {
					perCPUValues[cpu] = perCPUValue{
						CPU:   cpu,
						Value: formattedValue,
					}
				}
			} else {
				perCPUValues[cpu] = perCPUValue{
					CPU:   cpu,
					Value: bytesToHexArray(valueBuf[cpu]),
				}
			}
		}
		entry.Values = perCPUValues

		entries = append(entries, entry)
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("iteration error: %w", err)
	}

	// Use pretty-printing if requested
	if pretty {
		return writePrettyJSON(w, entries)
	}

	// Custom JSON formatting to match bpftool
	fmt.Fprintf(w, "[")
	for i, entry := range entries {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, "{\n\t\"key\": ")

		// Marshal key (can be BTF formatted or hex)
		keyJSON, err := json.Marshal(entry.Key)
		if err != nil {
			return fmt.Errorf("failed to marshal key: %w", err)
		}
		fmt.Fprintf(w, "%s", keyJSON)

		fmt.Fprintf(w, ",\n\t\"values\": [")

		// Output per-CPU values
		for j, cpuVal := range entry.Values {
			if j > 0 {
				fmt.Fprintf(w, ",")
			}
			fmt.Fprintf(w, "\n\t\t{\"cpu\": %d,\"value\": ", cpuVal.CPU)

			valueJSON, err := json.Marshal(cpuVal.Value)
			if err != nil {
				return fmt.Errorf("failed to marshal value: %w", err)
			}
			fmt.Fprintf(w, "%s}", valueJSON)
		}

		fmt.Fprintf(w, "\n\t]\n}")
	}
	fmt.Fprintf(w, "]\n")

	return nil
}
