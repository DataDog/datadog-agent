// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package verifier is responsible for exposing information the verifier provides
// for any loaded eBPF program
package verifier

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

// Statistics represent that statistics exposed via
// the eBPF verifier when  LogLevelStats is enabled
type Statistics struct {
	VerificationTime           int `json:"verification_time"`
	StackDepth                 int `json:"stack_usage"`
	InstructionsProcessed      int `json:"instruction_processed"`
	InstructionsProcessedLimit int `json:"limit"`
	MaxStatesPerInstruction    int `json:"max_states_per_insn"`
	TotalStates                int `json:"total_states"`
	PeakStates                 int `json:"peak_states"`
}

var stackUsage = regexp.MustCompile(`stack depth\s+(?P<usage>\d+).*\n`)
var verificationTime = regexp.MustCompile(`verification time\s+(?P<time>\d+).*\n`)
var insnStats = regexp.MustCompile(`processed (?P<processed>\d+) insns \(limit (?P<limit>\d+)\) max_states_per_insn (?P<max_states>\d+) total_states (?P<total_states>\d+) peak_states (?P<peak_states>\d+) mark_read (?P<mark_read>\d+)`)

func objectFileBase(path string) string {
	return strings.ReplaceAll(
		strings.Split(filepath.Base(path), ".")[0], "-", "_",
	)
}

//go:generate go run functions.go ../bytecode/build/co-re
//go:generate go fmt programs.go

// BuildVerifierStats accepts a list of eBPF object files and generates a
// map of all programs and their Statistics
func BuildVerifierStats(objectFiles []string) (map[string]*Statistics, error) {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get host kernel version: %w", err)
	}
	if kversion < kernel.VersionCode(5, 2, 0) {
		return nil, fmt.Errorf("Kernel %s does not expose verifier statistics", kversion)
	}

	stats := make(map[string]*Statistics)
	for _, file := range objectFiles {
		bc, err := os.Open(file)
		if err != nil {
			return nil, fmt.Errorf("couldn't open asset: %v", err)
		}
		defer bc.Close()

		objectFileName := objectFileBase(file)
		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		if err != nil {
			return nil, fmt.Errorf("failed to load collection spec: %v", err)
		}

		for _, mapSpec := range collectionSpec.Maps {
			if mapSpec.MaxEntries == 0 {
				mapSpec.MaxEntries = 1
			}
		}

		// patch telemetry patch points
		newIns := asm.Mov.Reg(asm.R1, asm.R1)
		for _, p := range collectionSpec.Programs {
			// do constant editing of programs for helper errors post-init
			ins := p.Instructions

			// patch telemetry helper calls
			const ebpfTelemetryPatchCall = -1
			iter := ins.Iterate()
			for iter.Next() {
				ins := iter.Ins
				if !ins.IsBuiltinCall() || ins.Constant != ebpfTelemetryPatchCall {
					continue
				}
				*ins = newIns.WithMetadata(ins.Metadata)
			}
		}

		opts := ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogLevel: ebpf.LogLevelBranch | ebpf.LogLevelStats,
				LogSize:  100 * 1024 * 1024,
			},
		}
		collection, err := ebpf.NewCollectionWithOptions(collectionSpec, opts)
		if err != nil {
			log.Printf("Load collection: %v", err)
			log.Printf("Skipping object file %s.o", objectFileName)
			continue
		}

		prog := interfaceMap[objectFileName]
		err = collection.Assign(prog)
		if err != nil {
			return nil, fmt.Errorf("failed to assign ebpf.Program: %v", err)
		}

		progPtr := reflect.ValueOf(prog)
		if progPtr.Type().Kind() != reflect.Ptr {
			return nil, fmt.Errorf("%T is not a pointer to struct", prog)
		}

		if progPtr.IsNil() {
			return nil, fmt.Errorf("nil pointer to %T", progPtr)
		}

		progElem := progPtr.Elem()
		if progElem.Kind() != reflect.Struct {
			return nil, fmt.Errorf("%s is not a struct", progElem)
		}
		for i := 0; i < progElem.NumField(); i++ {
			programName := progElem.Type().Field(i).Name

			field := progElem.Field(i)

			switch field.Type() {
			case reflect.TypeOf((*ebpf.Program)(nil)):
				p := field.Interface().(*ebpf.Program)
				stat, err := unmarshalStatistics(p.VerifierLog)
				if err != nil {
					return nil, fmt.Errorf("failed to unmarshal verifier log for program %s: %w", programName, err)
				}
				stats[programName] = stat
			default:
				return nil, fmt.Errorf("Unexpected type %T", field)
			}
		}
	}

	return stats, nil
}

func unmarshalStatistics(output string) (*Statistics, error) {
	var err error
	var v Statistics

	v.StackDepth, err = strconv.Atoi(stackUsage.FindStringSubmatch(output)[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse stack usage %q: %w", output, err)
	}

	v.VerificationTime, err = strconv.Atoi(verificationTime.FindStringSubmatch(output)[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse verification time %q: %w", output, err)
	}

	reStats := insnStats.FindStringSubmatch(output)
	v.InstructionsProcessed, err = strconv.Atoi(reStats[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse instructions processed %q: %w", output, err)
	}

	v.InstructionsProcessedLimit, err = strconv.Atoi(reStats[2])
	if err != nil {
		return nil, fmt.Errorf("failed to parse instructions processed limit %q: %w", output, err)
	}

	v.MaxStatesPerInstruction, err = strconv.Atoi(reStats[3])
	if err != nil {
		return nil, fmt.Errorf("failed to parse max states per instruction %q: %w", output, err)
	}

	v.TotalStates, err = strconv.Atoi(reStats[4])
	if err != nil {
		return nil, fmt.Errorf("failed to parse total states %q: %w", output, err)
	}

	v.PeakStates, err = strconv.Atoi(reStats[5])
	if err != nil {
		return nil, fmt.Errorf("failed to parse peak states %q: %w", output, err)
	}

	return &v, nil
}
