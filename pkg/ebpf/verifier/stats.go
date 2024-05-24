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
	"math"
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
	stackUsage          = regexp.MustCompile(`stack depth\s+(?P<usage>\d+).*\n`)
	insnProcessed       = regexp.MustCompile(`processed (?P<processed>\d+) insns`)
	insnLimit           = regexp.MustCompile(`\(limit (?P<limit>\d+)\)`)
	maxStates           = regexp.MustCompile(`max_states_per_insn (?P<max_states>\d+)`)
	totalStates         = regexp.MustCompile(`total_states (?P<total_states>\d+)`)
	peakStates          = regexp.MustCompile(`peak_states (?P<peak_states>\d+)`)
	insnRegex           = regexp.MustCompile(`^([0-9]+): \([0-9a-f]+\) (.*)`)
	regStateRegex       = regexp.MustCompile(`^([0-9]+): (R[0-9]+.*)`)
	singleRegStateRegex = regexp.MustCompile(`R([0-9]+)(_[^=]+)?=([^ ]+)`)
	regInfoRegex        = regexp.MustCompile(`^([a-z_]+)(P)?(-?[0-9]+|\((.*)\))`)
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

func getSourceMap(file string) (map[string]map[int]*SourceLine, map[string][]string, error) {
	// call llvm-objdump to get the source map in the shell
	// We cannot use the go DWARF library because it doesn't support certain features
	// (replications) for eBPF programs.
	cmd := exec.Command("llvm-objdump", "-Sl", file)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to run llvm-objdump on %s: %w", file, err)
	}

	sourceMap := make(map[string]map[int]*SourceLine)
	funcsPerSection := make(map[string][]string)
	lines := strings.Split(string(out), "\n")
	nextLineInfo := ""
	currLineInfo, currLine := "", ""
	currSect, currFunc := "", ""

	sectionRegex := regexp.MustCompile("Disassembly of section (.*):")
	functionRegex := regexp.MustCompile("^[0-9a-fA-F]{16} <([a-zA-Z_][a-zA-Z0-9_]+)>:$")
	lineInfoRegex := regexp.MustCompile("^; [^:]+:[0-9]+")
	functionJustStarted := false
	insnOffset := 0

	// Very ad-hoc parsing but enough for our purposes
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		// With -l, llvm-objdump will print the source line info
		// in two lines starting with ;. The first is the file and line number,
		// the second is the source line itself.
		// So we keep track of the last two things we found that started with ";"
		// Sometimes we can get a function entry point for an assembly line without source information,
		// so we need to discard that. We only save the source information if the first line is of the form
		// "; <file>:<line>" and the second line is the actual source line.
		// Note that a single code line might translate to multiple assembly instructions, so we do
		// this once and keep the state for all assembly lines following.
		if line[0] == ';' {
			if lineInfoRegex.MatchString(line) {
				nextLineInfo = line
			} else if nextLineInfo != "" {
				currLineInfo = strings.TrimPrefix(nextLineInfo, "; ")
				currLine = strings.TrimPrefix(line, "; ")
				nextLineInfo = ""
			}
			continue
		}
		nextLineInfo = "" // Reset the next line info if we don't have a source line

		// Check for section headers
		sectionMatch := sectionRegex.FindStringSubmatch(line)
		if len(sectionMatch) >= 2 {
			currSect = strings.ReplaceAll(sectionMatch[1], "/", "__") // match naming convention
			log.Printf("Found section %s\n", currSect)
			continue
		}

		// Check for function names
		functionMatch := functionRegex.FindStringSubmatch(line)
		if len(functionMatch) >= 2 && !strings.HasPrefix(functionMatch[1], "LBB") { // Ignore block labels
			currFunc = functionMatch[1]
			log.Printf("Found function %s\n", currFunc)

			if currSect == "" {
				log.Printf("WARN: Found function %s without section, line=%v\n", currFunc, line)
			} else {
				funcsPerSection[currSect] = append(funcsPerSection[currSect], currFunc)
			}

			if _, ok := sourceMap[currFunc]; !ok {
				sourceMap[currFunc] = make(map[int]*SourceLine)
			}
			functionJustStarted = true // Mark that this function just started so we have the instruction offset of the start
			continue
		}

		// We should have a section and function at this point, ignore the line if we don't
		if currSect == "" || currFunc == "" {
			continue
		}

		line = strings.TrimLeft(line, " \t")
		parts := strings.Split(line, ":")
		insn, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		if functionJustStarted {
			// llvm-objdump counts instructions since the start of the section, but the verifier
			// counts from the start of the function. We need to account for that offset
			insnOffset = insn
			functionJustStarted = false
		}
		insn -= insnOffset

		sourceMap[currFunc][insn] = &SourceLine{
			LineInfo: currLineInfo,
			Line:     currLine,
		}
	}

	return sourceMap, funcsPerSection, nil
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
			sourceMap, funcsPerSect, err = getSourceMap(file)
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
						compl, err := unmarshalComplexity(p.VerifierLog, progSourceMap)
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

func parseRegisterState(regMatch []string) (*RegisterState, error) {
	if len(regMatch) != 4 {
		return nil, fmt.Errorf("failed to parse register state: %v, should have a full match and 3 groups", regMatch)
	}

	regNum, err := strconv.Atoi(regMatch[1])
	if err != nil {
		return nil, fmt.Errorf("cannot parse register number %v: %w", regMatch[1], err)
	}

	livenessCode := regMatch[2]
	var liveness string

	switch livenessCode {
	case "_w":
		liveness = "written"
	case "_r":
		liveness = "read"
	case "_D":
		liveness = "done"
	default:
		liveness = ""
	}

	regValue := regMatch[3]
	infoMatches := regInfoRegex.FindStringSubmatch(regValue)
	if len(infoMatches) == 0 {
		return nil, fmt.Errorf("Cannot parse register value %v", regValue)
	}

	regType := infoMatches[1]
	if regType == "inv" {
		regType = "scalar" // for some reason the verifier logs scalars as "inv", which is confusing
	}

	if regType == "scalar" {
		// Parse scalar values a bit better
		if infoMatches[4] == "" {
			regValue = infoMatches[3] // Parsed the direct scalar value
		} else {
			// Parse the interval
			minValue := int64(0)
			maxValue := int64(0)
			hasRange := false

			for _, kv := range strings.Split(infoMatches[4], ",") {
				kvParts := strings.Split(kv, "=")
				if strings.Contains(kvParts[0], "min") {
					// Ignore errors here, mostly due to sizes (can't parse UINT_MAX in INT64) and for now we don't care
					// about
					v, _ := strconv.ParseInt(kvParts[1], 10, 64)
					minValue = min(v, minValue)
					hasRange = true
				} else if strings.Contains(kvParts[0], "max") {
					v, _ := strconv.ParseInt(kvParts[1], 10, 64)
					maxValue = max(v, maxValue)
					hasRange = true
				}
			}

			if hasRange {
				regValue = fmt.Sprintf("[%s, %s]", tryPowerOfTwoRepresentation(minValue), tryPowerOfTwoRepresentation(maxValue))
			} else {
				regValue = "?"
			}
		}
	} else {
		regValue = strings.Replace(regValue, regType, "", 1) // Remove the type from the value
		regValue = strings.Trim(regValue, "()")              // Remove the parentheses
	}

	return &RegisterState{
		Register: regNum,
		Live:     liveness,
		Type:     regType,
		Value:    regValue,
		Precise:  infoMatches[2] == "P",
	}, nil
}

func unmarshalComplexity(output string, progSourceMap map[int]*SourceLine) (*ComplexityInfo, error) {
	complexity := &ComplexityInfo{
		InsnMap:   make(map[int]*InstructionInfo),
		SourceMap: make(map[string]*SourceLineStats),
	}

	var regStateMatch []string // Matched groups from the last regStateRegex match

	// Read all the verifier log, parse the assembly instructions and count how many times we've seen them
	for _, line := range strings.Split(output, "\n") {
		match := regStateRegex.FindStringSubmatch(line)
		if len(match) > 0 {
			regStateMatch = match // Save the last match with register state, we will use it when we get to an instruction
			continue
		}

		match = insnRegex.FindStringSubmatch(line)
		if len(match) == 0 {
			continue // Only interested in lines that contain assembly instructions
		}
		insIdx, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse instruction index (line is '%s'): %w", line, err)
		}
		if _, ok := complexity.InsnMap[insIdx]; !ok {
			complexity.InsnMap[insIdx] = &InstructionInfo{Index: insIdx}
		}
		complexity.InsnMap[insIdx].TimesProcessed++
		complexity.InsnMap[insIdx].Code = match[2]
		if progSourceMap != nil {
			complexity.InsnMap[insIdx].Source = progSourceMap[insIdx]
		}

		// Now parse the register state if we have it and the instruction number matches
		if len(regStateMatch) >= 3 && regStateMatch[1] == match[1] {
			regData := regStateMatch[2]

			// For ease of parsing, replace certain patterns that introduce spaces and make parsing harder
			regData = strings.ReplaceAll(regData, "; ", ";")

			regMatches := singleRegStateRegex.FindAllStringSubmatch(regData, -1)
			regState := make(map[int]*RegisterState)
			for _, regMatch := range regMatches {
				data, err := parseRegisterState(regMatch)
				if err != nil {
					return nil, fmt.Errorf("failed to parse register state (line is '%s', register is '%s'): %w", line, regData, err)
				}
				regState[data.Register] = data
			}

			complexity.InsnMap[insIdx].RegisterState = regState
			complexity.InsnMap[insIdx].RegisterStateRaw = regStateMatch[0]
		} else {
			log.Printf("WARN: No register state found for instruction %d\n", insIdx)
		}
	}

	// Now build the source map for the source lines
	for _, insn := range complexity.InsnMap {
		if insn.Source == nil {
			continue
		}
		if _, ok := complexity.SourceMap[insn.Source.LineInfo]; !ok {
			complexity.SourceMap[insn.Source.LineInfo] = &SourceLineStats{
				NumInstructions:            0,
				MaxPasses:                  0,
				TotalInstructionsProcessed: 0,
				MinPasses:                  math.MaxInt32,
				AssemblyInsns:              []int{},
			}
		}
		stats := complexity.SourceMap[insn.Source.LineInfo]
		stats.NumInstructions++
		stats.MaxPasses = max(stats.MaxPasses, insn.TimesProcessed)
		stats.MinPasses = min(stats.MinPasses, insn.TimesProcessed)
		stats.TotalInstructionsProcessed += insn.TimesProcessed
		stats.AssemblyInsns = append(stats.AssemblyInsns, insn.Index)
	}

	return complexity, nil
}

func tryPowerOfTwoRepresentation(value int64) string {
	if value == 0 {
		return "0"
	} else if value == math.MaxInt64 {
		// Compute here to avoid overflow
		return "2^63 - 1"
	} else if value == math.MinInt64 {
		return "-2^63"
	}

	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}

	if (value & (value - 1)) == 0 { // Exact power of two, return a nice representation
		return fmt.Sprintf("%s2^%d", sign, int(math.Log2(float64(value))))
	} else if ((value + 1) & value) == 0 { // Value is a power of two minus one
		return fmt.Sprintf("%s2^%d - 1", sign, int(math.Log2(float64(value+1))))
	} else {
		return fmt.Sprintf("%s%d (%s0x%X)", sign, value, sign, value)
	}
}
