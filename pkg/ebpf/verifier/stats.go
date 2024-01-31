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

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
)

// stats holds the value of a verifier statistics and a regular expression
// to parse it from the verifier log.
type stat struct {
	// `Value` must be exported to be settable
	Value int
	parse *regexp.Regexp
}

// Statistics represent that statistics exposed via
// the eBPF verifier when  LogLevelStats is enabled
type Statistics struct {
	StackDepth                 stat `json:"stack_usage" kernel:"4.15"`
	InstructionsProcessed      stat `json:"instruction_processed" kernel:"4.15"`
	InstructionsProcessedLimit stat `json:"limit" kernel:"4.15"`
	VerificationTime           stat `json:"verification_time" kernel:"5.2"`
	MaxStatesPerInstruction    stat `json:"max_states_per_insn" kernel:"5.2"`
	TotalStates                stat `json:"total_states" kernel:"5.2"`
	PeakStates                 stat `json:"peak_states" kernel:"5.2"`
}

var stackUsage = regexp.MustCompile(`stack depth\s+(?P<usage>\d+).*\n`)
var verificationTime = regexp.MustCompile(`verification time\s+(?P<time>\d+).*\n`)
var insnProcessed = regexp.MustCompile(`processed (?P<processed>\d+) insns`)
var insnLimit = regexp.MustCompile(`\(limit (?P<limit>\d+)\)`)
var maxStates = regexp.MustCompile(`max_states_per_insn (?P<max_states>\d+)`)
var totalStates = regexp.MustCompile(`total_states (?P<total_states>\d+)`)
var peakStates = regexp.MustCompile(`peak_states (?P<peak_states>\d+)`)

//go:generate go run functions.go
//go:generate go fmt programs.go

func isCOREAsset(path string) bool {
	return filepath.Base(filepath.Dir(path)) == "co-re"
}

// BuildVerifierStats accepts a list of eBPF object files and generates a
// map of all programs and their Statistics
func BuildVerifierStats(objectFiles []string) (map[string]*Statistics, map[string]struct{}, error) {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get host kernel version: %w", err)
	}
	if kversion < kernel.VersionCode(4, 15, 0) {
		return nil, nil, fmt.Errorf("Kernel %s does not expose verifier statistics", kversion)
	}

	failedToLoad := make(map[string]struct{})
	stats := make(map[string]*Statistics)
	for _, file := range objectFiles {
		if !isCOREAsset(file) {
			bc, err := os.Open(file)
			if err != nil {
				return nil, nil, fmt.Errorf("couldn't open asset: %v", err)
			}
			defer bc.Close()

			if err := generateLoadFunction(file, stats, failedToLoad)(bc, manager.Options{}); err != nil {
				return nil, nil, fmt.Errorf("failed to load non-core asset: %w", err)
			}

			continue
		}

		if err := ddebpf.LoadCOREAsset(file, generateLoadFunction(file, stats, failedToLoad)); err != nil {
			return nil, nil, fmt.Errorf("failed to load core asset: %w", err)
		}
	}

	return stats, failedToLoad, nil
}

func programKey(specName, objFileName string) string {
	return fmt.Sprintf("%s/Program__%s", objFileName, specName)
}

func generateLoadFunction(file string, stats map[string]*Statistics, failedToLoad map[string]struct{}) func(bytecode.AssetReader, manager.Options) error {
	return func(bc bytecode.AssetReader, managerOptions manager.Options) error {
		kversion, err := kernel.HostVersion()
		if err != nil {
			return fmt.Errorf("failed to get host kernel version: %w", err)
		}

		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		if err != nil {
			return fmt.Errorf("failed to load collection spec: %v", err)
		}

		// Max entry has to be > 0 for all maps
		for _, mapSpec := range collectionSpec.Maps {
			if mapSpec.MaxEntries == 0 {
				mapSpec.MaxEntries = 1
			}
		}

		// replace telemetry patch points with nops
		// r1 = r1
		newIns := asm.Mov.Reg(asm.R1, asm.R1)
		for _, p := range collectionSpec.Programs {
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
				// maximum log size accepted by the kernel:
				// https://github.com/cilium/ebpf/blob/main/prog.go#L42
				LogSize:     1073741823,
				KernelTypes: managerOptions.VerifierOptions.Programs.KernelTypes,
			},
		}

		objectFileName := strings.ReplaceAll(
			strings.Split(filepath.Base(file), ".")[0], "-", "_",
		)
		for _, progSpec := range collectionSpec.Programs {
			prog, ok := interfaceMap[fmt.Sprintf("%s_%s", progSpec.Name, objectFileName)]
			if !ok {
				return fmt.Errorf("no interface entry for program_file %s_%s", progSpec.Name, objectFileName)
			}
			err = collectionSpec.LoadAndAssign(prog, &opts)
			if err != nil {
				log.Printf("failed to load and assign ebpf.Program in file %s: %v", objectFileName, err)
				failedToLoad[programKey(progSpec.Name, objectFileName)] = struct{}{}
				continue
			}

			progPtr := reflect.ValueOf(prog)
			if progPtr.Type().Kind() != reflect.Ptr {
				return fmt.Errorf("%T is not a pointer to struct", prog)
			}

			if progPtr.IsNil() {
				return fmt.Errorf("nil pointer to %T", progPtr)
			}

			progElem := progPtr.Elem()
			if progElem.Kind() != reflect.Struct {
				return fmt.Errorf("%s is not a struct", progElem)
			}
			for i := 0; i < progElem.NumField(); i++ {
				programName := progElem.Type().Field(i).Name

				field := progElem.Field(i)
				switch field.Type() {
				case reflect.TypeOf((*ebpf.Program)(nil)):
					p := field.Interface().(*ebpf.Program)
					stat, err := unmarshalStatistics(p.VerifierLog, kversion)
					if err != nil {
						return fmt.Errorf("failed to unmarshal verifier log for program %s: %w", programName, err)
					}
					stats[fmt.Sprintf("%s/%s", objectFileName, programName)] = stat
				default:
					return fmt.Errorf("Unexpected type %T", field)
				}
			}
		}

		return nil
	}
}

type structField struct {
	reflect.StructField
	value reflect.Value
}

func unmarshalStatistics(output string, hostVersion kernel.Version) (*Statistics, error) {
	v := Statistics{
		VerificationTime:           stat{parse: verificationTime},
		StackDepth:                 stat{parse: stackUsage},
		InstructionsProcessed:      stat{parse: insnProcessed},
		InstructionsProcessedLimit: stat{parse: insnLimit},
		MaxStatesPerInstruction:    stat{parse: maxStates},
		TotalStates:                stat{parse: totalStates},
		PeakStates:                 stat{parse: peakStates},
	}

	// we want statsValue to be settable which is why we get
	// reflect.Value in this roundabout manner.
	statsValue := reflect.ValueOf(&v).Elem()
	statsType := statsValue.Type()
	for i := 0; i < statsType.NumField(); i++ {
		field := structField{statsType.Field(i), statsValue.Field(i)}
		version := field.Tag.Get("kernel")
		if version == "" {
			return nil, fmt.Errorf("field %s not tagged with kernel version", field.Name)
		}
		if hostVersion < kernel.ParseVersion(version) {
			continue
		}

		s := field.value.Interface().(stat)
		parsedValue, err := strconv.Atoi(s.parse.FindStringSubmatch(output)[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse value for field %s: %w", field.Name, err)
		}

		newStat := stat{Value: parsedValue}
		field.value.Set(reflect.ValueOf(newStat))
	}

	return &v, nil
}
