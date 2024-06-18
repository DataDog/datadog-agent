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
	"os/exec"
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

var (
	stackUsage    = regexp.MustCompile(`stack depth\s+(?P<usage>\d+).*\n`)
	insnProcessed = regexp.MustCompile(`processed (?P<processed>\d+) insns`)
	insnLimit     = regexp.MustCompile(`\(limit (?P<limit>\d+)\)`)
	maxStates     = regexp.MustCompile(`max_states_per_insn (?P<max_states>\d+)`)
	totalStates   = regexp.MustCompile(`total_states (?P<total_states>\d+)`)
	peakStates    = regexp.MustCompile(`peak_states (?P<peak_states>\d+)`)
)

func isCOREAsset(path string) bool {
	return filepath.Base(filepath.Dir(path)) == "co-re"
}

// BuildVerifierStats accepts a list of eBPF object files and generates a
// map of all programs and their Statistics, and a map of their detailed complexity info (only filled if DetailedComplexity is true)
func BuildVerifierStats(opts *StatsOptions) (*StatsResult, map[string]struct{}, error) {
	kversion, err := kernel.HostVersion()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get host kernel version: %w", err)
	}
	if kversion < kernel.VersionCode(4, 15, 0) {
		return nil, nil, fmt.Errorf("Kernel %s does not expose verifier statistics", kversion)
	}

	failedToLoad := make(map[string]struct{})
	results := &StatsResult{
		Stats:           make(map[string]*Statistics),
		Complexity:      make(map[string]*ComplexityInfo),
		FuncsPerSection: make(map[string]map[string][]string),
	}

	for _, file := range opts.ObjectFiles {
		if !isCOREAsset(file) {
			bc, err := os.Open(file)
			if err != nil {
				return nil, nil, fmt.Errorf("couldn't open asset %s: %v", file, err)
			}
			defer bc.Close()

			if err := generateLoadFunction(file, opts, results, failedToLoad)(bc, manager.Options{}); err != nil {
				return nil, nil, fmt.Errorf("failed to load non-core asset %s: %w", file, err)
			}

			continue
		}

		if err := ddebpf.LoadCOREAsset(file, generateLoadFunction(file, opts, results, failedToLoad)); err != nil {
			return nil, nil, fmt.Errorf("failed to load core asset %s: %w", file, err)
		}
	}

	return results, failedToLoad, nil
}

func generateLoadFunction(file string, opts *StatsOptions, results *StatsResult, failedToLoad map[string]struct{}) func(bytecode.AssetReader, manager.Options) error {
	return func(bc bytecode.AssetReader, managerOptions manager.Options) error {
		kversion, err := kernel.HostVersion()
		if err != nil {
			return fmt.Errorf("failed to get host kernel version: %w", err)
		}

		log.Printf("Loading asset %s\n", file)
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

		progOpts := ebpf.ProgramOptions{
			LogLevel:    ebpf.LogLevelStats,
			LogSize:     10 * 1024 * 1024,
			KernelTypes: managerOptions.VerifierOptions.Programs.KernelTypes,
		}

		if opts.DetailedComplexity {
			// We need the full instruction-level verifier log if we want to calculate complexity
			// for each line
			progOpts.LogLevel |= ebpf.LogLevelInstruction
			progOpts.LogSize = 1073741823 // Maximum log size for the verifier
		}

		collOpts := ebpf.CollectionOptions{
			Programs: progOpts,
		}

		var sourceMap map[string]map[int]*SourceLine
		var funcsPerSect map[string][]string
		objectFileName := strings.ReplaceAll(
			strings.Split(filepath.Base(file), ".")[0], "-", "_",
		)

		if opts.DetailedComplexity {
			sourceMap, funcsPerSect, err = getSourceMap(file, collectionSpec)
			if err != nil {
				return fmt.Errorf("failed to get llvm-objdump data for %v: %w", file, err)
			}
			results.FuncsPerSection[objectFileName] = funcsPerSect
		}
		for _, progSpec := range collectionSpec.Programs {
			if len(opts.FilterPrograms) > 0 {
				found := false
				for _, filter := range opts.FilterPrograms {
					if filter.FindString(progSpec.Name) != "" {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			log.Printf("Loading program %s\n", progSpec.Name)

			prog := reflect.New(
				reflect.StructOf([]reflect.StructField{
					{
						Name: fmt.Sprintf("Func_%s", progSpec.Name),
						Type: reflect.TypeOf(&ebpf.Program{}),
						Tag:  reflect.StructTag(fmt.Sprintf(`ebpf:"%s"`, progSpec.Name)),
					},
				}),
			)
			err = collectionSpec.LoadAndAssign(prog.Elem().Addr().Interface(), &collOpts)
			if err != nil {
				log.Printf("failed to load and assign ebpf.Program in file %s: %v", objectFileName, err)
				failedToLoad[fmt.Sprintf("%s/%s", objectFileName, progSpec.Name)] = struct{}{}
				continue
			}

			if prog.Type().Kind() != reflect.Ptr {
				return fmt.Errorf("%T is not a pointer to struct", prog)
			}

			if prog.IsNil() {
				return fmt.Errorf("nil pointer to %T", prog)
			}

			progElem := prog.Elem()
			if progElem.Kind() != reflect.Struct {
				return fmt.Errorf("%s is not a struct", progElem)
			}
			for i := 0; i < progElem.NumField(); i++ {
				field := progElem.Field(i)
				switch field.Type() {
				case reflect.TypeOf((*ebpf.Program)(nil)):
					p := field.Interface().(*ebpf.Program)
					vlog := p.VerifierLog
					// All unassigned programs and maps are cleaned up by the ebpf loader: https://github.com/cilium/ebpf/blob/main/collection.go#L439
					// We only need to take care to cleanup assigned programs.
					p.Close()

					if opts.VerifierLogsDir != "" {
						logFileName := fmt.Sprintf("%s_%s.ebpfvlog", objectFileName, progSpec.Name)
						logPath := filepath.Join(opts.VerifierLogsDir, logFileName)
						if err = os.WriteFile(logPath, []byte(vlog), 0644); err != nil {
							return fmt.Errorf("failed to write verifier log to %s for program %s: %w", logPath, progSpec.Name, err)
						}
					}

					stat, err := unmarshalStatistics(vlog, kversion)
					if err != nil {
						return fmt.Errorf("failed to unmarshal verifier log for program %s: %w", progSpec.Name, err)
					}
					progName := fmt.Sprintf("%s/%s", objectFileName, progSpec.Name)
					results.Stats[progName] = stat

					if opts.DetailedComplexity {
						progSourceMap := sourceMap[progSpec.Name]
						if progSourceMap == nil {
							log.Printf("No source map found for program %s\n", progSpec.Name)
						}
						vlp := newVerifierLogParser(progSourceMap)
						if vlp == nil {
							return fmt.Errorf("failed to create verifier log parser for program %s", progSpec.Name)
						}

						compl, err := vlp.parseVerifierLog(vlog)
						if err != nil {
							return fmt.Errorf("failed to unmarshal complexity info for program %s: %w", progSpec.Name, err)
						}
						results.Complexity[progName] = compl
					}
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
		if field.Name == "Complexity" {
			continue
		}
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
